//go:build !windows

package fs

import "os"

// isReparsePoint is a no-op on non-Windows platforms.
func isReparsePoint(_ os.FileInfo) bool { return false }
