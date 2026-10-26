package archiver

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/cache"
	"github.com/hooperits/hitstor/internal/chunker"
	"github.com/hooperits/hitstor/internal/compress"
	"github.com/hooperits/hitstor/internal/crypto"
	"github.com/hooperits/hitstor/internal/fs"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/scanner"
	"github.com/hooperits/hitstor/internal/sysprofile"
)

const packSizeThreshold = 128 * 1024 * 1024 // 128 MiB

// Archiver orchestrates the backup process: scan → chunk → dedup → compress →
// encrypt → pack → index → tree → snapshot.
type Archiver struct {
	repo *repository.Repository

	// Pack management.
	pw    *repository.PackWriter
	index *repository.Index

	// Dedup tracking for blobs added in this backup session.
	known map[backend.ID]bool

	// Optional files cache for incremental backups.
	fc *cache.FilesCache

	// Concurrency control.
	mu       sync.Mutex
	parallel int

	// Progress callbacks.
	progressFn     func(path string, size uint64, isNew bool)
	scanCompleteFn func(files, dirs int, totalBytes uint64)

	// Optional custom filesystem scanner (plugin).
	sc scanner.Scanner

	// DryRun stops after scan phase without processing or writing data.
	dryRun bool

	// captureSystemProfile records the host's storage and boot layout in the
	// snapshot, so a bare-metal restore can rebuild the machine's shape and
	// not only its contents.
	captureSystemProfile bool

	// Statistics.
	summary repository.SnapshotSummary
}

// New creates a new Archiver backed by the given repository.
func New(repo *repository.Repository) *Archiver {
	return &Archiver{
		repo:  repo,
		index: &repository.Index{},
		known: make(map[backend.ID]bool),
	}
}

// SetCache enables the files cache for incremental backups.
func (a *Archiver) SetCache(fc *cache.FilesCache) {
	a.fc = fc
}

// SetParallel sets the number of parallel workers for file processing.
func (a *Archiver) SetParallel(n int) {
	a.parallel = n
}

// SetProgress sets a callback invoked for each file processed during backup.
func (a *Archiver) SetProgress(fn func(path string, size uint64, isNew bool)) {
	a.progressFn = fn
}

// SetScanComplete sets a callback invoked when the filesystem scan finishes.
func (a *Archiver) SetScanComplete(fn func(files, dirs int, totalBytes uint64)) {
	a.scanCompleteFn = fn
}

// SetDryRun enables dry-run mode: scan only, no data processing or writing.
func (a *Archiver) SetDryRun(dryRun bool) {
	a.dryRun = dryRun
}

// SetScanner sets a custom filesystem scanner (e.g., a plugin scanner).
// When nil, the built-in default scanner is used.
func (a *Archiver) SetScanner(s scanner.Scanner) {
	a.sc = s
}

// SetCaptureSystemProfile records the host's storage and boot layout alongside
// the snapshot. Required for bare-metal restore.
func (a *Archiver) SetCaptureSystemProfile(capture bool) {
	a.captureSystemProfile = capture
}

