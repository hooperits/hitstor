package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/hooperits/hitstor/internal/chunker"
	"github.com/hooperits/hitstor/internal/compress"
	"github.com/hooperits/hitstor/internal/crypto"
	"github.com/hooperits/hitstor/internal/fs"
	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/scanner"
	"github.com/hooperits/hitstor/internal/sysprofile"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

const flushThreshold = 256 * 1024 * 1024 // 256 MiB pending compressed data

// ErrSessionCancelled is returned when the server reports the session no longer
// exists, typically because the backup was cancelled from the dashboard.
var ErrSessionCancelled = errors.New("backup session cancelled by server")

// Backup orchestrates a remote backup to a HitStor server.
type Backup struct {
	Client   *Client
	APIKey   string
	Paths    []string
	Excludes []string
	Hostname string
	Tags     []string
	JobID    string // Links session to a BackupJob (empty for legacy agents).
	Parallel int
	// UseVSS enables consistent filesystem snapshots while reading:
	// Windows VSS, Linux LVM or btrfs when available. Falls back to live paths.
	UseVSS bool
	// CaptureXattrs records extended attributes (SELinux contexts, file
	// capabilities, POSIX ACLs) alongside each node. Required for bare-metal
	// restore; costs two extra syscalls per entry, so it is opt-in.
	CaptureXattrs bool
	// CaptureSystemProfile records the host's disk and boot layout in the
	// snapshot, so a recovery can rebuild the machine's shape, not just its
	// files. Paired with CaptureXattrs for bare-metal restore.
	CaptureSystemProfile bool
	Scanner              scanner.Scanner // Optional custom filesystem scanner (plugin).

	// Progress callbacks.
	progressFn     func(path string, size uint64, isNew bool)
	scanCompleteFn func(files, dirs int, totalBytes uint64)
}

// BackupResult holds the outcome of a backup operation.
type BackupResult struct {
	SnapshotID     string
	FilesNew       uint64
	FilesUnchanged uint64
	FilesSkipped   uint64 // files skipped due to open/read errors
	BytesAdded     uint64
	BytesProcessed uint64
	Duration       float64
}

// SetProgress sets the per-file progress callback.
func (b *Backup) SetProgress(fn func(path string, size uint64, isNew bool)) {
	b.progressFn = fn
}

// SetScanComplete sets the callback invoked when scanning finishes.
func (b *Backup) SetScanComplete(fn func(files, dirs int, totalBytes uint64)) {
	b.scanCompleteFn = fn
}

// fileEntry holds scan results and processed metadata for a single entry.
type fileEntry struct {
	absPath    string
	node       fs.Node
	contentIDs [][32]byte // ordered blob IDs for file chunks
	skipped    bool       // true if file couldn't be read (non-fatal error)
}

