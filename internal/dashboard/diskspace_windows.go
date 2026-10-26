//go:build windows

package dashboard

import (
	"fmt"
	"syscall"
	"unsafe"
)

// getDiskSpace returns total and free bytes for the filesystem at path.
func getDiskSpace(path string) (total, free uint64, err error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetDiskFreeSpaceExW")

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, fmt.Errorf("dashboard: disk space: %w", err)
	}

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	r1, _, e1 := proc.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if r1 == 0 {
		return 0, 0, fmt.Errorf("dashboard: disk space: %w", e1)
	}

	return totalBytes, freeBytesAvailable, nil
}
