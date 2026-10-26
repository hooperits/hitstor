package restorer

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/archiver"
	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/backend/local"
	"github.com/hooperits/hitstor/internal/fs"
	"github.com/hooperits/hitstor/internal/repository"
)

// setupRepo creates a local backend in a temp dir and initialises a repository.
func setupRepo(t *testing.T) *repository.Repository {
	t.Helper()
	repoDir := t.TempDir()
	be, err := local.New(repoDir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}
	repo, err := repository.Init(be, "testpass")
	if err != nil {
		t.Fatalf("repository.Init: %v", err)
	}
	t.Cleanup(func() { be.Close() })
	return repo
}

// backupDir runs a full backup of the given source directory and returns the
// snapshot ID.  The snapshot's root tree will contain one directory node whose
// name is filepath.Base(srcDir).
func backupDir(t *testing.T, repo *repository.Repository, srcDir string) backend.ID {
	t.Helper()
	a := archiver.New(repo)
	snapID, _, err := a.Backup([]string{srcDir}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	return snapID
}

// TestFullRestore backs up a directory with files and subdirs, restores it,
// and verifies file contents match byte-for-byte.
func TestFullRestore(t *testing.T) {
	srcDir := t.TempDir()
	dirName := filepath.Base(srcDir)

	// Create test structure inside srcDir:
	//   hello.txt    (content: "Hello, HitStor!")
	//   data/
	//     numbers.bin (content: 1024 bytes of 0xAB)
	//     nested/
	//       deep.txt  (content: "deep file content")
	writeFile(t, filepath.Join(srcDir, "hello.txt"), []byte("Hello, HitStor!"), 0644)
	mkdirAll(t, filepath.Join(srcDir, "data"))
	binContent := bytes.Repeat([]byte{0xAB}, 1024)
	writeFile(t, filepath.Join(srcDir, "data", "numbers.bin"), binContent, 0644)
	mkdirAll(t, filepath.Join(srcDir, "data", "nested"))
	writeFile(t, filepath.Join(srcDir, "data", "nested", "deep.txt"), []byte("deep file content"), 0644)

	repo := setupRepo(t)
	snapID := backupDir(t, repo, srcDir)

	// Restore to a different directory.
	restoreDir := t.TempDir()
	r := New(repo)
	if err := r.Restore(snapID, restoreDir, nil); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// The root tree contains one node (the backed-up directory). All files
	// are nested under restoreDir/<dirName>/.
	base := filepath.Join(restoreDir, dirName)
	verifyFileContent(t, filepath.Join(base, "hello.txt"), []byte("Hello, HitStor!"))
	verifyFileContent(t, filepath.Join(base, "data", "numbers.bin"), binContent)
	verifyFileContent(t, filepath.Join(base, "data", "nested", "deep.txt"), []byte("deep file content"))

	// Verify statistics.
	if r.FilesRestored != 3 {
		t.Errorf("FilesRestored = %d, want 3", r.FilesRestored)
	}
}

// TestMetadataPreserved verifies that permissions and timestamps are preserved
// after a backup and restore cycle.
func TestMetadataPreserved(t *testing.T) {
	srcDir := t.TempDir()
	dirName := filepath.Base(srcDir)

	filePath := filepath.Join(srcDir, "metafile.txt")
	writeFile(t, filePath, []byte("metadata test"), 0644)

	// Set specific permissions.
	if err := os.Chmod(filePath, 0640); err != nil {
		t.Fatal(err)
	}

	// Set specific timestamps.
	mtime := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	atime := time.Date(2024, 6, 15, 11, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filePath, atime, mtime); err != nil {
		t.Fatal(err)
	}

	repo := setupRepo(t)
	snapID := backupDir(t, repo, srcDir)

	restoreDir := t.TempDir()
	r := New(repo)
	if err := r.Restore(snapID, restoreDir, nil); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	restoredPath := filepath.Join(restoreDir, dirName, "metafile.txt")
	fi, err := os.Lstat(restoredPath)
	if err != nil {
		t.Fatalf("Lstat restored file: %v", err)
	}

	// Check permissions.
	gotPerm := fi.Mode().Perm()
	if gotPerm != 0640 {
		t.Errorf("permissions = %o, want %o", gotPerm, 0640)
	}

	// Check mtime (truncated to seconds for comparison).
	gotMtime := fi.ModTime().Truncate(time.Second)
	wantMtime := mtime.Truncate(time.Second)
	if !gotMtime.Equal(wantMtime) {
		t.Errorf("mtime = %v, want %v", gotMtime, wantMtime)
	}
}

// TestSymlinksRestored verifies that symlinks are restored as symlinks
// with the correct target.
func TestSymlinksRestored(t *testing.T) {
	srcDir := t.TempDir()
	dirName := filepath.Base(srcDir)

	// Create a target file and a relative symlink.
	writeFile(t, filepath.Join(srcDir, "target.txt"), []byte("target content"), 0644)

	linkPath := filepath.Join(srcDir, "mylink")
	if err := os.Symlink("target.txt", linkPath); err != nil {
		t.Fatal(err)
	}

	repo := setupRepo(t)
	snapID := backupDir(t, repo, srcDir)

	restoreDir := t.TempDir()
	r := New(repo)
	if err := r.Restore(snapID, restoreDir, nil); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	restoredLink := filepath.Join(restoreDir, dirName, "mylink")

	// Verify it is a symlink.
	fi, err := os.Lstat(restoredLink)
	if err != nil {
		t.Fatalf("Lstat restored symlink: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("restored entry is not a symlink: mode=%v", fi.Mode())
	}

	// Verify link target.
	gotTarget, err := os.Readlink(restoredLink)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if gotTarget != "target.txt" {
		t.Errorf("symlink target = %q, want %q", gotTarget, "target.txt")
	}
}

// TestIncludeFilter_SingleFile restores only one specific file and verifies
// only that file exists in the output.
func TestIncludeFilter_SingleFile(t *testing.T) {
	srcDir := t.TempDir()
	dirName := filepath.Base(srcDir)

	writeFile(t, filepath.Join(srcDir, "keep.txt"), []byte("keep me"), 0644)
	writeFile(t, filepath.Join(srcDir, "skip.txt"), []byte("skip me"), 0644)

	repo := setupRepo(t)
	snapID := backupDir(t, repo, srcDir)

	restoreDir := t.TempDir()
	r := New(repo)
	// Include filter: only restore the file at <dirName>/keep.txt.
	if err := r.Restore(snapID, restoreDir, []string{dirName + "/keep.txt"}); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// keep.txt should exist.
	verifyFileContent(t, filepath.Join(restoreDir, dirName, "keep.txt"), []byte("keep me"))

	// skip.txt should NOT exist.
	if _, err := os.Lstat(filepath.Join(restoreDir, dirName, "skip.txt")); !os.IsNotExist(err) {
		t.Errorf("skip.txt should not exist, but Lstat returned: %v", err)
	}
}

// TestIncludeFilter_Subdirectory restores only a subdirectory and verifies
// only that subtree exists.
func TestIncludeFilter_Subdirectory(t *testing.T) {
	srcDir := t.TempDir()
	dirName := filepath.Base(srcDir)

	// Create structure:
	//   top.txt
	//   sub/
	//     inner.txt
	writeFile(t, filepath.Join(srcDir, "top.txt"), []byte("top"), 0644)
	mkdirAll(t, filepath.Join(srcDir, "sub"))
	writeFile(t, filepath.Join(srcDir, "sub", "inner.txt"), []byte("inner"), 0644)

	repo := setupRepo(t)
	snapID := backupDir(t, repo, srcDir)

	restoreDir := t.TempDir()
	r := New(repo)
	// Include filter: only restore the sub/ directory and its contents.
	if err := r.Restore(snapID, restoreDir, []string{dirName + "/sub"}); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// sub/inner.txt should exist.
	verifyFileContent(t, filepath.Join(restoreDir, dirName, "sub", "inner.txt"), []byte("inner"))

	// top.txt should NOT exist.
	if _, err := os.Lstat(filepath.Join(restoreDir, dirName, "top.txt")); !os.IsNotExist(err) {
		t.Errorf("top.txt should not exist, but Lstat returned: %v", err)
	}
}

// TestNonExistentSnapshot verifies that restoring a non-existent snapshot
// returns an error.
func TestNonExistentSnapshot(t *testing.T) {
	repo := setupRepo(t)
	r := New(repo)

	var fakeID backend.ID
	fakeID[0] = 0xFF
	fakeID[1] = 0xEE

	restoreDir := t.TempDir()
	err := r.Restore(fakeID, restoreDir, nil)
	if err == nil {
		t.Fatal("expected error for non-existent snapshot, got nil")
	}
}

// TestParallelRestoreCorrectness backs up a directory with many files, restores
// it with parallel workers, and verifies all file contents, permissions,
// timestamps, and symlinks match the originals.
func TestParallelRestoreCorrectness(t *testing.T) {
	srcDir := t.TempDir()
	dirName := filepath.Base(srcDir)

	// Create 30 files with varied content and permissions.
	type fileInfo struct {
		relPath string
		content []byte
		perm    os.FileMode
		mtime   time.Time
	}
	var expectedFiles []fileInfo

	for i := 0; i < 30; i++ {
		data := bytes.Repeat([]byte{byte(i)}, 512*(i+1))
		name := filepath.Join("data", "file"+string(rune('A'+i%26))+string(rune('0'+i/26))+".dat")
		perm := os.FileMode(0644)
		if i%3 == 0 {
			perm = 0600
		}
		mtime := time.Date(2024, 3, 15, 10, i, 0, 0, time.UTC)

		absPath := filepath.Join(srcDir, name)
		mkdirAll(t, filepath.Dir(absPath))
		writeFile(t, absPath, data, perm)
		if err := os.Chmod(absPath, perm); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(absPath, mtime, mtime); err != nil {
			t.Fatal(err)
		}

		expectedFiles = append(expectedFiles, fileInfo{
			relPath: name,
			content: data,
			perm:    perm,
			mtime:   mtime,
		})
	}

	// Create a symlink.
	writeFile(t, filepath.Join(srcDir, "data", "target.txt"), []byte("link target"), 0644)
	if err := os.Symlink("target.txt", filepath.Join(srcDir, "data", "mylink")); err != nil {
		t.Fatal(err)
	}

	repo := setupRepo(t)
	snapID := backupDir(t, repo, srcDir)

	// Restore with parallel workers.
	restoreDir := t.TempDir()
	r := New(repo)
	r.SetParallel(4)
	if err := r.Restore(snapID, restoreDir, nil); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	base := filepath.Join(restoreDir, dirName)

	// Verify all file contents and permissions.
	for _, ef := range expectedFiles {
		restoredPath := filepath.Join(base, ef.relPath)
		verifyFileContent(t, restoredPath, ef.content)

		fi, err := os.Lstat(restoredPath)
		if err != nil {
			t.Fatalf("Lstat(%s): %v", restoredPath, err)
		}

		gotPerm := fi.Mode().Perm()
		if gotPerm != ef.perm {
			t.Errorf("permissions for %s = %o, want %o", ef.relPath, gotPerm, ef.perm)
		}

		gotMtime := fi.ModTime().Truncate(time.Second)
		wantMtime := ef.mtime.Truncate(time.Second)
		if !gotMtime.Equal(wantMtime) {
			t.Errorf("mtime for %s = %v, want %v", ef.relPath, gotMtime, wantMtime)
		}
	}

	// Verify symlink.
	restoredLink := filepath.Join(base, "data", "mylink")
	fi, err := os.Lstat(restoredLink)
	if err != nil {
		t.Fatalf("Lstat symlink: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink, got mode=%v", fi.Mode())
	}
	gotTarget, err := os.Readlink(restoredLink)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if gotTarget != "target.txt" {
		t.Errorf("symlink target = %q, want %q", gotTarget, "target.txt")
	}

	// Verify statistics: 30 data files + 1 target.txt = 31 files.
	if r.FilesRestored != 31 {
		t.Errorf("FilesRestored = %d, want 31", r.FilesRestored)
	}
}

// TestProgressCallback verifies SetProgress fires for each restored file.
func TestProgressCallback(t *testing.T) {
	srcDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "a.txt"), []byte("aaa"), 0644)
	writeFile(t, filepath.Join(srcDir, "b.txt"), []byte("bbb"), 0644)

	repo := setupRepo(t)
	snapID := backupDir(t, repo, srcDir)

	restoreDir := t.TempDir()
	r := New(repo)

	var mu sync.Mutex
	var progressPaths []string
	r.SetProgress(func(path string, size uint64) {
		mu.Lock()
		progressPaths = append(progressPaths, path)
		mu.Unlock()
	})

	if err := r.Restore(snapID, restoreDir, nil); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(progressPaths) != 2 {
		t.Errorf("progress callback called %d times, want 2", len(progressPaths))
	}
}

