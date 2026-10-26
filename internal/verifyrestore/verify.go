// Package verifyrestore performs automated test-restores and checksum
// comparison for filesystem snapshots — the primary sellable trust signal
// that a backup can actually be restored.
package verifyrestore

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/chunker"
	"github.com/hooperits/hitstor/internal/compress"
	"github.com/hooperits/hitstor/internal/crypto"
	"github.com/hooperits/hitstor/internal/fs"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/restorer"
)

// Options controls how thorough a verification run is.
type Options struct {
	// MaxFiles limits how many regular files are restored and re-hashed.
	// Zero means no file-count limit.
	MaxFiles int
	// MaxBytes limits total bytes of regular-file content verified.
	// Zero means no byte limit.
	MaxBytes uint64
	// Parallel is passed to the restorer (0 = restorer default).
	Parallel int
	// TempDir is the parent directory for the throwaway restore root.
	// Empty uses the system temp directory.
	TempDir string
	// KeepTemp leaves the restore directory in place (for debugging).
	KeepTemp bool
}

// Mismatch describes one path that failed verification.
type Mismatch struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Result is the outcome of a verification run.
type Result struct {
	OK            bool          `json:"ok"`
	SnapshotID    string        `json:"snapshot_id"`
	FilesChecked  int           `json:"files_checked"`
	FilesSampled  int           `json:"files_sampled"`
	BytesChecked  uint64        `json:"bytes_checked"`
	SymlinksOK    int           `json:"symlinks_ok"`
	Mismatches    []Mismatch    `json:"mismatches,omitempty"`
	Duration      time.Duration `json:"duration"`
	Sampled       bool          `json:"sampled"`
	Error         string        `json:"error,omitempty"`
	RestoreTarget string        `json:"restore_target,omitempty"`
}

// Status strings persisted on BackupJob / shown in the dashboard.
const (
	StatusOK      = "ok"
	StatusFailed  = "failed"
	StatusError   = "error"
	StatusSkipped = "skipped"
)

// DefaultMaxFiles is the post-backup sample size used by the server.
const DefaultMaxFiles = 50

// DefaultMaxBytes is the post-backup sample budget used by the server (~100 MiB).
const DefaultMaxBytes = 100 * 1024 * 1024

type fileItem struct {
	relPath string
	node    fs.Node
}

type tree struct {
	Nodes []fs.Node `json:"nodes"`
}

// Verify restores a snapshot (or a deterministic sample of its files) to a
// temporary directory, re-chunks each restored file with the repository
// chunker parameters, and compares content blob IDs to the snapshot tree.
func Verify(repo *repository.Repository, snapID backend.ID, opts Options) Result {
	start := time.Now()
	res := Result{SnapshotID: snapID.String()}

	snap, err := repo.LoadSnapshot(snapID)
	if err != nil {
		res.Error = fmt.Sprintf("load snapshot: %v", err)
		res.Duration = time.Since(start)
		return res
	}

	root, err := loadTree(repo, snap.Tree)
	if err != nil {
		res.Error = fmt.Sprintf("load root tree: %v", err)
		res.Duration = time.Since(start)
		return res
	}

	var files []fileItem
	var symlinks []fileItem
	if err := walkCollect(repo, root, "", &files, &symlinks); err != nil {
		res.Error = err.Error()
		res.Duration = time.Since(start)
		return res
	}

	sort.Slice(files, func(i, j int) bool { return files[i].relPath < files[j].relPath })

	selected, sampled := selectSample(files, snapID, opts)
	res.Sampled = sampled
	res.FilesSampled = len(selected)

	parent := opts.TempDir
	if parent == "" {
		parent = os.TempDir()
	}
	restoreRoot, err := os.MkdirTemp(parent, "hitstor-verify-*")
	if err != nil {
		res.Error = fmt.Sprintf("create temp dir: %v", err)
		res.Duration = time.Since(start)
		return res
	}
	res.RestoreTarget = restoreRoot
	if !opts.KeepTemp {
		defer func() {
			if rmErr := os.RemoveAll(restoreRoot); rmErr != nil {
				slog.Warn("verifyrestore: cleanup temp dir", "path", restoreRoot, "error", rmErr)
			}
		}()
	}

	includes := make([]string, 0, len(selected)+len(symlinks))
	for _, f := range selected {
		includes = append(includes, f.relPath)
	}
	for _, s := range symlinks {
		if !sampled || symlinkNearSample(s.relPath, selected) {
			includes = append(includes, s.relPath)
		}
	}

	r := restorer.New(repo)
	if opts.Parallel > 0 {
		r.SetParallel(opts.Parallel)
	}
	if err := r.Restore(snapID, restoreRoot, includes); err != nil {
		res.Error = fmt.Sprintf("restore: %v", err)
		res.Duration = time.Since(start)
		return res
	}

	mk := repo.Key()
	cfg := repo.Cfg()
	minSz := uint(cfg.ChunkerParams.MinSize)
	avgSz := uint(cfg.ChunkerParams.AvgSize)
	maxSz := uint(cfg.ChunkerParams.MaxSize)

	for _, item := range selected {
		abs := filepath.Join(restoreRoot, filepath.FromSlash(item.relPath))
		if err := verifyFileContent(abs, item, mk.IDKey, minSz, avgSz, maxSz); err != nil {
			res.Mismatches = append(res.Mismatches, Mismatch{Path: item.relPath, Reason: err.Error()})
			continue
		}
		res.FilesChecked++
		res.BytesChecked += item.node.Size
	}

	for _, item := range symlinks {
		if sampled && !symlinkNearSample(item.relPath, selected) {
			continue
		}
		abs := filepath.Join(restoreRoot, filepath.FromSlash(item.relPath))
		target, err := os.Readlink(abs)
		if err != nil {
			res.Mismatches = append(res.Mismatches, Mismatch{Path: item.relPath, Reason: fmt.Sprintf("read symlink: %v", err)})
			continue
		}
		if target != item.node.LinkTarget {
			res.Mismatches = append(res.Mismatches, Mismatch{
				Path:   item.relPath,
				Reason: fmt.Sprintf("symlink target %q != expected %q", target, item.node.LinkTarget),
			})
			continue
		}
		res.SymlinksOK++
	}

	res.OK = len(res.Mismatches) == 0 && res.Error == ""
	res.Duration = time.Since(start)
	return res
}

