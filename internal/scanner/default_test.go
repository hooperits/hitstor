package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hooperits/hitstor/internal/fs"
)

func TestNewDefault(t *testing.T) {
	s := NewDefault()
	if s == nil {
		t.Fatal("NewDefault returned nil")
	}
}

func TestClose(t *testing.T) {
	s := NewDefault()
	if err := s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestScanSingleFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewDefault()
	defer s.Close()

	ch, err := s.Scan([]string{dir}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var results []ScanResult
	for r := range ch {
		results = append(results, r)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	if results[0].Err != nil {
		t.Errorf("unexpected error: %v", results[0].Err)
	}
	if results[0].Node.Name != "hello.txt" {
		t.Errorf("Name = %q, want %q", results[0].Node.Name, "hello.txt")
	}
	if results[0].Node.Type != fs.NodeTypeFile {
		t.Errorf("Type = %q, want %q", results[0].Node.Type, fs.NodeTypeFile)
	}
}

func TestScanTree(t *testing.T) {
	dir := t.TempDir()

	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewDefault()
	defer s.Close()

	ch, err := s.Scan([]string{dir}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var results []ScanResult
	for r := range ch {
		if r.Err != nil {
			t.Errorf("scan error: %v", r.Err)
			continue
		}
		results = append(results, r)
	}

	// Should have: a.txt, sub/, sub/b.txt = 3 entries
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	types := make(map[string]fs.NodeType)
	for _, r := range results {
		types[r.Node.Name] = r.Node.Type
	}

	if types["a.txt"] != fs.NodeTypeFile {
		t.Errorf("a.txt type = %q, want file", types["a.txt"])
	}
	if types["sub"] != fs.NodeTypeDir {
		t.Errorf("sub type = %q, want dir", types["sub"])
	}
	if types["b.txt"] != fs.NodeTypeFile {
		t.Errorf("b.txt type = %q, want file", types["b.txt"])
	}
}

func TestScanWithExcludes(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("k"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skip.log"), []byte("s"), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewDefault()
	defer s.Close()

	ch, err := s.Scan([]string{dir}, []string{"*.log"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var names []string
	for r := range ch {
		if r.Err != nil {
			t.Errorf("scan error: %v", r.Err)
			continue
		}
		names = append(names, r.Node.Name)
	}

	if len(names) != 1 || names[0] != "keep.txt" {
		t.Errorf("got names %v, want [keep.txt]", names)
	}
}

func TestScanNonExistentPath(t *testing.T) {
	s := NewDefault()
	defer s.Close()

	_, err := s.Scan([]string{"/nonexistent/path"}, nil)
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestScanEmpty(t *testing.T) {
	dir := t.TempDir()

	s := NewDefault()
	defer s.Close()

	ch, err := s.Scan([]string{dir}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var count int
	for range ch {
		count++
	}

	// Empty dir produces no entries (root is skipped by fs.Walk).
	if count != 0 {
		t.Errorf("got %d entries, want 0", count)
	}
}

func TestScanMultiplePaths(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir1, "one.txt"), []byte("1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "two.txt"), []byte("2"), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewDefault()
	defer s.Close()

	ch, err := s.Scan([]string{dir1, dir2}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var names []string
	for r := range ch {
		if r.Err != nil {
			t.Errorf("scan error: %v", r.Err)
			continue
		}
		names = append(names, r.Node.Name)
	}

	if len(names) != 2 {
		t.Errorf("got %d entries, want 2: %v", len(names), names)
	}
}
