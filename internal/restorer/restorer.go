// Package restorer reconstructs files and directories from a HitStor backup
// snapshot, writing them to a target path on the local filesystem.
package restorer

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/compress"
	"github.com/hooperits/hitstor/internal/fs"
	"github.com/hooperits/hitstor/internal/repository"
)

// tree is a local copy of archiver.Tree to avoid importing the archiver package.
type tree struct {
	Nodes []fs.Node `json:"nodes"`
}

// Restorer restores a snapshot from a repository to the local filesystem.
type Restorer struct {
	repo          *repository.Repository
	FilesRestored uint64
	BytesRestored uint64
	// XattrsSkipped counts extended attributes that could not be reapplied,
	// typically because the restore is not running as root or the target
	// filesystem lacks support. Surfaced so a restore can be reported as
	// complete-but-degraded rather than silently lossy.
	XattrsSkipped      uint64
	parallel           int
	progressFn         func(path string, size uint64)
	treeWalkCompleteFn func(files int, totalBytes uint64)
}

// New creates a new Restorer backed by the given repository.
func New(repo *repository.Repository) *Restorer {
	return &Restorer{
		repo: repo,
	}
}

// SetParallel sets the number of parallel workers for file restoration.
func (r *Restorer) SetParallel(n int) {
	r.parallel = n
}

// SetProgress sets a callback invoked for each file restored.
func (r *Restorer) SetProgress(fn func(path string, size uint64)) {
	r.progressFn = fn
}

// SetTreeWalkComplete sets a callback invoked when the tree walk finishes,
// reporting the total number of files and bytes to restore.
func (r *Restorer) SetTreeWalkComplete(fn func(files int, totalBytes uint64)) {
	r.treeWalkCompleteFn = fn
}

// Restore restores a snapshot to the target path.
// If includes is non-empty, only paths matching the includes are restored.
func (r *Restorer) Restore(snapID backend.ID, targetPath string, includes []string) error {
	slog.Info("restore started", "snapshot", snapID.String()[:8], "target", targetPath)

	snap, err := r.repo.LoadSnapshot(snapID)
	if err != nil {
		return fmt.Errorf("restorer: load snapshot: %w", err)
	}

	rootTree, err := r.loadTree(snap.Tree)
	if err != nil {
		return fmt.Errorf("restorer: load root tree: %w", err)
	}

	// Create the target directory if it does not exist.
	if err := os.MkdirAll(targetPath, 0700); err != nil {
		return fmt.Errorf("restorer: create target dir: %w", err)
	}

	// Phase 1: Walk tree sequentially — create directories, collect file work items.
	type dirMeta struct {
		path    string
		mode    os.FileMode
		mtimeNs int64
		atimeNs int64
	}
	var dirs []dirMeta

	type fileWork struct {
		nodePath string
		node     fs.Node
	}
	var files []fileWork

	var walkTree func(t *tree, basePath string, relPrefix string) error
	walkTree = func(t *tree, basePath string, relPrefix string) error {
		for _, node := range t.Nodes {
			nodePath := path.Join(basePath, node.Name)
			relPath := node.Name
			if relPrefix != "" {
				relPath = relPrefix + "/" + node.Name
			}

			if !r.shouldProcess(relPath, includes) {
				continue
			}

			switch node.Type {
			case fs.NodeTypeDir:
				if err := os.MkdirAll(nodePath, os.FileMode(node.Mode)); err != nil {
					return fmt.Errorf("restorer: mkdir %s: %w", nodePath, err)
				}

				dirs = append(dirs, dirMeta{
					path:    nodePath,
					mode:    os.FileMode(node.Mode),
					mtimeNs: node.MtimeNs,
					atimeNs: node.AtimeNs,
				})

				r.setOwnership(nodePath, node.UID, node.GID)
				r.setXattrs(nodePath, node.Xattrs)

				subtreeID := backend.ID(node.Subtree)
				subTree, err := r.loadTree(subtreeID)
				if err != nil {
					return fmt.Errorf("restorer: load subtree for %s: %w", nodePath, err)
				}

				if err := walkTree(subTree, nodePath, relPath); err != nil {
					return err
				}

			case fs.NodeTypeFile:
				files = append(files, fileWork{nodePath: nodePath, node: node})

			case fs.NodeTypeSymlink:
				os.Remove(nodePath)

				if err := os.Symlink(node.LinkTarget, nodePath); err != nil {
					return fmt.Errorf("restorer: symlink %s -> %s: %w", nodePath, node.LinkTarget, err)
				}

				r.setOwnership(nodePath, node.UID, node.GID)
				r.setXattrs(nodePath, node.Xattrs)

			default:
				return fmt.Errorf("restorer: unsupported node type %q for %s", node.Type, nodePath)
			}
		}
		return nil
	}

	if err := walkTree(rootTree, targetPath, ""); err != nil {
		return err
	}

	// Report tree walk completion.
	if r.treeWalkCompleteFn != nil {
		var totalBytes uint64
		for _, fw := range files {
			totalBytes += fw.node.Size
		}
		r.treeWalkCompleteFn(len(files), totalBytes)
	}

	// Phase 2: Restore files in parallel.
	workers := r.parallel
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
				slog.Debug("restoring file", "path", fw.nodePath)
				if err := r.restoreFile(fw.nodePath, fw.node); err != nil {
					errOnce.Do(func() { firstErr = fmt.Errorf("restorer: restore file %s: %w", fw.nodePath, err) })
					slog.Warn("file restore error, skipping", "path", fw.nodePath, "error", err)
					continue
				}

				if err := os.Chmod(fw.nodePath, os.FileMode(fw.node.Mode)); err != nil {
					slog.Warn("chmod error", "path", fw.nodePath, "error", err)
				}

				r.setOwnership(fw.nodePath, fw.node.UID, fw.node.GID)
				r.setXattrs(fw.nodePath, fw.node.Xattrs)

				mtime := time.Unix(0, fw.node.MtimeNs)
				atime := time.Unix(0, fw.node.AtimeNs)
				if err := os.Chtimes(fw.nodePath, atime, mtime); err != nil {
					slog.Warn("chtimes error", "path", fw.nodePath, "error", err)
				}

				atomic.AddUint64(&r.FilesRestored, 1)

				if r.progressFn != nil {
					r.progressFn(fw.nodePath, fw.node.Size)
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
		return firstErr
	}

	// Phase 3: Set directory timestamps in reverse order (deepest first).
	for i := len(dirs) - 1; i >= 0; i-- {
		d := dirs[i]
		mtime := time.Unix(0, d.mtimeNs)
		atime := time.Unix(0, d.atimeNs)
		if err := os.Chtimes(d.path, atime, mtime); err != nil {
			return fmt.Errorf("restorer: chtimes dir %s: %w", d.path, err)
		}
		if err := os.Chmod(d.path, d.mode); err != nil {
			return fmt.Errorf("restorer: chmod dir %s: %w", d.path, err)
		}
	}

	slog.Info("restore complete",
		"files", r.FilesRestored,
		"bytes", r.BytesRestored,
	)

	return nil
}

