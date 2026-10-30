package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/plugin"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// WorkloadSource runs a plugin backup. *plugin.WorkloadGRPCClient
// implements it.
type WorkloadSource interface {
	Backup(ctx context.Context, req plugin.BackupRequest, handle plugin.ItemHandler) (plugin.BackupOutcome, error)
}

// WorkloadRunError is a workload backup that stored nothing, with the
// plugin's reports on each object, so the server can show why per object.
type WorkloadRunError struct {
	Err     error
	Results []plugin.ObjectResult
}

func (e *WorkloadRunError) Error() string {
	var failed []string
	for _, r := range e.Results {
		if r.Status == plugin.ObjectFailed && r.Object != "" {
			failed = append(failed, fmt.Sprintf("%s (%s: %s)", r.Object, r.Reason, r.Message))
		}
	}
	if len(failed) == 0 {
		return e.Err.Error()
	}
	return e.Err.Error() + ": " + strings.Join(failed, "; ")
}

func (e *WorkloadRunError) Unwrap() error { return e.Err }

// abortWatch notes whether reading an item failed on the plugin's side:
// the plugin aborted it, or its stream broke. Either loses nothing already
// stored, unlike a failure to chunk or upload.
type abortWatch struct {
	r      io.Reader
	failed bool
}

func (a *abortWatch) Read(p []byte) (int, error) {
	n, err := a.r.Read(p)
	if err != nil && err != io.EOF {
		a.failed = true
	}
	return n, err
}

func failedResults(rs []plugin.ObjectResult) int {
	n := 0
	for _, r := range rs {
		if r.Status == plugin.ObjectFailed {
			n++
		}
	}
	return n
}

// WorkloadPathPrefix marks the snapshot paths of workload backups, e.g.
// "workload:mssql", so they are never mistaken for filesystem roots.
const WorkloadPathPrefix = "workload:"

// WorkloadBackup backs up a workload through a plugin. Each item the plugin
// produces is stored as the file <plugin id>/<item path> in the snapshot,
// next to a <item path>.meta.json manifest, through the same chunk, dedup,
// compress, and upload pipeline as filesystem backups.
type WorkloadBackup struct {
	Client   *Client
	APIKey   string
	Hostname string
	JobID    string
	Tags     []string

	PluginID      string
	PluginVersion string
	Source        WorkloadSource
	Request       plugin.BackupRequest
	// ServerRecordsResults is set when the server records per-object
	// results (it says so in the schedule).
	ServerRecordsResults bool

	progressFn func(path string, size uint64, isNew bool)
}

// SetProgress sets the per-item progress callback.
func (b *WorkloadBackup) SetProgress(fn func(path string, size uint64, isNew bool)) {
	b.progressFn = fn
}

