//go:build !windows

package agent

import (
	"errors"
	"log/slog"
	"os"
	"syscall"
)

// setOwnership sets the ownership of the given path. EPERM errors are
// silently ignored since changing ownership requires root privileges.
func setOwnership(filePath string, uid, gid uint32) {
	err := os.Lchown(filePath, int(uid), int(gid))
	if err != nil && !errors.Is(err, syscall.EPERM) {
		slog.Warn("restore: chown", "path", filePath, "error", err)
	}
}
