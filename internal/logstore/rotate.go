package logstore

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type rotatingFile struct {
	mu     sync.Mutex
	cfg    Config
	file   *os.File
	closed bool
}

func newRotatingFile(cfg Config) (*rotatingFile, error) {
	r := &rotatingFile{cfg: cfg}
	if !cfg.FileEnabled {
		return r, nil
	}
	if err := os.MkdirAll(filepath.Dir(cfg.FilePath), 0o750); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, err
	}
	r.file = f
	return r, nil
}

func (r *rotatingFile) close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	r.closed = true
	return err
}

func (r *rotatingFile) writeLine(line []byte) error {
	if !r.cfg.FileEnabled {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("logstore: rotating file closed")
	}
	if r.file == nil {
		f, err := os.OpenFile(r.cfg.FilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
		if err != nil {
			return err
		}
		r.file = f
	}
	if err := r.rotateIfNeededLocked(); err != nil {
		return err
	}
	if _, err := r.file.Write(line); err != nil {
		return err
	}
	_, err := r.file.Write([]byte("\n"))
	return err
}

func (r *rotatingFile) rotateIfNeededLocked() error {
	maxBytes := int64(r.cfg.RotateMaxMB) * 1024 * 1024
	if maxBytes <= 0 || r.file == nil {
		return nil
	}
	info, err := r.file.Stat()
	if err != nil {
		return err
	}
	if info.Size() < maxBytes {
		return nil
	}
	if err := r.file.Close(); err != nil {
		return err
	}
	r.file = nil

	base := r.cfg.FilePath
	maxFiles := r.cfg.RotateMaxFiles
	if maxFiles < 1 {
		maxFiles = 1
	}
	oldest := base + fmt.Sprintf(".%d", maxFiles)
	_ = os.Remove(oldest)
	for i := maxFiles - 1; i >= 1; i-- {
		src := base + fmt.Sprintf(".%d", i)
		dst := base + fmt.Sprintf(".%d", i+1)
		if _, err := os.Stat(src); err == nil {
			_ = os.Rename(src, dst)
		}
	}
	if err := os.Rename(base, base+".1"); err != nil {
		return err
	}
	f, err := os.OpenFile(base, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	r.file = f
	return nil
}

// RotatedPaths returns active and rotated log file paths that exist.
func RotatedPaths(cfg Config) []string {
	if !cfg.FileEnabled || cfg.FilePath == "" {
		return nil
	}
	var paths []string
	if _, err := os.Stat(cfg.FilePath); err == nil {
		paths = append(paths, cfg.FilePath)
	}
	for i := 1; i <= cfg.RotateMaxFiles; i++ {
		p := cfg.FilePath + fmt.Sprintf(".%d", i)
		if _, err := os.Stat(p); err == nil {
			paths = append(paths, p)
		}
	}
	return paths
}