// Run executes the full backup pipeline:
// BeginSession → Scan → Chunk/Compress → CheckBlobs → Upload → Finalize.
func (b *Backup) Run(ctx context.Context) (*BackupResult, error) {
	// 0. Merge platform-specific default excludes.
	if len(defaultExcludes) > 0 {
		b.Excludes = append(defaultExcludes, b.Excludes...)
	}

	// 1. BeginSession.
	hostname := b.Hostname
	if hostname == "" {
		hostname, _ = os.Hostname()
	}

	slog.Info("beginning session", "server", b.Client.conn.Target(), "hostname", hostname)
	var (
		beginResp *pb.BeginSessionResponse
		err       error
	)
	if b.JobID != "" {
		beginResp, err = b.Client.BeginSessionWithJob(ctx, b.APIKey, hostname, b.Paths, b.Excludes, b.Tags, b.JobID)
	} else {
		beginResp, err = b.Client.BeginSession(ctx, b.APIKey, hostname, b.Paths, b.Excludes, b.Tags)
	}
	if err != nil {
		return nil, fmt.Errorf("begin session: %w", err)
	}

	sessionID := beginResp.SessionId
	var idKey [32]byte
	copy(idKey[:], beginResp.IdKey)
	chunkerMin := uint(beginResp.ChunkerMin)
	chunkerAvg := uint(beginResp.ChunkerAvg)
	chunkerMax := uint(beginResp.ChunkerMax)

	// Build incremental lookup map from previous snapshot metadata.
	previousFiles := ParsePreviousFiles(beginResp)
	prevMap := make(map[string]PreviousFile, len(previousFiles))
	for _, pf := range previousFiles {
		prevMap[pf.Path] = pf
	}

	backupType := "full"
	if len(previousFiles) > 0 {
		backupType = "incremental"
	}

	slog.Info("session started",
		"session_id", sessionID,
		"backup_type", backupType,
		"parent_snapshot", beginResp.ParentSnapshotId,
		"previous_files", len(previousFiles),
		"chunker_min", chunkerMin,
		"chunker_avg", chunkerAvg,
		"chunker_max", chunkerMax,
	)

	// 2. Consistent snapshots (Windows VSS / Linux LVM|btrfs).
	// Metadata paths always use original live roots so restores target real paths.
	consistency, cerr := PrepareConsistency(b.Paths, b.UseVSS)
	if consistency != nil {
		defer consistency.Close()
	}
	if cerr != nil {
		slog.Error("consistency snapshot failed, falling back to live paths", "error", cerr)
	} else if consistency != nil && consistency.Method != ConsistencyNone {
		b.Paths = consistency.ReadPaths
		slog.Info("using consistent read paths",
			"method", consistency.Method,
			"original", consistency.OriginalPaths,
			"read", consistency.ReadPaths,
		)
	}

	// 3. Scan filesystem.
	entries, totalFiles, totalDirs, totalBytes, err := b.scan()
	if err != nil {
		return nil, err
	}

	if b.scanCompleteFn != nil {
		b.scanCompleteFn(totalFiles, totalDirs, totalBytes)
	}

	slog.Info("scan complete", "files", totalFiles, "dirs", totalDirs, "bytes", totalBytes)

	// 3. Process files: chunk, compute BlobID, compress.
	// For incremental backups, skip files whose mtime and size match the previous snapshot.
	pending := &pendingChunks{
		chunks: make(map[[32]byte]ChunkData),
	}

	parallel := b.Parallel
	if parallel <= 0 {
		parallel = runtime.NumCPU()
		if parallel > 8 {
			parallel = 8
		}
	}

	// Collect file entries for parallel processing, applying incremental skip.
	// Relative paths are computed from original (live) roots when consistency is active.
	metaPaths := b.Paths
	if consistency != nil && len(consistency.OriginalPaths) > 0 {
		metaPaths = consistency.OriginalPaths
	}
	var fileEntries []*fileEntry
	var skippedFiles int
	for _, e := range entries {
		if e.node.Type != fs.NodeTypeFile {
			continue
		}
		relPath := relativePathFor(e.absPath, consistency, metaPaths)
		if prev, ok := prevMap[relPath]; ok && e.node.MtimeNs == prev.MtimeNs && int64(e.node.Size) == prev.Size {
			// File unchanged — reuse content IDs from previous snapshot.
			e.contentIDs = prev.ContentIDs
			skippedFiles++
			if b.progressFn != nil {
				b.progressFn(e.absPath, e.node.Size, false)
			}
			continue
		}
		fileEntries = append(fileEntries, e)
	}

	if skippedFiles > 0 {
		slog.Info("incremental skip", "skipped", skippedFiles, "to_process", len(fileEntries))
	}

	// Process files in parallel with bounded concurrency.
	var mu sync.Mutex
	var processErr error    // infrastructure errors only (gRPC, cancellation)
	var filesErrored uint64 // files skipped due to open/read errors
	work := make(chan *fileEntry, parallel)
	var wg sync.WaitGroup

	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fe := range work {
				contentIDs, bytesRead, err := processFile(fe.absPath, idKey, chunkerMin, chunkerAvg, chunkerMax, pending, &mu)
				if err != nil {
					// Per-file error (locked, permission denied, etc.): skip and continue.
					slog.Warn("skipping file", "path", fe.absPath, "error", err)
					mu.Lock()
					fe.skipped = true
					filesErrored++
					mu.Unlock()
					continue
				}

				mu.Lock()
				fe.contentIDs = contentIDs
				mu.Unlock()

				isNew := len(contentIDs) > 0
				if b.progressFn != nil {
					b.progressFn(fe.absPath, bytesRead, isNew)
				}

				// Check if we need to flush pending chunks.
				mu.Lock()
				pendingSize := pending.totalSize
				mu.Unlock()

				if pendingSize >= flushThreshold {
					if err := b.flushPending(ctx, sessionID, pending, &mu); err != nil {
						mu.Lock()
						if processErr == nil {
							processErr = err
						}
						mu.Unlock()
						return
					}
				}
			}
		}()
	}

	for _, fe := range fileEntries {
		mu.Lock()
		hasErr := processErr != nil
		mu.Unlock()
		if hasErr {
			break
		}
		work <- fe
	}
	close(work)
	wg.Wait()

	if processErr != nil {
		return nil, processErr
	}

	// 4. Flush any remaining pending chunks.
	if err := b.flushPending(ctx, sessionID, pending, &mu); err != nil {
		return nil, err
	}

	// 5. Build FileNode list and FinalizeBackup.
	fileNodes := buildFileNodesWithConsistency(entries, consistency, metaPaths)

	slog.Info("finalizing backup",
		"file_nodes", len(fileNodes),
		"backup_type", backupType,
		"files_skipped_incremental", skippedFiles,
		"files_skipped_errors", filesErrored,
	)
	finalResp, err := b.Client.FinalizeBackup(ctx, sessionID, fileNodes, b.captureSystemProfile(ctx))
	if err != nil {
		return nil, fmt.Errorf("finalize backup: %w", err)
	}

	return &BackupResult{
		SnapshotID:     finalResp.SnapshotId,
		FilesNew:       finalResp.FilesNew,
		FilesUnchanged: finalResp.FilesUnchanged,
		FilesSkipped:   filesErrored,
		BytesAdded:     finalResp.BytesAdded,
		BytesProcessed: finalResp.BytesProcessed,
		Duration:       finalResp.DurationSeconds,
	}, nil
}

