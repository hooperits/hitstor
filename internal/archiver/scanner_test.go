package archiver

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/hooperits/hitstor/internal/fs"
)

// collectResults drains the ScanResult channel into a slice.
func collectResults(ch <-chan ScanResult) []ScanResult {
	var results []ScanResult
	for r := range ch {
		results = append(results, r)
	}
	return results
}

// TestScan_BasicDirectory tests that scanning a temp directory with files
// returns all files and directories.
func TestScan_BasicDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create structure:
	// dir/
	//   alpha.txt
	//   sub/
	//     beta.txt
	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "beta.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	ch, err := Scan([]string{dir}, nil)
	if err != nil {
		t.Fatalf("Scan() returned error: %v", err)
	}

	results := collectResults(ch)

	// Check no errors in results.
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("unexpected error result: %v", r.Err)
		}
	}

	// Expect 3 entries: alpha.txt, sub/, sub/beta.txt.
	if len(results) != 3 {
		names := make([]string, len(results))
		for i, r := range results {
			names[i] = r.Node.Name
		}
		t.Fatalf("got %d results %v, want 3", len(results), names)
	}

	// Collect names and sort for deterministic comparison.
	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.Node.Name
	}
	sort.Strings(names)

	expected := []string{"alpha.txt", "beta.txt", "sub"}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("sorted names[%d] = %q, want %q", i, names[i], name)
		}
	}
}

// TestScan_ExcludeLogFiles tests that exclude pattern "*.log" skips .log files.
func TestScan_ExcludeLogFiles(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("k"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skip.log"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "also_skip.log"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}

	ch, err := Scan([]string{dir}, []string{"*.log"})
	if err != nil {
		t.Fatalf("Scan() returned error: %v", err)
	}

	results := collectResults(ch)

	for _, r := range results {
		if r.Err != nil {
			t.Errorf("unexpected error result: %v", r.Err)
		}
	}

	if len(results) != 1 {
		names := make([]string, len(results))
		for i, r := range results {
			names[i] = r.Node.Name
		}
		t.Fatalf("got %d results %v, want 1 (only keep.txt)", len(results), names)
	}

	if results[0].Node.Name != "keep.txt" {
		t.Errorf("result name = %q, want %q", results[0].Node.Name, "keep.txt")
	}
}

// TestScan_ExcludeDirectory tests that exclude pattern "dir/" skips entire
// directory subtrees.
func TestScan_ExcludeDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create structure:
	// dir/
	//   keep.txt
	//   skipme/
	//     hidden.txt
	//   data/
	//     info.txt
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("k"), 0o644); err != nil {
		t.Fatal(err)
	}
	skipme := filepath.Join(dir, "skipme")
	if err := os.Mkdir(skipme, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skipme, "hidden.txt"), []byte("h"), 0o644); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(dir, "data")
	if err := os.Mkdir(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "info.txt"), []byte("i"), 0o644); err != nil {
		t.Fatal(err)
	}

	ch, err := Scan([]string{dir}, []string{"skipme/"})
	if err != nil {
		t.Fatalf("Scan() returned error: %v", err)
	}

	results := collectResults(ch)

	for _, r := range results {
		if r.Err != nil {
			t.Errorf("unexpected error result: %v", r.Err)
		}
	}

	// Should see: data, info.txt, keep.txt (skipme/ and its contents excluded).
	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.Node.Name
	}
	sort.Strings(names)

	expected := []string{"data", "info.txt", "keep.txt"}
	if len(names) != len(expected) {
		t.Fatalf("got names %v, want %v", names, expected)
	}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("sorted names[%d] = %q, want %q", i, names[i], name)
		}
	}
}

// TestScan_NonExistentPath tests that Scan returns an error for a path that
// does not exist.
func TestScan_NonExistentPath(t *testing.T) {
	_, err := Scan([]string{"/nonexistent/path/that/does/not/exist"}, nil)
	if err == nil {
		t.Fatal("Scan() should return error for non-existent path, got nil")
	}
}

