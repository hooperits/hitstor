//go:build windows

package agent

import (
	"os"
	"syscall"
)

const flagSequentialScan = 0x08000000 // FILE_FLAG_SEQUENTIAL_SCAN

// openFileForBackup opens a file for reading using Windows backup semantics.
// When the process has SE_BACKUP_NAME privilege (e.g. running as SYSTEM),
// FILE_FLAG_BACKUP_SEMANTICS bypasses normal ACL checks and sharing-mode
// conflicts, allowing reads of files that os.Open cannot access.
func openFileForBackup(path string) (*os.File, error) {
	pathp, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(
		pathp,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS|flagSequentialScan,
		0,
	)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}