// loadTree loads a tree blob from the repository: LookupBlob -> LoadBlob -> Decompress -> JSON unmarshal.
func (r *Restorer) loadTree(treeID backend.ID) (*tree, error) {
	blob, packID := r.repo.LookupBlob(treeID)
	if blob == nil {
		return nil, fmt.Errorf("tree blob %s not found in index", treeID)
	}

	compressedData, err := r.repo.LoadBlob(*packID, *blob)
	if err != nil {
		return nil, fmt.Errorf("load tree blob %s: %w", treeID, err)
	}

	jsonData, err := compress.Decompress(compressedData)
	if err != nil {
		return nil, fmt.Errorf("decompress tree blob %s: %w", treeID, err)
	}

	var t tree
	if err := json.Unmarshal(jsonData, &t); err != nil {
		return nil, fmt.Errorf("unmarshal tree blob %s: %w", treeID, err)
	}

	return &t, nil
}

// restoreFile loads all content blobs for a file node, decompresses them, and
// writes the concatenated data to the given path.
func (r *Restorer) restoreFile(filePath string, node fs.Node) error {
	// Ensure parent directory exists.
	if err := os.MkdirAll(path.Dir(filePath), 0700); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	for _, contentID := range node.Content {
		blobID := backend.ID(contentID)
		blob, packID := r.repo.LookupBlob(blobID)
		if blob == nil {
			return fmt.Errorf("content blob %s not found in index", blobID)
		}

		compressedData, err := r.repo.LoadBlob(*packID, *blob)
		if err != nil {
			return fmt.Errorf("load content blob %s: %w", blobID, err)
		}

		data, err := compress.Decompress(compressedData)
		if err != nil {
			return fmt.Errorf("decompress content blob %s: %w", blobID, err)
		}

		if _, err := f.Write(data); err != nil {
			return fmt.Errorf("write data: %w", err)
		}

		atomic.AddUint64(&r.BytesRestored, uint64(len(data)))
	}

	return nil
}

// shouldProcess determines whether a node at relPath should be processed given
// the include filter list.
//
// If includes is empty, all nodes are processed.
// Otherwise, a node is processed if:
//   - relPath starts with any include (the node is within an included path), OR
//   - Any include starts with relPath+"/" (the node is a parent directory of an included path)
func (r *Restorer) shouldProcess(relPath string, includes []string) bool {
	if len(includes) == 0 {
		return true
	}

	for _, inc := range includes {
		// The node is exactly the included path.
		if relPath == inc {
			return true
		}
		// The node is within an included path (relPath is a child of inc).
		if strings.HasPrefix(relPath, inc+"/") {
			return true
		}
		// The node is a parent of an included path (inc starts with relPath/).
		if strings.HasPrefix(inc, relPath+"/") {
			return true
		}
	}

	return false
}

// setXattrs restores extended attributes recorded in the snapshot.
//
// It runs after ownership is set, because chown clears security.capability;
// applying capabilities first would silently lose them. Attributes the target
// filesystem or the current privilege level rejects are logged and skipped —
// metadata that cannot be reproduced must never fail a restore.
func (r *Restorer) setXattrs(filePath string, attrs map[string][]byte) {
	if len(attrs) == 0 {
		return
	}
	skipped, err := fs.WriteXattrsReport(filePath, attrs)
	if err != nil {
		slog.Warn("xattr restore error", "path", filePath, "error", err)
		return
	}
	if len(skipped) > 0 {
		slog.Debug("xattrs skipped", "path", filePath, "attrs", skipped)
		atomic.AddUint64(&r.XattrsSkipped, uint64(len(skipped)))
	}
}

// setOwnership sets the ownership of the given path. EPERM errors are
// silently ignored since changing ownership requires root privileges.
func (r *Restorer) setOwnership(filePath string, uid, gid uint32) {
	err := os.Lchown(filePath, int(uid), int(gid))
	if err != nil {
		// Ignore EPERM - we may not be running as root.
		if !errors.Is(err, syscall.EPERM) {
			// Log or ignore other errors too - ownership is best-effort.
			_ = err
		}
	}
}