// pendingChunks tracks compressed chunks waiting to be checked and uploaded.
type pendingChunks struct {
	chunks    map[[32]byte]ChunkData
	allIDs    [][32]byte // all blob IDs seen (for CheckBlobs)
	totalSize int64      // total compressed bytes pending
}

func (p *pendingChunks) add(id [32]byte, data ChunkData) {
	if _, exists := p.chunks[id]; exists {
		return // already tracked
	}
	p.chunks[id] = data
	p.allIDs = append(p.allIDs, id)
	p.totalSize += int64(len(data.CompressedData))
}

// processFile chunks a file, computes blob IDs, compresses data, and adds to pending.
// Returns the ordered content IDs and bytes read.
func processFile(absPath string, idKey [32]byte, minSize, avgSize, maxSize uint, pending *pendingChunks, mu *sync.Mutex) ([][32]byte, uint64, error) {
	f, err := openFileForBackup(absPath)
	if err != nil {
		return nil, 0, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	ch := chunker.New(f, minSize, avgSize, maxSize)

	var contentIDs [][32]byte
	var bytesRead uint64

	for {
		chunk, err := ch.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, fmt.Errorf("chunk: %w", err)
		}

		bytesRead += uint64(chunk.Length)

		// Compute blob ID from uncompressed data.
		blobID := crypto.BlobID(idKey, chunk.Data)
		contentIDs = append(contentIDs, blobID)

		// Compress the chunk data.
		compressed, err := compress.Compress(chunk.Data)
		if err != nil {
			return nil, 0, fmt.Errorf("compress: %w", err)
		}

		mu.Lock()
		pending.add(blobID, ChunkData{
			BlobID:          blobID,
			CompressedData:  compressed,
			UncompressedLen: uint32(len(chunk.Data)),
		})
		mu.Unlock()
	}

	return contentIDs, bytesRead, nil
}