// Backup scans the given paths, chunks and deduplicates file data, builds a
// tree of directory blobs, and writes a snapshot to the repository. Returns the
// completed snapshot and its ID.
func (a *Archiver) Backup(paths []string, excludes []string, hostname string, tags []string) (snapID backend.ID, snap *repository.Snapshot, err error) {
	startTime := time.Now()
	mk := a.repo.Key()

	slog.Info("backup started", "paths", paths, "excludes", excludes)

	// Resolve hostname if not provided.
	if hostname == "" {
		hostname, _ = os.Hostname()
	}

	// Resolve absolute paths.
	absPaths := make([]string, len(paths))
	for i, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return backend.ID{}, nil, fmt.Errorf("archiver: resolve path %s: %w", p, err)
		}
		absPaths[i] = abs
	}

	// Phase 1: Scan filesystem.
	var sc scanner.Scanner
	if a.sc != nil {
		sc = a.sc
	} else {
		sc = scanner.NewDefault()
	}
	ch, err := sc.Scan(absPaths, excludes)
	if err != nil {
		return backend.ID{}, nil, fmt.Errorf("archiver: scan: %w", err)
	}

	// Collect entries organized by parent directory.
	// entries[absPath] stores the node for each scanned path.
	type entry struct {
		node fs.Node
	}
	entries := map[string]*entry{}
	// children[parentAbsPath] = list of child absolute paths.
	children := map[string][]string{}
	// rootPaths are the backup root entries (the paths passed to backup).
	rootPathSet := map[string]bool{}
	for _, p := range absPaths {
		rootPathSet[p] = true
	}

	for result := range ch {
		if result.Err != nil {
			slog.Warn("scan error", "path", result.Path, "error", result.Err)
			continue
		}
		absPath := result.Path
		entries[absPath] = &entry{node: result.Node}

		if !rootPathSet[absPath] {
			parentDir := filepath.Dir(absPath)
			children[parentDir] = append(children[parentDir], absPath)
		}
	}

	// Populate entries for root paths. fs.Walk skips the root directory
	// itself (relPath == "."), so we create nodes for root paths manually.
	for _, rootPath := range absPaths {
		if entries[rootPath] != nil {
			continue // already present (shouldn't happen, but be safe)
		}
		fi, err := os.Lstat(rootPath)
		if err != nil {
			return backend.ID{}, nil, fmt.Errorf("archiver: stat root path %s: %w", rootPath, err)
		}
		node, err := fs.NodeFromFileInfo(rootPath, fi)
		if err != nil {
			return backend.ID{}, nil, fmt.Errorf("archiver: node for root path %s: %w", rootPath, err)
		}
		entries[rootPath] = &entry{node: node}
	}

	// Report scan completion.
	var scanFileCount, scanDirCount int
	var scanTotalBytes uint64
	for _, e := range entries {
		if e.node.Type == fs.NodeTypeDir {
			scanDirCount++
		} else if e.node.Type == fs.NodeTypeFile {
			scanFileCount++
			scanTotalBytes += e.node.Size
		}
	}
	if a.scanCompleteFn != nil {
		a.scanCompleteFn(scanFileCount, scanDirCount, scanTotalBytes)
	}

	// Dry-run: return scan summary without processing or writing data.
	if a.dryRun {
		a.summary.TotalFiles = uint64(scanFileCount)
		a.summary.TotalSize = scanTotalBytes
		a.summary.DirsNew = uint64(scanDirCount)
		a.summary.TotalDuration = time.Since(startTime).Seconds()
		return backend.ID{}, &repository.Snapshot{
			Time:     startTime,
			Hostname: hostname,
			Paths:    absPaths,
			Tags:     tags,
			Summary:  a.summary,
		}, nil
	}

	// Phase 2: Process files (chunk → dedup → compress → pack).
	cfg := a.repo.Cfg()

	type fileWork struct {
		absPath string
		entry   *entry
	}
	var files []fileWork
	for absPath, e := range entries {
		if e.node.Type != fs.NodeTypeFile {
			continue
		}
		files = append(files, fileWork{absPath: absPath, entry: e})
	}

	workers := a.parallel
	if workers <= 0 {
		workers = 1
	}
	if workers > len(files) {
		workers = len(files)
	}

	var (
		wg       sync.WaitGroup
		firstErr error
		errOnce  sync.Once
	)

	work := make(chan fileWork, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fw := range work {
				contentIDs, bytesProcessed, changed, isNew, err := a.processFile(fw.absPath, fw.entry.node, mk, cfg)
				if err != nil {
					errOnce.Do(func() { firstErr = fmt.Errorf("archiver: process %s: %w", fw.absPath, err) })
					slog.Warn("file processing error, skipping", "path", fw.absPath, "error", err)
					continue
				}
				if changed {
					slog.Warn("file changed during read, skipping", "path", fw.absPath)
					continue
				}

				slog.Debug("file processed", "path", fw.absPath, "bytes", bytesProcessed)

				a.mu.Lock()
				fw.entry.node.Content = contentIDs
				a.summary.BytesProcessed += bytesProcessed
				a.summary.TotalFiles++
				a.summary.TotalSize += fw.entry.node.Size
				if isNew {
					a.summary.FilesNew++
				} else {
					a.summary.FilesUnchanged++
				}
				a.mu.Unlock()

				if a.progressFn != nil {
					a.progressFn(fw.absPath, fw.entry.node.Size, isNew)
				}
			}
		}()
	}

	for _, fw := range files {
		work <- fw
	}
	close(work)
	wg.Wait()

	if firstErr != nil {
		return backend.ID{}, nil, firstErr
	}

	// Phase 3: Build tree blobs bottom-up.
	// Collect all directory paths and sort from deepest to shallowest.
	dirPaths := make([]string, 0)
	for absPath, e := range entries {
		if e.node.Type == fs.NodeTypeDir {
			dirPaths = append(dirPaths, absPath)
			a.summary.DirsNew++
		}
	}
	sort.Slice(dirPaths, func(i, j int) bool {
		return strings.Count(dirPaths[i], string(filepath.Separator)) >
			strings.Count(dirPaths[j], string(filepath.Separator))
	})

	// treeIDs maps directory absolute path to its tree blob ID.
	treeIDs := map[string]backend.ID{}

	for _, dirPath := range dirPaths {
		tb := NewTreeBuilder(mk.IDKey)

		for _, childPath := range children[dirPath] {
			childEntry := entries[childPath]
			if childEntry == nil {
				continue
			}
			node := childEntry.node

			// For directory children, set subtree to their tree blob ID.
			if node.Type == fs.NodeTypeDir {
				if treeID, ok := treeIDs[childPath]; ok {
					node.Subtree = fs.ID(treeID)
				}
			}

			tb.AddNode(node)
		}

		blobID, compData, uncompLen, err := tb.Finalize()
		if err != nil {
			return backend.ID{}, nil, fmt.Errorf("archiver: build tree for %s: %w", dirPath, err)
		}

		// Dedup: skip tree blob if it already exists in this session or repo index.
		if !a.known[blobID] {
			if existing, _ := a.repo.LookupBlob(blobID); existing != nil {
				a.known[blobID] = true
			} else {
				if err := a.addBlob(blobID, repository.TreeBlob, compData, uncompLen); err != nil {
					return backend.ID{}, nil, err
				}
			}
		}

		treeIDs[dirPath] = blobID
	}

	// Build root tree blob from backup root entries.
	rootTB := NewTreeBuilder(mk.IDKey)
	for _, rootPath := range absPaths {
		e := entries[rootPath]
		if e == nil {
			continue
		}
		node := e.node
		if node.Type == fs.NodeTypeDir {
			if treeID, ok := treeIDs[rootPath]; ok {
				node.Subtree = fs.ID(treeID)
			}
		}
		rootTB.AddNode(node)
	}

	rootBlobID, rootCompData, rootUncompLen, err := rootTB.Finalize()
	if err != nil {
		return backend.ID{}, nil, fmt.Errorf("archiver: build root tree: %w", err)
	}

	// Dedup: skip root tree blob if it already exists.
	if !a.known[rootBlobID] {
		if existing, _ := a.repo.LookupBlob(rootBlobID); existing != nil {
			a.known[rootBlobID] = true
		} else {
			if err := a.addBlob(rootBlobID, repository.TreeBlob, rootCompData, rootUncompLen); err != nil {
				return backend.ID{}, nil, err
			}
		}
	}

	// Capture the host layout before flushing, so its blob lands in a pack and
	// is covered by the index written in the next phase.
	systemProfileID, err := a.saveSystemProfile()
	if err != nil {
		// A missing layout must not cost the backup; the snapshot is still
		// valid for file-level restore.
		slog.Warn("archiver: system profile not captured", "error", err)
	}

	// Phase 4: Flush remaining pack data.
	if err := a.flushPack(); err != nil {
		return backend.ID{}, nil, err
	}

	// Phase 5: Write index (crash consistency: index before snapshot).
	if len(a.index.Packs) > 0 {
		if _, err := a.repo.SaveIndex(a.index); err != nil {
			return backend.ID{}, nil, fmt.Errorf("archiver: save index: %w", err)
		}
	}

	// Phase 6: Create and save snapshot.
	username := ""
	if u, err := user.Current(); err == nil {
		username = u.Username
	}

	a.summary.TotalDuration = time.Since(startTime).Seconds()

	snap = &repository.Snapshot{
		Time:     time.Now(),
		Hostname: hostname,
		Username: username,
		Paths:    paths,
		Excludes: excludes,
		Tree:     backend.ID(rootBlobID),
		Tags:     tags,
		Summary:  a.summary,

		SystemProfile: systemProfileID,
	}

	snapID, err = a.repo.SaveSnapshot(snap)
	if err != nil {
		return backend.ID{}, nil, fmt.Errorf("archiver: save snapshot: %w", err)
	}

	if err := a.repo.MaybeConsolidateIndexes(); err != nil {
		slog.Warn("archiver: consolidate indexes", "error", err)
	}

	slog.Info("backup complete",
		"snapshot", snapID.String()[:8],
		"files_new", a.summary.FilesNew,
		"files_unchanged", a.summary.FilesUnchanged,
		"bytes_added", a.summary.BytesAdded,
	)

	return snapID, snap, nil
}