func symlinkNearSample(rel string, selected []fileItem) bool {
	dir := path.Dir(rel)
	for _, f := range selected {
		if path.Dir(f.relPath) == dir {
			return true
		}
	}
	return false
}

func selectSample(files []fileItem, snapID backend.ID, opts Options) ([]fileItem, bool) {
	if opts.MaxFiles <= 0 && opts.MaxBytes == 0 {
		return files, false
	}
	if len(files) == 0 {
		return nil, false
	}

	order := make([]int, len(files))
	for i := range order {
		order[i] = i
	}
	state := binary.BigEndian.Uint64(snapID[:8])
	if state == 0 {
		state = 1
	}
	for i := len(order) - 1; i > 0; i-- {
		state = state*6364136223846793005 + 1
		j := int(state % uint64(i+1))
		order[i], order[j] = order[j], order[i]
	}

	var out []fileItem
	var bytes uint64
	for _, idx := range order {
		f := files[idx]
		if opts.MaxFiles > 0 && len(out) >= opts.MaxFiles {
			break
		}
		if opts.MaxBytes > 0 && bytes > 0 && bytes+f.node.Size > opts.MaxBytes {
			if f.node.Size > opts.MaxBytes && len(out) == 0 {
				out = append(out, f)
				bytes += f.node.Size
				break
			}
			continue
		}
		out = append(out, f)
		bytes += f.node.Size
	}
	return out, len(out) < len(files)
}

func verifyFileContent(abs string, item fileItem, idKey [32]byte, min, avg, max uint) error {
	fi, err := os.Lstat(abs)
	if err != nil {
		return fmt.Errorf("stat restored file: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("expected regular file, got mode %v", fi.Mode())
	}
	if uint64(fi.Size()) != item.node.Size {
		return fmt.Errorf("size %d != expected %d", fi.Size(), item.node.Size)
	}

	f, err := os.Open(abs)
	if err != nil {
		return fmt.Errorf("open restored file: %w", err)
	}
	defer f.Close()

	if item.node.Size == 0 {
		if len(item.node.Content) != 0 {
			return fmt.Errorf("empty file has %d content blobs", len(item.node.Content))
		}
		return nil
	}

	ch := chunker.New(f, min, avg, max)
	var got []fs.ID
	for {
		chunk, err := ch.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("chunk: %w", err)
		}
		raw := crypto.BlobID(idKey, chunk.Data)
		got = append(got, fs.ID(raw))
	}

	if len(got) != len(item.node.Content) {
		return fmt.Errorf("content chunks %d != expected %d", len(got), len(item.node.Content))
	}
	for i := range got {
		if got[i] != item.node.Content[i] {
			return fmt.Errorf("content chunk %d mismatch (got %s want %s)",
				i, shortID(got[i]), shortID(item.node.Content[i]))
		}
	}
	return nil
}

func shortID(id fs.ID) string {
	s := id.String()
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func walkCollect(repo *repository.Repository, t *tree, relPrefix string, files, symlinks *[]fileItem) error {
	for _, node := range t.Nodes {
		relPath := node.Name
		if relPrefix != "" {
			relPath = relPrefix + "/" + node.Name
		}
		switch node.Type {
		case fs.NodeTypeDir:
			sub, err := loadTree(repo, backend.ID(node.Subtree))
			if err != nil {
				return fmt.Errorf("load subtree %s: %w", relPath, err)
			}
			if err := walkCollect(repo, sub, relPath, files, symlinks); err != nil {
				return err
			}
		case fs.NodeTypeFile:
			*files = append(*files, fileItem{relPath: relPath, node: node})
		case fs.NodeTypeSymlink:
			*symlinks = append(*symlinks, fileItem{relPath: relPath, node: node})
		}
	}
	return nil
}

func loadTree(repo *repository.Repository, treeID backend.ID) (*tree, error) {
	blob, packID := repo.LookupBlob(treeID)
	if blob == nil {
		return nil, fmt.Errorf("tree blob %s not found in index", treeID)
	}
	compressedData, err := repo.LoadBlob(*packID, *blob)
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