// Run executes BeginSession → plugin Backup (chunk/upload as items stream
// in) → item manifests → FinalizeBackup.
func (b *WorkloadBackup) Run(ctx context.Context) (*BackupResult, error) {
	hostname := b.Hostname
	if hostname == "" {
		hostname, _ = os.Hostname()
	}
	paths := []string{WorkloadPathPrefix + b.PluginID}
	tags := append(append([]string{}, b.Tags...), "plugin:"+b.PluginID)
	if b.Request.Kind != "" {
		tags = append(tags, "kind:"+b.Request.Kind)
	}
	// A copy-only full is no base for later backups; retention must know.
	if copyOnly, _ := strconv.ParseBool(b.Request.Options["copy_only"]); copyOnly {
		tags = append(tags, "copy-only")
	}

	// With a server that records them, the plugin reports objects it skips
	// or fails and keeps going; an older server would show such a run as a
	// plain success, so the plugin fails it as before. Before a differential
	// or log backup the plugin checks the chain against what the
	// repositories hold.
	req := b.Request
	req.ObjectResults = b.ServerRecordsResults
	if req.Kind != "" && req.Kind != "full" && b.JobID != "" && b.Client != nil {
		known, provided, err := b.Client.GetWorkloadKnown(ctx, b.APIKey, b.JobID)
		if err != nil {
			slog.Warn("could not list the backups the server holds; the plugin checks the chain on its own", "error", err)
		}
		req.Known, req.KnownProvided = known, provided && err == nil
	}

	var (
		beginResp *pb.BeginSessionResponse
		err       error
	)
	if b.JobID != "" {
		beginResp, err = b.Client.BeginSessionWithJob(ctx, b.APIKey, hostname, paths, nil, tags, b.JobID)
	} else {
		beginResp, err = b.Client.BeginSession(ctx, b.APIKey, hostname, paths, nil, tags)
	}
	if err != nil {
		return nil, fmt.Errorf("begin session: %w", err)
	}
	sessionID := beginResp.SessionId
	var idKey [32]byte
	copy(idKey[:], beginResp.IdKey)
	minSize, avgSize, maxSize := uint(beginResp.ChunkerMin), uint(beginResp.ChunkerAvg), uint(beginResp.ChunkerMax)

	slog.Info("workload backup started", "session_id", sessionID, "plugin", b.PluginID,
		"kind", b.Request.Kind, "selection", b.Request.Selection)

	pending := &pendingChunks{chunks: make(map[[32]byte]ChunkData)}
	var mu sync.Mutex
	var nodes []*pb.FileNode
	digests := make(map[string]string)
	mtime := time.Now().UnixNano()

	// store chunks one stream into the snapshot as a regular file.
	store := func(snapPath string, r io.Reader) (uint64, error) {
		ids, n, err := chunkReader(r, idKey, minSize, avgSize, maxSize, pending, &mu)
		if err != nil {
			return 0, err
		}
		content := make([][]byte, len(ids))
		for i := range ids {
			id := ids[i]
			content[i] = id[:]
		}
		nodes = append(nodes, &pb.FileNode{Path: snapPath, Mode: 0o644, MtimeNs: mtime, Size: int64(n), ContentIds: content})
		if pending.totalSize >= flushThreshold {
			if err := flushPendingChunks(ctx, b.Client, sessionID, pending, &mu); err != nil {
				return 0, err
			}
		}
		if b.progressFn != nil {
			b.progressFn(snapPath, n, true)
		}
		return n, nil
	}

	// storeErr is a failure to chunk or upload, as opposed to the plugin's
	// own: the chunks of finished items may be lost with it, so nothing is
	// kept.
	var storeErr error
	handle := func(itemPath string, data io.Reader) error {
		h := sha256.New()
		aw := &abortWatch{r: data}
		if _, err := store(b.PluginID+"/"+itemPath, io.TeeReader(aw, h)); err != nil {
			if !aw.failed {
				storeErr = err // not the plugin's stream: ours
			}
			return fmt.Errorf("store item %s: %w", itemPath, err)
		}
		digests[itemPath] = hex.EncodeToString(h.Sum(nil))
		return nil
	}
	outcome, err := b.Source.Backup(ctx, req, handle)
	if err != nil && req.KnownProvided && len(outcome.Items) == 0 && status.Code(err) == codes.ResourceExhausted {
		// A plugin that takes only small messages: let it check on its own.
		slog.Warn("the plugin refused the list of known backups; retrying without it", "error", err)
		req.Known, req.KnownProvided = nil, false
		outcome, err = b.Source.Backup(ctx, req, handle)
	}
	items, results := outcome.Items, outcome.Results
	if storeErr != nil {
		return nil, &WorkloadRunError{Err: fmt.Errorf("store backup: %w", storeErr), Results: results}
	}
	// failAfter is a plugin failure to report once the finished items are
	// stored: a server that does not record partial runs would show one as
	// a plain success.
	var failAfter error
	if err != nil {
		if errors.Is(err, ErrSessionCancelled) || len(items) == 0 {
			return nil, &WorkloadRunError{Err: fmt.Errorf("plugin backup: %w", err), Results: results}
		}
		if !b.ServerRecordsResults {
			failAfter = &WorkloadRunError{Err: fmt.Errorf("plugin backup failed after %d item(s), which were kept: %w", len(items), err), Results: results}
		}
		// The database already counts the finished items as backed up
		// (a log backup truncates the log): keep them, and report the rest.
		slog.Warn("workload backup failed part way; keeping the items that finished", "items", len(items), "error", err)
		results = append(results, plugin.ObjectResult{Status: plugin.ObjectFailed, Reason: "error", Message: err.Error()})
	}
	if len(items) == 0 {
		switch {
		case failedResults(results) > 0:
			return nil, &WorkloadRunError{Err: errors.New("no object could be backed up"), Results: results}
		case len(results) == 0:
			return nil, errors.New("plugin backup produced no items")
		}
		// Every object was excluded by design: a run with nothing to do.
	}

	created := time.Now().UTC()
	for _, it := range items {
		manifest, err := json.MarshalIndent(plugin.ItemManifest{
			Plugin:        b.PluginID,
			PluginVersion: b.PluginVersion,
			Item:          it.Path,
			Kind:          b.Request.Kind,
			Size:          it.Size,
			SHA256:        digests[it.Path],
			CreatedAt:     created,
			Metadata:      it.Metadata,
		}, "", "  ")
		if err != nil {
			return nil, err
		}
		if _, err := store(b.PluginID+"/"+it.Path+plugin.MetadataSuffix, bytes.NewReader(manifest)); err != nil {
			return nil, fmt.Errorf("store manifest of %s: %w", it.Path, err)
		}
	}

	if err := flushPendingChunks(ctx, b.Client, sessionID, pending, &mu); err != nil {
		return nil, err
	}
	nodes = append(nodes, parentDirNodes(nodes)...)

	slog.Info("finalizing workload backup", "items", len(items), "file_nodes", len(nodes), "objects_without_items", len(results))
	finalResp, err := b.Client.FinalizeWorkload(ctx, sessionID, nodes, results)
	if err != nil {
		return nil, fmt.Errorf("finalize backup: %w", err)
	}
	if failAfter != nil {
		return nil, failAfter
	}
	return &BackupResult{
		SnapshotID:     finalResp.SnapshotId,
		FilesNew:       finalResp.FilesNew,
		FilesUnchanged: finalResp.FilesUnchanged,
		BytesAdded:     finalResp.BytesAdded,
		BytesProcessed: finalResp.BytesProcessed,
		Duration:       finalResp.DurationSeconds,
	}, nil
}

// parentDirNodes returns directory nodes for every ancestor of the given
// file nodes, so the snapshot tree is complete.
func parentDirNodes(files []*pb.FileNode) []*pb.FileNode {
	dirs := make(map[string]bool)
	for _, f := range files {
		for d := path.Dir(f.Path); d != "." && d != "/" && !dirs[d]; d = path.Dir(d) {
			dirs[d] = true
		}
	}
	names := make([]string, 0, len(dirs))
	for d := range dirs {
		names = append(names, d)
	}
	sort.Strings(names)
	out := make([]*pb.FileNode, 0, len(names))
	for _, d := range names {
		out = append(out, &pb.FileNode{Path: d, IsDir: true, Mode: 0o755})
	}
	return out
}