// Summary returns the current backup statistics.
func (a *Archiver) Summary() repository.SnapshotSummary {
	return a.summary
}

// processFile reads a file, chunks it with FastCDC, deduplicates chunks, and
// packs new ones. Returns the ordered list of content blob IDs, total bytes read,
// and whether the file changed during read (in which case it should be skipped).
func (a *Archiver) processFile(absPath string, node fs.Node, mk *crypto.MasterKey, cfg repository.Config) ([]fs.ID, uint64, bool, bool, error) {
	pathHash := sha256.Sum256([]byte(absPath))

	// Check files cache for unchanged file (incremental backup).
	if a.fc != nil {
		cachedIDs, ok := a.fc.LookupFile(pathHash, node.Inode, node.Size, node.MtimeNs)
		if ok {
			// File unchanged — reuse cached chunk IDs.
			contentIDs := make([]fs.ID, len(cachedIDs))
			a.mu.Lock()
			for i, id := range cachedIDs {
				contentIDs[i] = fs.ID(id)
				a.known[backend.ID(id)] = true
			}
			a.mu.Unlock()
			return contentIDs, 0, false, false, nil
		}
	}

	f, err := os.Open(absPath)
	if err != nil {
		return nil, 0, false, false, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	ch := chunker.New(f,
		uint(cfg.ChunkerParams.MinSize),
		uint(cfg.ChunkerParams.AvgSize),
		uint(cfg.ChunkerParams.MaxSize),
	)

	var contentIDs []fs.ID
	var bytesRead uint64
	isNew := false

	for {
		chunk, err := ch.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, false, false, fmt.Errorf("chunk: %w", err)
		}

		bytesRead += uint64(chunk.Length)

		// Compute content-addressable blob ID.
		rawID := crypto.BlobID(mk.IDKey, chunk.Data)
		blobID := backend.ID(rawID)
		contentIDs = append(contentIDs, fs.ID(blobID))

		// Check if blob already exists (in repo index or this session).
		a.mu.Lock()
		known := a.known[blobID]
		a.mu.Unlock()
		if known {
			continue
		}
		if a.fc != nil && a.fc.IsKnownChunk(blobID) {
			a.mu.Lock()
			a.known[blobID] = true
			a.mu.Unlock()
			continue
		}
		if existing, _ := a.repo.LookupBlob(blobID); existing != nil {
			a.mu.Lock()
			a.known[blobID] = true
			a.mu.Unlock()
			continue
		}

		// New blob: compress (outside lock — expensive operation).
		compressed, err := compress.Compress(chunk.Data)
		if err != nil {
			return nil, 0, false, false, fmt.Errorf("compress: %w", err)
		}

		// Add to pack (under lock).
		a.mu.Lock()
		if !a.known[blobID] {
			if err := a.addBlob(blobID, repository.DataBlob, compressed, len(chunk.Data)); err != nil {
				a.mu.Unlock()
				return nil, 0, false, false, err
			}
			a.summary.BytesAdded += uint64(len(compressed))
		}
		a.mu.Unlock()

		// Record in known_chunks cache.
		if a.fc != nil {
			a.fc.AddKnownChunk(blobID, len(compressed))
		}

		isNew = true
	}

	// Detect file changed during read: re-stat and compare mtime/size.
	fi, statErr := os.Lstat(absPath)
	if statErr == nil {
		if fi.Size() != int64(node.Size) || fi.ModTime().UnixNano() != node.MtimeNs {
			return nil, 0, true, false, nil
		}
	}

	// Update files cache with this file's chunk IDs.
	if a.fc != nil {
		cacheIDs := make([][32]byte, len(contentIDs))
		for i, id := range contentIDs {
			cacheIDs[i] = [32]byte(id)
		}
		a.fc.UpdateFile(pathHash, node.Inode, node.Size, node.MtimeNs, cacheIDs)
	}

	return contentIDs, bytesRead, false, isNew, nil
}

