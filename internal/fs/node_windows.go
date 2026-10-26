//go:build windows

package fs

import (
	"fmt"
	"os"
	"syscall"
)

// NodeFromFileInfo creates a Node from a path and its os.FileInfo (obtained via os.Lstat).
// On Windows, UID and GID are always 0. Inode is emulated from the NTFS FileIndex
// via GetFileInformationByHandle. CTime represents creation time (not change time).
func NodeFromFileInfo(path string, fi os.FileInfo) (Node, error) {
	node, err := nodeFromFileInfoCommon(path, fi)
	if err != nil {
		return Node{}, err
	}

	node.UID = 0
	node.GID = 0

	// Extract atime and ctime from Win32FileAttributeData.
	if wdata, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		node.AtimeNs = wdata.LastAccessTime.Nanoseconds()
		node.CtimeNs = wdata.CreationTime.Nanoseconds()
	}

	// Get inode (FileIndex) and link count via GetFileInformationByHandle.
	// Skip for directories — the extra CreateFile syscall per directory is
	// unnecessary overhead and inode/links are only meaningful for files.
	if !fi.IsDir() {
		inode, links, err := getFileIDAndLinks(path)
		if err != nil {
			// Non-fatal: locked files or special paths may fail.
			node.Inode = 0
			node.Links = 1
		} else {
			node.Inode = inode
			node.Links = links
		}
	}

	return node, nil
}

// getFileIDAndLinks opens the file and calls GetFileInformationByHandle
// to retrieve the NTFS file index (synthetic inode) and hard link count.
func getFileIDAndLinks(path string) (inode uint64, links uint64, err error) {
	pathp, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}

	h, err := syscall.CreateFile(
		pathp,
		0, // no access needed for metadata query
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("fs: CreateFile %s: %w", path, err)
	}
	defer syscall.CloseHandle(h)

	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &info); err != nil {
		return 0, 0, fmt.Errorf("fs: GetFileInformationByHandle %s: %w", path, err)
	}

	inode = uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)
	links = uint64(info.NumberOfLinks)
	return inode, links, nil
}
