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
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/plugin"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
)

// WorkloadSource runs a plugin backup. *plugin.WorkloadGRPCClient
// implements it.
type WorkloadSource interface {
	Backup(ctx context.Context, req plugin.BackupRequest, handle plugin.ItemHandler) ([]plugin.BackupItem, error)
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

	items, err := b.Source.Backup(ctx, b.Request, func(itemPath string, data io.Reader) error {
		h := sha256.New()
		if _, err := store(b.PluginID+"/"+itemPath, io.TeeReader(data, h)); err != nil {
			return fmt.Errorf("store item %s: %w", itemPath, err)
		}
		digests[itemPath] = hex.EncodeToString(h.Sum(nil))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("plugin backup: %w", err)
	}
	if len(items) == 0 {
		return nil, errors.New("plugin backup produced no items")
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

	slog.Info("finalizing workload backup", "items", len(items), "file_nodes", len(nodes))
	finalResp, err := b.Client.FinalizeBackup(ctx, sessionID, nodes, nil)
	if err != nil {
		return nil, fmt.Errorf("finalize backup: %w", err)
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