// TestTreeWalkCompleteCallback verifies SetTreeWalkComplete reports correct totals.
func TestTreeWalkCompleteCallback(t *testing.T) {
	srcDir := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "x.txt"), []byte("hello"), 0644)
	writeFile(t, filepath.Join(srcDir, "y.txt"), []byte("world!"), 0644)

	repo := setupRepo(t)
	snapID := backupDir(t, repo, srcDir)

	restoreDir := t.TempDir()
	r := New(repo)

	var gotFiles int
	var gotBytes uint64
	r.SetTreeWalkComplete(func(files int, totalBytes uint64) {
		gotFiles = files
		gotBytes = totalBytes
	})

	if err := r.Restore(snapID, restoreDir, nil); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if gotFiles != 2 {
		t.Errorf("tree walk files = %d, want 2", gotFiles)
	}
	if gotBytes != 11 { // "hello" (5) + "world!" (6)
		t.Errorf("tree walk bytes = %d, want 11", gotBytes)
	}
}

// TestLoadTreeNotFound exercises loadTree with a non-existent blob ID.
func TestLoadTreeNotFound(t *testing.T) {
	repo := setupRepo(t)
	r := New(repo)

	var fakeID backend.ID
	fakeID[0] = 0xDE
	fakeID[1] = 0xAD

	_, err := r.loadTree(fakeID)
	if err == nil {
		t.Error("expected error for non-existent tree blob")
	}
}