// flushPending does CheckBlobs + UploadChunks for all accumulated pending chunks,
// then clears the pending set.
func (b *Backup) flushPending(ctx context.Context, sessionID string, pending *pendingChunks, mu *sync.Mutex) error {
	mu.Lock()
	if len(pending.allIDs) == 0 {
		mu.Unlock()
		return nil
	}

	// Take a snapshot of IDs and chunks.
	ids := make([][32]byte, len(pending.allIDs))
	copy(ids, pending.allIDs)
	chunks := make(map[[32]byte]ChunkData, len(pending.chunks))
	for k, v := range pending.chunks {
		chunks[k] = v
	}

	// Reset pending.
	pending.chunks = make(map[[32]byte]ChunkData)
	pending.allIDs = nil
	pending.totalSize = 0
	mu.Unlock()

	// CheckBlobs to find which are needed.
	slog.Info("checking blobs", "count", len(ids))
	needed, err := b.Client.CheckBlobs(ctx, sessionID, ids)
	if err != nil {
		if isSessionNotFound(err) {
			return ErrSessionCancelled
		}
		return fmt.Errorf("check blobs: %w", err)
	}

	// Upload only needed chunks.
	var toUpload []ChunkData
	for id := range needed {
		if chunk, ok := chunks[id]; ok {
			toUpload = append(toUpload, chunk)
		}
	}

	if len(toUpload) > 0 {
		slog.Info("uploading chunks", "needed", len(toUpload), "total", len(ids))
		if _, err := b.Client.UploadChunks(ctx, sessionID, toUpload); err != nil {
			if isSessionNotFound(err) {
				return ErrSessionCancelled
			}
			return fmt.Errorf("upload chunks: %w", err)
		}
	} else {
		slog.Info("all chunks deduplicated", "total", len(ids))
	}

	return nil
}

// isSessionNotFound checks if a gRPC error indicates the session was not found
// (typically because the backup was cancelled from the dashboard).
func isSessionNotFound(err error) bool {
	if st, ok := grpcstatus.FromError(err); ok {
		return st.Code() == codes.NotFound
	}
	return false
}

// scan walks all paths and collects file/dir/symlink entries.
func (b *Backup) scan() ([]*fileEntry, int, int, uint64, error) {
	var entries []*fileEntry
	var totalFiles, totalDirs int
	var totalBytes uint64

	if b.Scanner != nil {
		ch, err := b.Scanner.Scan(b.Paths, b.Excludes)
		if err != nil {
			return nil, 0, 0, 0, fmt.Errorf("scan: %w", err)
		}
		for result := range ch {
			if result.Err != nil {
				slog.Warn("scan error", "error", result.Err)
				continue
			}
			entries = append(entries, &fileEntry{
				absPath: result.Path,
				node:    result.Node,
			})
			switch result.Node.Type {
			case fs.NodeTypeFile:
				totalFiles++
				totalBytes += result.Node.Size
			case fs.NodeTypeDir:
				totalDirs++
			}
		}
		return entries, totalFiles, totalDirs, totalBytes, nil
	}

	for _, root := range b.Paths {
		walkOpts := fs.WalkOptions{Excludes: b.Excludes, Xattrs: b.CaptureXattrs}
		err := fs.WalkWithOptions(root, walkOpts, func(path string, node fs.Node) error {
			entries = append(entries, &fileEntry{
				absPath: path,
				node:    node,
			})
			switch node.Type {
			case fs.NodeTypeFile:
				totalFiles++
				totalBytes += node.Size
			case fs.NodeTypeDir:
				totalDirs++
			}
			return nil
		})
		if err != nil {
			return nil, 0, 0, 0, fmt.Errorf("scan %s: %w", root, err)
		}
	}

	return entries, totalFiles, totalDirs, totalBytes, nil
}

