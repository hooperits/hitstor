//go:build !windows

package dashboard

import (
	"fmt"
	"syscall"
)

// getDiskSpace returns total and free bytes for the filesystem at path.
func getDiskSpace(path string) (total, free uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("dashboard: disk space: %w", err)
	}
	total = stat.Blocks * uint64(stat.Bsize)
	free = stat.Bavail * uint64(stat.Bsize)
	return total, free, nil
}