// TestRestoreFileNotFound exercises restoreFile with a node containing a non-existent content blob.
func TestRestoreFileNotFound(t *testing.T) {
	repo := setupRepo(t)
	r := New(repo)

	dir := t.TempDir()
	filePath := filepath.Join(dir, "missing.txt")

	var fakeContentID fs.ID
	fakeContentID[0] = 0xBE
	fakeContentID[1] = 0xEF

	node := fs.Node{
		Name:    "missing.txt",
		Type:    fs.NodeTypeFile,
		Content: []fs.ID{fakeContentID},
	}

	err := r.restoreFile(filePath, node)
	if err == nil {
		t.Error("expected error for non-existent content blob")
	}
}

// TestSetOwnershipCurrentUser exercises setOwnership with the current user's UID/GID
// which should succeed (no EPERM).
func TestSetOwnershipCurrentUser(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "owned.txt")
	writeFile(t, f, []byte("test"), 0644)

	r := New(nil) // repo not needed for setOwnership
	r.setOwnership(f, uint32(os.Getuid()), uint32(os.Getgid()))

	// Verify file still exists and is accessible.
	if _, err := os.Stat(f); err != nil {
		t.Errorf("file not accessible after setOwnership: %v", err)
	}
}

// --- helpers ---

func writeFile(t *testing.T, path string, data []byte, perm os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, perm); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
}

func verifyFileContent(t *testing.T, path string, expected []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if !bytes.Equal(got, expected) {
		t.Errorf("content mismatch for %s:\n  got  (%d bytes): %q\n  want (%d bytes): %q",
			path, len(got), truncate(got, 80), len(expected), truncate(expected, 80))
	}
}

func truncate(b []byte, max int) []byte {
	if len(b) <= max {
		return b
	}
	return append(b[:max], []byte("...")...)
}