// addBlob adds a compressed blob to the current pack, flushing if the pack
// exceeds the size threshold.
func (a *Archiver) addBlob(id backend.ID, blobType repository.BlobType, compressedData []byte, uncompressedLen int) error {
	if a.pw == nil {
		a.pw = repository.NewPackWriter(a.repo.Key().EncryptKey)
	}

	if err := a.pw.AddBlob(id, blobType, compressedData, uncompressedLen); err != nil {
		return fmt.Errorf("archiver: add blob to pack: %w", err)
	}

	a.known[id] = true

	if a.pw.Size() >= packSizeThreshold {
		return a.flushPack()
	}
	return nil
}

// flushPack finalizes the current pack, saves it to the repository, and
// records its blobs in the index.
func (a *Archiver) flushPack() error {
	if a.pw == nil || a.pw.Count() == 0 {
		return nil
	}

	packID, data, err := a.pw.Finalize()
	if err != nil {
		return fmt.Errorf("archiver: finalize pack: %w", err)
	}

	if _, err := a.repo.SavePack(data); err != nil {
		return fmt.Errorf("archiver: save pack: %w", err)
	}

	// Read back the header to get precise blob metadata.
	blobs, err := repository.ReadPackHeader(a.repo.Key().EncryptKey, data)
	if err != nil {
		return fmt.Errorf("archiver: read pack header: %w", err)
	}

	a.index.Packs = append(a.index.Packs, repository.IndexPack{
		PackID: packID,
		Blobs:  blobs,
	})

	a.pw = nil
	return nil
}

