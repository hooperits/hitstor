//go:build windows

package fs

import (
	"os"
	"syscall"
)

// isReparsePoint returns true if the file is a Windows reparse point
// (junction, symlink, mount point, etc.). These must be skipped during
// directory traversal to avoid infinite loops from NTFS built-in junctions
// like C:\Users\All Users → C:\ProgramData.
func isReparsePoint(fi os.FileInfo) bool {
	wdata, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return false
	}
	return wdata.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
