//go:build !windows

package local

import (
	"fmt"
	"syscall"
)

// DiskUsage returns the total and free bytes on the filesystem containing the backend.
func (l *Local) DiskUsage() (total, free uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(l.basePath, &stat); err != nil {
		return 0, 0, fmt.Errorf("local: disk usage: %w", err)
	}
	total = stat.Blocks * uint64(stat.Bsize)
	free = stat.Bavail * uint64(stat.Bsize)
	return total, free, nil
}
