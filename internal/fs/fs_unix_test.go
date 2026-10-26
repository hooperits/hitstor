//go:build !windows

package fs

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestNodeFromFileInfo_RegularFile tests that NodeFromFileInfo correctly reads
// mode, uid, gid, mtime, size, inode, and links from a real temporary file.
func TestNodeFromFileInfo_RegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile.txt")

	content := []byte("hello hitstor")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Set a known mtime so we can verify it.
	knownTime := time.Date(2024, 6, 15, 12, 30, 0, 0, time.UTC)
	if err := os.Chtimes(path, knownTime, knownTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}

	node, err := NodeFromFileInfo(path, fi)
	if err != nil {
		t.Fatalf("NodeFromFileInfo: %v", err)
	}

	// Verify type.
	if node.Type != NodeTypeFile {
		t.Errorf("Type = %q, want %q", node.Type, NodeTypeFile)
	}

	// Verify name.
	if node.Name != "testfile.txt" {
		t.Errorf("Name = %q, want %q", node.Name, "testfile.txt")
	}

	// Verify mode (permission bits).
	if node.Mode != 0o644 {
		t.Errorf("Mode = %#o, want %#o", node.Mode, 0o644)
	}

	// Verify uid/gid match the current user.
	stat := fi.Sys().(*syscall.Stat_t)
	if node.UID != stat.Uid {
		t.Errorf("UID = %d, want %d", node.UID, stat.Uid)
	}
	if node.GID != stat.Gid {
		t.Errorf("GID = %d, want %d", node.GID, stat.Gid)
	}

	// Verify mtime.
	if node.MtimeNs != knownTime.UnixNano() {
		t.Errorf("MtimeNs = %d, want %d", node.MtimeNs, knownTime.UnixNano())
	}

	// Verify size.
	if node.Size != uint64(len(content)) {
		t.Errorf("Size = %d, want %d", node.Size, len(content))
	}
}

// TestNodeFromFileInfo_InodeAndLinks tests that NodeFromFileInfo correctly reads
// the inode number and hardlink count from a real file.
func TestNodeFromFileInfo_InodeAndLinks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inode_test.txt")

	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}

	node, err := NodeFromFileInfo(path, fi)
	if err != nil {
		t.Fatalf("NodeFromFileInfo: %v", err)
	}

	// Inode should be nonzero.
	if node.Inode == 0 {
		t.Error("Inode = 0, want nonzero")
	}

	// A newly created file should have exactly 1 hard link.
	if node.Links != 1 {
		t.Errorf("Links = %d, want 1", node.Links)
	}

	// Verify against the actual stat values.
	stat := fi.Sys().(*syscall.Stat_t)
	if node.Inode != stat.Ino {
		t.Errorf("Inode = %d, want %d", node.Inode, stat.Ino)
	}
	if node.Links != uint64(stat.Nlink) {
		t.Errorf("Links = %d, want %d", node.Links, stat.Nlink)
	}

	// Create a hardlink and verify the link count increases.
	hardlink := filepath.Join(dir, "hardlink.txt")
	if err := os.Link(path, hardlink); err != nil {
		t.Fatalf("Link: %v", err)
	}

	fi2, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat after hardlink: %v", err)
	}

	node2, err := NodeFromFileInfo(path, fi2)
	if err != nil {
		t.Fatalf("NodeFromFileInfo after hardlink: %v", err)
	}

	if node2.Links != 2 {
		t.Errorf("Links after hardlink = %d, want 2", node2.Links)
	}

	// Both the original and the hardlink should share the same inode.
	if node2.Inode != node.Inode {
		t.Errorf("Inode changed after hardlink: got %d, want %d", node2.Inode, node.Inode)
	}
}
