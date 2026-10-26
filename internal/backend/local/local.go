package local

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/hooperits/hitstor/internal/backend"
)

// Local implements the backend.Backend interface using the local filesystem.
type Local struct {
	basePath   string
	syncWrites bool
}

// Compile-time check that Local implements backend.Backend.
var _ backend.Backend = (*Local)(nil)

// New creates a new local backend rooted at basePath.
func New(basePath string) (*Local, error) {
	return NewWithOptions(basePath, Options{})
}

// NewWithOptions creates a local backend with the given options.
func NewWithOptions(basePath string, opts Options) (*Local, error) {
	sync := opts.SyncWrites || os.Getenv("HITSTOR_SYNC_WRITES") == "1"
	return &Local{basePath: basePath, syncWrites: sync}, nil
}

// SetSyncWrites enables fsync after atomic renames.
func (l *Local) SetSyncWrites(enabled bool) {
	l.syncWrites = enabled || os.Getenv("HITSTOR_SYNC_WRITES") == "1"
}

// SyncWritesEnabled reports whether fsync is active for this backend.
func (l *Local) SyncWritesEnabled() bool {
	return l.syncWrites
}

// BasePath returns the root directory of the local backend.
func (l *Local) BasePath() string {
	return l.basePath
}

// path returns the filesystem path for a given file type and ID.
func (l *Local) path(ft backend.FileType, id backend.ID) string {
	idHex := id.String()
	switch ft {
	case backend.DataFile:
		return filepath.Join(l.basePath, "data", idHex[:2], idHex)
	case backend.ConfigFile:
		return filepath.Join(l.basePath, "config")
	default:
		return filepath.Join(l.basePath, ft.String(), idHex)
	}
}

// Save stores data from rd under the given file type and ID.
// It writes to a temporary file first, then atomically renames it
// to the final path.
func (l *Local) Save(ctx context.Context, ft backend.FileType, id backend.ID, rd io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	p := l.path(ft, id)

	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	// Write to a temporary file in the same directory so rename is atomic.
	tmp, err := os.CreateTemp(dir, "hitstor-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Clean up the temp file on any error.
	defer func() {
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
	}()

	if err := copyWithContext(ctx, tmp, rd); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, p); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	if l.syncWrites {
		if f, err := os.Open(p); err == nil {
			_ = f.Sync()
			f.Close()
		}
		if d, err := os.Open(dir); err == nil {
			_ = d.Sync()
			d.Close()
		}
	}

	// Rename succeeded, prevent deferred cleanup.
	tmpPath = ""
	return nil
}

// LoadRange reads a byte range from a stored object.
func (l *Local) LoadRange(ctx context.Context, ft backend.FileType, id backend.ID, offset, length int64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p := l.path(ft, id)
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", p, err)
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		f.Close()
		return nil, fmt.Errorf("seek %s: %w", p, err)
	}
	return io.NopCloser(io.LimitReader(f, length)), nil
}

// Load retrieves the data for the given file type and ID.
// The caller must close the returned ReadCloser.
func (l *Local) Load(ctx context.Context, ft backend.FileType, id backend.ID) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p := l.path(ft, id)

	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", p, err)
	}

	return f, nil
}

// Stat returns metadata about the file for the given file type and ID.
func (l *Local) Stat(ctx context.Context, ft backend.FileType, id backend.ID) (backend.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return backend.FileInfo{}, err
	}
	p := l.path(ft, id)

	fi, err := os.Stat(p)
	if err != nil {
		return backend.FileInfo{}, fmt.Errorf("stat %s: %w", p, err)
	}

	return backend.FileInfo{Size: fi.Size()}, nil
}

// Remove deletes the file for the given file type and ID.
func (l *Local) Remove(ctx context.Context, ft backend.FileType, id backend.ID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p := l.path(ft, id)

	if err := os.Remove(p); err != nil {
		return fmt.Errorf("remove %s: %w", p, err)
	}

	return nil
}

// List returns all IDs stored for the given file type.
// For DataFile, it reads fan-out subdirectories. For other types,
// it reads a flat directory.
func (l *Local) List(ctx context.Context, ft backend.FileType) ([]backend.ID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ft == backend.ConfigFile {
		// ConfigFile uses a fixed filename, not ID-based names.
		// Check if the config file exists; if so, return a zero ID
		// as a sentinel. In practice, config is handled specially.
		return nil, nil
	}

	var ids []backend.ID

	if ft == backend.DataFile {
		// Data files use fan-out subdirectories: data/{first_byte_hex}/{id_hex}
		dataDir := filepath.Join(l.basePath, "data")
		prefixDirs, err := os.ReadDir(dataDir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("read data directory: %w", err)
		}

		for _, prefixDir := range prefixDirs {
			if !prefixDir.IsDir() {
				continue
			}

			subDir := filepath.Join(dataDir, prefixDir.Name())
			entries, err := os.ReadDir(subDir)
			if err != nil {
				return nil, fmt.Errorf("read subdirectory %s: %w", subDir, err)
			}

			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				id, err := parseID(entry.Name())
				if err != nil {
					continue // skip files with non-hex names
				}
				ids = append(ids, id)
			}
		}
	} else {
		// Other file types use a flat directory: {type}/{id_hex}
		dir := filepath.Join(l.basePath, ft.String())
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("read directory %s: %w", dir, err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			id, err := parseID(entry.Name())
			if err != nil {
				continue // skip files with non-hex names
			}
			ids = append(ids, id)
		}
	}

	return ids, nil
}

// Close is a no-op for the local backend.
func (l *Local) Close() error {
	return nil
}

// parseID parses a hex-encoded string into a backend.ID.
func parseID(s string) (backend.ID, error) {
	var id backend.ID

	b, err := hex.DecodeString(s)
	if err != nil {
		return id, err
	}

	if len(b) != len(id) {
		return id, fmt.Errorf("invalid ID length: got %d, want %d", len(b), len(id))
	}

	copy(id[:], b)
	return id, nil
}
