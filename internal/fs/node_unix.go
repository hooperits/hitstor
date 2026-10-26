//go:build !windows

package fs

import (
	"fmt"
	"os"
	"syscall"
)

// NodeFromFileInfo creates a Node from a path and its os.FileInfo (obtained via os.Lstat).
// On Unix, it extracts UID, GID, inode, link count, atime, and ctime from syscall.Stat_t.
func NodeFromFileInfo(path string, fi os.FileInfo) (Node, error) {
	node, err := nodeFromFileInfoCommon(path, fi)
	if err != nil {
		return Node{}, err
	}

	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return Node{}, fmt.Errorf("fs: unable to get syscall.Stat_t for %s", path)
	}

	node.UID = stat.Uid
	node.GID = stat.Gid
	node.Inode = stat.Ino
	node.Links = uint64(stat.Nlink)
	node.AtimeNs = stat.Atim.Nano()
	node.CtimeNs = stat.Ctim.Nano()

	return node, nil
}
