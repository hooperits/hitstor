//go:build !windows

package local

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const exclusiveFlockName = ".exclusive"

// AcquireFlock obtains a non-blocking exclusive flock on locks/.exclusive.
func (l *Local) AcquireFlock() (*os.File, error) {
	dir := filepath.Join(l.basePath, "locks")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create locks dir: %w", err)
	}
	path := filepath.Join(dir, exclusiveFlockName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open flock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("acquire flock: %w", err)
	}
	return f, nil
}
