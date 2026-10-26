//go:build windows

package fs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNodeFromFileInfo_RegularFile_Windows tests that NodeFromFileInfo
// correctly reads metadata on Windows (UID/GID are always 0).
func TestNodeFromFileInfo_RegularFile_Windows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile.txt")

	content := []byte("hello hitstor")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

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

	if node.Type != NodeTypeFile {
		t.Errorf("Type = %q, want %q", node.Type, NodeTypeFile)
	}
	if node.Name != "testfile.txt" {
		t.Errorf("Name = %q, want %q", node.Name, "testfile.txt")
	}

	// Windows UID/GID are always 0.
	if node.UID != 0 {
		t.Errorf("UID = %d, want 0", node.UID)
	}
	if node.GID != 0 {
		t.Errorf("GID = %d, want 0", node.GID)
	}

	if node.MtimeNs != knownTime.UnixNano() {
		t.Errorf("MtimeNs = %d, want %d", node.MtimeNs, knownTime.UnixNano())
	}
	if node.Size != uint64(len(content)) {
		t.Errorf("Size = %d, want %d", node.Size, len(content))
	}
}

// TestNodeFromFileInfo_InodeAndLinks_Windows tests that the NTFS FileIndex
// is used as a synthetic inode.
func TestNodeFromFileInfo_InodeAndLinks_Windows(t *testing.T) {
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

	// On NTFS, inode (FileIndex) should be nonzero.
	if node.Inode == 0 {
		t.Error("Inode = 0, want nonzero on NTFS")
	}

	// A new file should have at least 1 link.
	if node.Links < 1 {
		t.Errorf("Links = %d, want >= 1", node.Links)
	}
}