// TestScan_ResultsIncludeCorrectNodeType tests that scan results include both
// files and directories with the correct NodeType.
func TestScan_ResultsIncludeCorrectNodeType(t *testing.T) {
	dir := t.TempDir()

	// Create structure:
	// dir/
	//   file.txt (file)
	//   subdir/  (directory)
	//     nested.txt (file)
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("f"), 0o644); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "nested.txt"), []byte("n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ch, err := Scan([]string{dir}, nil)
	if err != nil {
		t.Fatalf("Scan() returned error: %v", err)
	}

	results := collectResults(ch)

	// Build a map of name -> NodeType for easy lookup.
	typeByName := make(map[string]fs.NodeType)
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("unexpected error result: %v", r.Err)
			continue
		}
		typeByName[r.Node.Name] = r.Node.Type
	}

	// Verify types.
	expectedTypes := map[string]fs.NodeType{
		"file.txt":   fs.NodeTypeFile,
		"subdir":     fs.NodeTypeDir,
		"nested.txt": fs.NodeTypeFile,
	}

	for name, wantType := range expectedTypes {
		gotType, ok := typeByName[name]
		if !ok {
			t.Errorf("missing result for %q", name)
			continue
		}
		if gotType != wantType {
			t.Errorf("node %q: Type = %q, want %q", name, gotType, wantType)
		}
	}
}

// TestScan_SymlinksIncluded tests that symlinks are included in scan results
// with NodeTypeSymlink and the correct link target.
func TestScan_SymlinksIncluded(t *testing.T) {
	dir := t.TempDir()

	// Create a target file and a symlink pointing to it.
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("target content"), 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	ch, err := Scan([]string{dir}, nil)
	if err != nil {
		t.Fatalf("Scan() returned error: %v", err)
	}

	results := collectResults(ch)

	// Build a map of name -> result.
	byName := make(map[string]ScanResult)
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("unexpected error result: %v", r.Err)
			continue
		}
		byName[r.Node.Name] = r
	}

	if len(byName) != 2 {
		t.Fatalf("got %d results, want 2", len(byName))
	}

	// Verify the symlink entry.
	linkResult, ok := byName["link.txt"]
	if !ok {
		t.Fatal("missing result for link.txt")
	}
	if linkResult.Node.Type != fs.NodeTypeSymlink {
		t.Errorf("link.txt Type = %q, want %q", linkResult.Node.Type, fs.NodeTypeSymlink)
	}
	if linkResult.Node.LinkTarget != target {
		t.Errorf("link.txt LinkTarget = %q, want %q", linkResult.Node.LinkTarget, target)
	}

	// Verify the target file entry.
	targetResult, ok := byName["target.txt"]
	if !ok {
		t.Fatal("missing result for target.txt")
	}
	if targetResult.Node.Type != fs.NodeTypeFile {
		t.Errorf("target.txt Type = %q, want %q", targetResult.Node.Type, fs.NodeTypeFile)
	}
}

// TestScan_MultiplePaths tests that Scan correctly handles multiple root paths.
func TestScan_MultiplePaths(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir1, "from_dir1.txt"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "from_dir2.txt"), []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}

	ch, err := Scan([]string{dir1, dir2}, nil)
	if err != nil {
		t.Fatalf("Scan() returned error: %v", err)
	}

	results := collectResults(ch)

	for _, r := range results {
		if r.Err != nil {
			t.Errorf("unexpected error result: %v", r.Err)
		}
	}

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.Node.Name
	}
	sort.Strings(names)

	expected := []string{"from_dir1.txt", "from_dir2.txt"}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("sorted names[%d] = %q, want %q", i, names[i], name)
		}
	}
}

// TestScan_ResultPathsAreAbsolute tests that the Path field in ScanResult
// contains absolute paths.
func TestScan_ResultPathsAreAbsolute(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("f"), 0o644); err != nil {
		t.Fatal(err)
	}

	ch, err := Scan([]string{dir}, nil)
	if err != nil {
		t.Fatalf("Scan() returned error: %v", err)
	}

	results := collectResults(ch)

	for _, r := range results {
		if r.Err != nil {
			t.Errorf("unexpected error result: %v", r.Err)
			continue
		}
		if !filepath.IsAbs(r.Path) {
			t.Errorf("path %q is not absolute", r.Path)
		}
	}
}

// TestScan_EmptyDirectory tests that scanning an empty directory returns
// no results (the root itself is not emitted by fs.Walk).
func TestScan_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	ch, err := Scan([]string{dir}, nil)
	if err != nil {
		t.Fatalf("Scan() returned error: %v", err)
	}

	results := collectResults(ch)

	if len(results) != 0 {
		t.Fatalf("got %d results for empty directory, want 0", len(results))
	}
}

// TestScan_NonExistentPathAmongValid tests that if any path doesn't exist,
// Scan returns an error immediately (all paths are validated before scanning).
func TestScan_NonExistentPathAmongValid(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("f"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Scan([]string{dir, "/nonexistent/path"}, nil)
	if err == nil {
		t.Fatal("Scan() should return error when any path doesn't exist, got nil")
	}
}
