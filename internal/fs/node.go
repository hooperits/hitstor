// Package fs provides filesystem metadata extraction and directory tree walking
// for the HitStor encrypted backup tool.
package fs

import (
	"encoding/hex"
	"fmt"
	"os"
)

// NodeType represents the type of a filesystem node.
type NodeType string

const (
	// NodeTypeFile represents a regular file.
	NodeTypeFile NodeType = "file"
	// NodeTypeDir represents a directory.
	NodeTypeDir NodeType = "dir"
	// NodeTypeSymlink represents a symbolic link.
	NodeTypeSymlink NodeType = "symlink"
)

// ID is a 32-byte identifier (SHA-256 hash) used to reference content and subtrees.
type ID [32]byte

// IsZero returns true if the ID is all zeros.
func (id ID) IsZero() bool { return id == ID{} }

// String returns the hex-encoded representation of the ID.
func (id ID) String() string { return hex.EncodeToString(id[:]) }

// Node holds the metadata for a single filesystem entry (file, directory, or symlink).
type Node struct {
	Name       string   `json:"name"`
	Type       NodeType `json:"type"`
	Mode       uint32   `json:"mode"`
	UID        uint32   `json:"uid"`
	GID        uint32   `json:"gid"`
	MtimeNs    int64    `json:"mtime_ns"`
	AtimeNs    int64    `json:"atime_ns"`
	CtimeNs    int64    `json:"ctime_ns"`
	Size       uint64   `json:"size"`
	Inode      uint64   `json:"inode"`
	Links      uint64   `json:"links"`
	LinkTarget string   `json:"link_target,omitempty"`
	Content    []ID     `json:"content,omitempty"`
	Subtree    ID       `json:"subtree,omitempty"`

	// Xattrs holds extended attributes, keyed by attribute name. On Linux this
	// carries SELinux contexts (security.selinux), file capabilities
	// (security.capability), and POSIX ACLs (system.posix_acl_*) — the system
	// state a bare-metal restore needs to produce a bootable machine.
	//
	// Optional: snapshots taken before extended-attribute capture omit it, and
	// restore then behaves exactly as it did before.
	Xattrs map[string][]byte `json:"xattrs,omitempty"`
}

// nodeFromFileInfoCommon creates a Node with platform-independent fields.
// Platform-specific files (node_unix.go, node_windows.go) call this, then
// fill in UID, GID, Inode, Links, AtimeNs, and CtimeNs.
func nodeFromFileInfoCommon(path string, fi os.FileInfo) (Node, error) {
	node := Node{
		Name:    fi.Name(),
		Mode:    uint32(fi.Mode().Perm()),
		Size:    uint64(fi.Size()),
		MtimeNs: fi.ModTime().UnixNano(),
	}

	switch {
	case fi.Mode().IsRegular():
		node.Type = NodeTypeFile
	case fi.Mode().IsDir():
		node.Type = NodeTypeDir
	case fi.Mode()&os.ModeSymlink != 0:
		node.Type = NodeTypeSymlink
		target, err := os.Readlink(path)
		if err != nil {
			return Node{}, fmt.Errorf("fs: readlink %s: %w", path, err)
		}
		node.LinkTarget = target
	default:
		return Node{}, fmt.Errorf("fs: unsupported file type %s for %s", fi.Mode().Type(), path)
	}

	return node, nil
}
