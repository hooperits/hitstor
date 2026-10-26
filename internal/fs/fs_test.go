package fs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestNodeFromFileInfo_Directory tests that NodeFromFileInfo correctly detects
// a directory type and reads its metadata.
func TestNodeFromFileInfo_Directory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	fi, err := os.Lstat(subdir)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}

	node, err := NodeFromFileInfo(subdir, fi)
	if err != nil {
		t.Fatalf("NodeFromFileInfo: %v", err)
	}

	if node.Type != NodeTypeDir {
		t.Errorf("Type = %q, want %q", node.Type, NodeTypeDir)
	}

	if node.Name != "subdir" {
		t.Errorf("Name = %q, want %q", node.Name, "subdir")
	}

	// Mode assertions are Unix-specific (Windows returns different values).
	if runtime.GOOS != "windows" {
		if node.Mode != 0o755 {
			t.Errorf("Mode = %#o, want %#o", node.Mode, 0o755)
		}
	}
}

// TestNodeFromFileInfo_Symlink tests that NodeFromFileInfo correctly detects
// a symlink and reads its target.
func TestNodeFromFileInfo_Symlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires developer mode or admin on Windows")
	}

	dir := t.TempDir()

	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("target"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}

	node, err := NodeFromFileInfo(link, fi)
	if err != nil {
		t.Fatalf("NodeFromFileInfo: %v", err)
	}

	if node.Type != NodeTypeSymlink {
		t.Errorf("Type = %q, want %q", node.Type, NodeTypeSymlink)
	}

	if node.Name != "link.txt" {
		t.Errorf("Name = %q, want %q", node.Name, "link.txt")
	}

	if node.LinkTarget != target {
		t.Errorf("LinkTarget = %q, want %q", node.LinkTarget, target)
	}
}

// TestWalk_BasicTraversal tests that Walk correctly traverses a simple directory
// tree and emits directories before their contents.
func TestWalk_BasicTraversal(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	var nodes []Node
	err := Walk(dir, nil, func(path string, node Node) error {
		nodes = append(nodes, node)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	if len(nodes) != 3 {
		t.Fatalf("got %d nodes, want 3", len(nodes))
	}

	expected := []struct {
		name     string
		nodeType NodeType
	}{
		{"a.txt", NodeTypeFile},
		{"sub", NodeTypeDir},
		{"b.txt", NodeTypeFile},
	}

	for i, exp := range expected {
		if nodes[i].Name != exp.name {
			t.Errorf("nodes[%d].Name = %q, want %q", i, nodes[i].Name, exp.name)
		}
		if nodes[i].Type != exp.nodeType {
			t.Errorf("nodes[%d].Type = %q, want %q", i, nodes[i].Type, exp.nodeType)
		}
	}
}

// TestWalk_ExcludePatterns tests that Walk correctly applies exclude patterns
// to skip files and directories.
func TestWalk_ExcludePatterns(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("k"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skip.log"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "cache")
	if err := os.Mkdir(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "cached.dat"), []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(dir, "data")
	if err := os.Mkdir(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "info.txt"), []byte("i"), 0o644); err != nil {
		t.Fatal(err)
	}

	excludes := []string{
		"*.log",  // exclude all .log files
		"cache/", // exclude cache directory subtree
	}

	var names []string
	err := Walk(dir, excludes, func(path string, node Node) error {
		names = append(names, node.Name)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	expected := []string{"data", "info.txt", "keep.txt"}
	if len(names) != len(expected) {
		t.Fatalf("got names %v, want %v", names, expected)
	}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("names[%d] = %q, want %q", i, names[i], name)
		}
	}
}

// TestWalk_SymlinksNotFollowed tests that Walk detects symlinks without following them.
func TestWalk_SymlinksNotFollowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires developer mode or admin on Windows")
	}

	dir := t.TempDir()

	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("target content"), 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	var nodes []Node
	err := Walk(dir, nil, func(path string, node Node) error {
		nodes = append(nodes, node)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	if len(nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(nodes))
	}

	if nodes[0].Name != "link.txt" {
		t.Errorf("nodes[0].Name = %q, want %q", nodes[0].Name, "link.txt")
	}
	if nodes[0].Type != NodeTypeSymlink {
		t.Errorf("nodes[0].Type = %q, want %q", nodes[0].Type, NodeTypeSymlink)
	}
	if nodes[0].LinkTarget != target {
		t.Errorf("nodes[0].LinkTarget = %q, want %q", nodes[0].LinkTarget, target)
	}

	if nodes[1].Name != "target.txt" {
		t.Errorf("nodes[1].Name = %q, want %q", nodes[1].Name, "target.txt")
	}
	if nodes[1].Type != NodeTypeFile {
		t.Errorf("nodes[1].Type = %q, want %q", nodes[1].Type, NodeTypeFile)
	}
}

// TestWalk_ExcludeRelativePath tests that exclude patterns are matched against
// the full relative path, not just the filename.
func TestWalk_ExcludeRelativePath(t *testing.T) {
	dir := t.TempDir()

	a := filepath.Join(dir, "a")
	if err := os.Mkdir(a, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a, "keep.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := filepath.Join(dir, "b")
	if err := os.Mkdir(b, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b, "keep.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	excludes := []string{"b/"}

	var names []string
	err := Walk(dir, excludes, func(path string, node Node) error {
		names = append(names, node.Name)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	expected := []string{"a", "keep.txt"}
	if len(names) != len(expected) {
		t.Fatalf("got names %v, want %v", names, expected)
	}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("names[%d] = %q, want %q", i, names[i], name)
		}
	}
}

// TestID_IsZero tests that ID.IsZero returns true for zero IDs and false otherwise.
func TestID_IsZero(t *testing.T) {
	var zero ID
	if !zero.IsZero() {
		t.Error("zero ID should be zero")
	}

	nonzero := ID{1}
	if nonzero.IsZero() {
		t.Error("nonzero ID should not be zero")
	}
}

// TestID_String tests that ID.String returns the correct hex encoding.
func TestID_String(t *testing.T) {
	var id ID
	for i := range id {
		id[i] = byte(i)
	}
	got := id.String()
	want := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