// saveSystemProfile captures the host's storage and boot layout and stores it
// as a data blob, returning the blob ID to record in the snapshot.
//
// Returns the zero ID when capture is disabled. Capture itself is best-effort:
// sysprofile records what it could not read as warnings inside the profile
// rather than failing, so an error here means the blob could not be written.
func (a *Archiver) saveSystemProfile() (repository.ID, error) {
	if !a.captureSystemProfile {
		return repository.ID{}, nil
	}

	profile, err := sysprofile.Capture(context.Background())
	if err != nil {
		return repository.ID{}, fmt.Errorf("archiver: capture system profile: %w", err)
	}

	blobID, compressedData, uncompressedLen, err := repository.EncodeSystemProfile(
		a.repo.Key().IDKey, profile)
	if err != nil {
		return repository.ID{}, err
	}

	// Identical layouts across snapshots dedup to a single blob.
	if a.known[backend.ID(blobID)] {
		return blobID, nil
	}
	if existing, _ := a.repo.LookupBlob(backend.ID(blobID)); existing != nil {
		a.known[backend.ID(blobID)] = true
		return blobID, nil
	}

	if err := a.addBlob(backend.ID(blobID), repository.DataBlob, compressedData, uncompressedLen); err != nil {
		return repository.ID{}, err
	}
	return blobID, nil
}
