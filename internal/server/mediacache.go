package server

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrNoMedia reports that a host has no cached recovery media, which is an
// ordinary state — most hosts never have media built — and not a failure.
var ErrNoMedia = errors.New("server: no recovery media cached for this host")

// MediaInfo describes one cached recovery image.
type MediaInfo struct {
	Host      string    `json:"host"`
	Path      string    `json:"path"`
	SizeBytes int64     `json:"size_bytes"`
	BuiltAt   time.Time `json:"built_at"`
}

// MediaCache holds one bare-metal recovery image per host.
//
// The image is cached server-side rather than fetched on demand because
// bare-metal restore exists for when the source host is unavailable or
// destroyed. A design that rebuilt the ISO from its own host at download time
// would break the feature in exactly the scenario it exists for.
//
// One image per host, overwritten on rebuild: images run to hundreds of
// megabytes, and only the newest layout is the one worth recovering to.
type MediaCache struct {
	dir string

	// mu serializes builds per host so two concurrent dashboard clicks cannot
	// interleave writes into the same temporary file.
	mu       sync.Mutex
	building map[string]bool
}

// NewMediaCache returns a cache rooted at dir. The directory is created lazily,
// on the first write, so a server that never builds media leaves no trace.
func NewMediaCache(dir string) *MediaCache {
	return &MediaCache{dir: dir, building: map[string]bool{}}
}

// Dir returns the cache's root directory.
func (c *MediaCache) Dir() string { return c.dir }

// isoPath resolves a host's cached image path, refusing any name that would
// place it outside the cache.
//
// Host names arrive from the agent registry, but treating them as trusted path
// components is exactly the assumption that turns a rename into a file write
// anywhere on the server.
func (c *MediaCache) isoPath(host string) (string, error) {
	if host == "" {
		return "", fmt.Errorf("server: recovery media: host name is required")
	}
	if host != filepath.Base(host) ||
		strings.ContainsAny(host, `/\`) ||
		host == "." || host == ".." {
		return "", fmt.Errorf("server: recovery media: %q is not a usable host name", host)
	}
	return filepath.Join(c.dir, host+".iso"), nil
}

// Get returns the cached image for a host, or ErrNoMedia.
func (c *MediaCache) Get(host string) (MediaInfo, error) {
	path, err := c.isoPath(host)
	if err != nil {
		return MediaInfo{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return MediaInfo{}, ErrNoMedia
		}
		return MediaInfo{}, fmt.Errorf("server: stat recovery media: %w", err)
	}
	return MediaInfo{
		Host:      host,
		Path:      path,
		SizeBytes: info.Size(),
		BuiltAt:   info.ModTime(),
	}, nil
}

// List returns every cached image, newest first.
func (c *MediaCache) List() []MediaInfo {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil
	}

	var out []MediaInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".iso") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, MediaInfo{
			Host:      strings.TrimSuffix(e.Name(), ".iso"),
			Path:      filepath.Join(c.dir, e.Name()),
			SizeBytes: info.Size(),
			BuiltAt:   info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BuiltAt.After(out[j].BuiltAt) })
	return out
}

// Remove deletes a host's cached image. Removing media that was never built is
// not an error: the caller wanted it gone, and it is.
func (c *MediaCache) Remove(host string) error {
	path, err := c.isoPath(host)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("server: remove recovery media: %w", err)
	}
	return nil
}

// Building reports whether a build is currently writing this host's image.
func (c *MediaCache) Building(host string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.building[host]
}

// MediaWriter receives a streamed image. Nothing replaces the host's existing
// media until Commit succeeds, so a build that dies partway leaves the
// operator with the image they already had.
type MediaWriter struct {
	cache   *MediaCache
	host    string
	final   string
	tmp     string
	file    *os.File
	written int64
	done    bool
}

// Writer begins receiving a new image for a host.
func (c *MediaCache) Writer(host string) (*MediaWriter, error) {
	final, err := c.isoPath(host)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	if c.building[host] {
		c.mu.Unlock()
		return nil, fmt.Errorf("server: recovery media for %q is already being built", host)
	}
	c.building[host] = true
	c.mu.Unlock()

	release := func() {
		c.mu.Lock()
		delete(c.building, host)
		c.mu.Unlock()
	}

	if err := os.MkdirAll(c.dir, 0o750); err != nil {
		release()
		return nil, fmt.Errorf("server: create recovery media cache: %w", err)
	}
	// Same directory as the final path, so Commit is an atomic rename rather
	// than a cross-filesystem copy of several gigabytes.
	f, err := os.CreateTemp(c.dir, host+".iso.partial-*")
	if err != nil {
		release()
		return nil, fmt.Errorf("server: create recovery media file: %w", err)
	}

	return &MediaWriter{cache: c, host: host, final: final, tmp: f.Name(), file: f}, nil
}

// Write appends to the incoming image.
func (w *MediaWriter) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	w.written += int64(n)
	return n, err
}

var _ io.Writer = (*MediaWriter)(nil)

// Written reports how many bytes have arrived so far, for progress reporting.
func (w *MediaWriter) Written() int64 { return w.written }

// Commit publishes the received image as this host's recovery media,
// replacing any previous one.
func (w *MediaWriter) Commit() (MediaInfo, error) {
	defer w.release()

	if err := w.file.Sync(); err != nil {
		return MediaInfo{}, fmt.Errorf("server: flush recovery media: %w", err)
	}
	if err := w.file.Close(); err != nil {
		return MediaInfo{}, fmt.Errorf("server: close recovery media: %w", err)
	}
	if err := os.Chmod(w.tmp, 0o640); err != nil {
		return MediaInfo{}, fmt.Errorf("server: set recovery media mode: %w", err)
	}
	if err := os.Rename(w.tmp, w.final); err != nil {
		os.Remove(w.tmp)
		return MediaInfo{}, fmt.Errorf("server: publish recovery media: %w", err)
	}

	built := time.Now()
	// The rename preserves the temp file's mtime; stamp it so media age
	// reflects when this build finished.
	_ = os.Chtimes(w.final, built, built)

	return MediaInfo{
		Host:      w.host,
		Path:      w.final,
		SizeBytes: w.written,
		BuiltAt:   built,
	}, nil
}

// Abort discards the partial image, leaving any previously cached media in
// place.
func (w *MediaWriter) Abort() error {
	defer w.release()

	w.file.Close()
	if err := os.Remove(w.tmp); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("server: discard partial recovery media: %w", err)
	}
	return nil
}

// release clears the per-host build lock exactly once.
func (w *MediaWriter) release() {
	if w.done {
		return
	}
	w.done = true
	w.cache.mu.Lock()
	delete(w.cache.building, w.host)
	w.cache.mu.Unlock()
}