// relativePathFor maps a scanned absolute path to the metadata-relative path,
// rewriting snapshot device paths back to live roots when consistency is active.
func relativePathFor(absReadPath string, cs *ConsistencySession, originalRoots []string) string {
	if cs != nil && cs.Method != ConsistencyNone {
		return cs.RelativePath(absReadPath)
	}
	return makeRelativePath(absReadPath, originalRoots)
}

// buildFileNodesWithConsistency converts scan entries into protobuf FileNode messages.
// Paths are made relative to the original live backup roots (not VSS/LVM device paths).
func buildFileNodesWithConsistency(entries []*fileEntry, cs *ConsistencySession, originalRoots []string) []*pb.FileNode {
	var nodes []*pb.FileNode
	addedRoots := make(map[string]bool)

	for _, e := range entries {
		if e.skipped {
			continue
		}
		relPath := relativePathFor(e.absPath, cs, originalRoots)

		node := &pb.FileNode{
			Path:    relPath,
			IsDir:   e.node.Type == fs.NodeTypeDir,
			Mode:    e.node.Mode,
			MtimeNs: e.node.MtimeNs,
			Size:    int64(e.node.Size),
			Uid:     e.node.UID,
			Gid:     e.node.GID,
		}

		if e.node.Type == fs.NodeTypeFile {
			contentBytes := make([][]byte, len(e.contentIDs))
			for i, id := range e.contentIDs {
				idCopy := id
				contentBytes[i] = idCopy[:]
			}
			node.ContentIds = contentBytes
		}

		if e.node.Type == fs.NodeTypeSymlink {
			node.LinkTarget = e.node.LinkTarget
		}

		if len(e.node.Xattrs) > 0 {
			node.Xattrs = e.node.Xattrs
		}

		nodes = append(nodes, node)

		if e.node.Type == fs.NodeTypeDir {
			addedRoots[relPath] = true
		}
		dir := filepath.Dir(relPath)
		for dir != "" && dir != "." && !addedRoots[dir] {
			addedRoots[dir] = true
			dir = filepath.Dir(dir)
		}
	}

	for dir := range addedRoots {
		found := false
		for _, n := range nodes {
			if n.Path == dir && n.IsDir {
				found = true
				break
			}
		}
		if !found {
			nodes = append(nodes, &pb.FileNode{
				Path:  dir,
				IsDir: true,
				Mode:  0755,
			})
		}
	}

	return nodes
}

// makeRelativePath computes a relative path from the backup root.
// For a file /data/foo/bar.txt backed up from root /data, returns "foo/bar.txt".
// The root directory itself returns its basename.
func makeRelativePath(absPath string, backupPaths []string) string {
	for _, root := range backupPaths {
		cleanRoot := filepath.Clean(root)
		cleanPath := filepath.Clean(absPath)

		if cleanPath == cleanRoot {
			// The root directory itself — use base name.
			return filepath.Base(cleanRoot)
		}

		prefix := cleanRoot + string(filepath.Separator)
		if strings.HasPrefix(cleanPath, prefix) {
			rel := cleanPath[len(prefix):]
			// Prepend the root base name so multiple roots don't collide.
			return filepath.Join(filepath.Base(cleanRoot), rel)
		}
	}

	// Fallback: use the basename.
	return filepath.Base(absPath)
}

// captureSystemProfile records the agent host's disk and boot layout for
// bare-metal restore, encoded as JSON for the server to store verbatim.
//
// Returns nil when the job does not capture it, or when capture fails: a
// missing layout degrades bare-metal readiness but must never fail a backup
// that has already transferred its data.
func (b *Backup) captureSystemProfile(ctx context.Context) []byte {
	if !b.CaptureSystemProfile {
		return nil
	}

	profile, err := sysprofile.Capture(ctx)
	if err != nil {
		slog.Warn("system profile not captured", "error", err)
		return nil
	}

	encoded, err := json.Marshal(profile)
	if err != nil {
		slog.Warn("system profile not encoded", "error", err)
		return nil
	}

	slog.Info("captured system profile",
		"disks", len(profile.Disks),
		"warnings", len(profile.Warnings),
	)
	return encoded
}
