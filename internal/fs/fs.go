package fs

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Walk traverses the directory tree rooted at root, calling fn for each regular
// file, directory, and symlink encountered. Directories are emitted before their
// contents. It uses os.Lstat (not os.Stat) so symlinks are not followed.
//
// The excludes parameter accepts glob patterns matched against both the filename
// component and the full relative path (relative to root) using filepath.Match.
// Patterns ending with "/" cause entire directory subtrees to be skipped.
//
// Special files (devices, sockets, FIFOs, named pipes) are skipped with a
// debug warning. On Windows, NTFS junctions and reparse points are skipped to
// prevent infinite directory loops. A canonical-path visited set provides
// additional cycle detection as a safety net.
// WalkOptions controls what Walk captures beyond the base node metadata.
type WalkOptions struct {
	// Excludes are glob patterns matched as described on Walk.
	Excludes []string
	// Xattrs captures extended attributes into Node.Xattrs. It costs two extra
	// syscalls per entry, so it is opt-in and enabled for jobs that need
	// bare-metal-restore fidelity.
	Xattrs bool
}

func Walk(root string, excludes []string, fn func(path string, node Node) error) error {
	return WalkWithOptions(root, WalkOptions{Excludes: excludes}, fn)
}

// WalkWithOptions walks root like Walk, capturing the extras named in opts.
func WalkWithOptions(root string, opts WalkOptions, fn func(path string, node Node) error) error {
	excludes := opts.Excludes
	root = filepath.Clean(root)

	// Track visited directories by canonical path to detect cycles caused by
	// NTFS junctions (e.g. AppData\Local\Application Data → AppData\Local).
	visited := make(map[string]bool)

	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Non-fatal: skip inaccessible files/directories (permission denied,
			// locked files, etc.) instead of aborting the entire backup.
			slog.Warn("skipping inaccessible path", "path", path, "error", err)
			if info != nil && info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Compute relative path for exclude matching.
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("fs: rel path %s: %w", path, err)
		}

		// Skip the root directory itself (it will be "." as a relative path).
		// We don't emit it because the caller already knows about it.
		if relPath == "." {
			return nil
		}

		// Check exclude patterns.
		if shouldExclude(relPath, info.IsDir(), excludes) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip reparse points (Windows junctions/symlinks) to prevent infinite loops.
		if info.IsDir() && isReparsePoint(info) {
			slog.Debug("skipping reparse point", "path", path)
			return filepath.SkipDir
		}

		// Cycle detection: resolve canonical path for directories. This catches
		// loops that isReparsePoint might miss (e.g. if the type assertion fails).
		if info.IsDir() {
			canonical, cerr := filepath.EvalSymlinks(path)
			if cerr == nil {
				canonical = filepath.Clean(canonical)
				if visited[canonical] {
					slog.Warn("skipping directory cycle", "path", path, "target", canonical)
					return filepath.SkipDir
				}
				visited[canonical] = true
			}
		}

		// Skip special files (devices, sockets, FIFOs, named pipes).
		mode := info.Mode()
		if !mode.IsRegular() && !mode.IsDir() && mode&os.ModeSymlink == 0 {
			slog.Debug("skipping special file", "path", path, "mode", mode.Type().String())
			return nil
		}

		node, err := NodeFromFileInfo(path, info)
		if err != nil {
			slog.Warn("skipping file (metadata failed)", "path", path, "error", err)
			return nil
		}

		if opts.Xattrs {
			// Unreadable attributes must not cost us the file itself.
			attrs, xerr := ReadXattrs(path)
			if xerr != nil {
				slog.Debug("xattrs unavailable", "path", path, "error", xerr)
			} else if len(attrs) > 0 {
				node.Xattrs = attrs
			}
		}

		return fn(path, node)
	})
}

// shouldExclude checks whether a relative path matches any of the exclude patterns.
// For each pattern, it matches against both the filename component and the full
// relative path. Patterns ending with "/" are directory-only patterns.
func shouldExclude(relPath string, isDir bool, excludes []string) bool {
	name := filepath.Base(relPath)

	for _, pattern := range excludes {
		// Directory-only pattern (ends with /).
		if strings.HasSuffix(pattern, "/") {
			if !isDir {
				continue
			}
			// Strip trailing slash for matching.
			dirPattern := strings.TrimSuffix(pattern, "/")

			// Match against filename.
			if matched, _ := filepath.Match(dirPattern, name); matched {
				return true
			}
			// Match against full relative path.
			if matched, _ := filepath.Match(dirPattern, relPath); matched {
				return true
			}
			continue
		}

		// Regular pattern: match against filename.
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
		// Match against full relative path.
		if matched, _ := filepath.Match(pattern, relPath); matched {
			return true
		}
	}

	return false
}
