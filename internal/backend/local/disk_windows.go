//go:build windows

package local

import (
	"fmt"
	"syscall"
	"unsafe"
)

// DiskUsage returns the total and free bytes on the filesystem containing the backend.
func (l *Local) DiskUsage() (total, free uint64, err error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	pathPtr, err := syscall.UTF16PtrFromString(l.basePath)
	if err != nil {
		return 0, 0, fmt.Errorf("local: disk usage: %w", err)
	}

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	ret, _, callErr := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if ret == 0 {
		return 0, 0, fmt.Errorf("local: disk usage: %w", callErr)
	}

	return totalBytes, freeBytesAvailable, nil
}
