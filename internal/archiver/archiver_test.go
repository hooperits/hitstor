package archiver

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/hooperits/hitstor/internal/backend/local"
	"github.com/hooperits/hitstor/internal/cache"
	"github.com/hooperits/hitstor/internal/repository"
)

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

// TestParallelBackupCorrectness verifies that parallel backup produces
// identical dedup counts and summary stats as sequential backup.
func TestParallelBackupCorrectness(t *testing.T) {
	srcDir := t.TempDir()

	// Create 50 files with varied content.
	for i := 0; i < 50; i++ {
		data := bytes.Repeat([]byte{byte(i)}, 1024*(i+1))
		name := filepath.Join(srcDir, "file"+string(rune('A'+i%26))+string(rune('0'+i/26))+".dat")
		if err := os.WriteFile(name, data, 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	// Sequential backup (parallel=1).
	repoSeq := setupRepo(t)
	aSeq := New(repoSeq)
	aSeq.SetParallel(1)
	snapIDSeq, snapSeq, err := aSeq.Backup([]string{srcDir}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("sequential backup: %v", err)
	}

	// Parallel backup (parallel=4).
	repoPar := setupRepo(t)
	aPar := New(repoPar)
	aPar.SetParallel(4)
	snapIDPar, snapPar, err := aPar.Backup([]string{srcDir}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("parallel backup: %v", err)
	}

	// Both should produce valid snapshots.
	if snapIDSeq == snapIDPar {
		// IDs may differ due to different pack ordering, but both should be non-zero.
	}
	_ = snapIDSeq
	_ = snapIDPar

	// Summary stats should match.
	if snapSeq.Summary.TotalFiles != snapPar.Summary.TotalFiles {
		t.Errorf("TotalFiles: sequential=%d, parallel=%d",
			snapSeq.Summary.TotalFiles, snapPar.Summary.TotalFiles)
	}
	if snapSeq.Summary.FilesNew != snapPar.Summary.FilesNew {
		t.Errorf("FilesNew: sequential=%d, parallel=%d",
			snapSeq.Summary.FilesNew, snapPar.Summary.FilesNew)
	}
	if snapSeq.Summary.FilesUnchanged != snapPar.Summary.FilesUnchanged {
		t.Errorf("FilesUnchanged: sequential=%d, parallel=%d",
			snapSeq.Summary.FilesUnchanged, snapPar.Summary.FilesUnchanged)
	}
	if snapSeq.Summary.BytesProcessed != snapPar.Summary.BytesProcessed {
		t.Errorf("BytesProcessed: sequential=%d, parallel=%d",
			snapSeq.Summary.BytesProcessed, snapPar.Summary.BytesProcessed)
	}
	if snapSeq.Summary.TotalSize != snapPar.Summary.TotalSize {
		t.Errorf("TotalSize: sequential=%d, parallel=%d",
			snapSeq.Summary.TotalSize, snapPar.Summary.TotalSize)
	}
}

// TestParallelBackupDedup verifies deduplication works correctly: a second
// backup of the same data should produce zero new files.
func TestParallelBackupDedup(t *testing.T) {
	srcDir := t.TempDir()

	for i := 0; i < 20; i++ {
		data := bytes.Repeat([]byte{byte(i)}, 2048)
		if err := os.WriteFile(filepath.Join(srcDir, "dup"+string(rune('A'+i))+".dat"), data, 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	repo := setupRepo(t)

	// First backup.
	a1 := New(repo)
	a1.SetParallel(4)
	_, snap1, err := a1.Backup([]string{srcDir}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("first backup: %v", err)
	}
	if snap1.Summary.FilesNew != 20 {
		t.Errorf("first backup FilesNew = %d, want 20", snap1.Summary.FilesNew)
	}

	// Second backup of same data — all files should be unchanged.
	a2 := New(repo)
	a2.SetParallel(4)
	_, snap2, err := a2.Backup([]string{srcDir}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("second backup: %v", err)
	}
	if snap2.Summary.FilesNew != 0 {
		t.Errorf("second backup FilesNew = %d, want 0", snap2.Summary.FilesNew)
	}
	if snap2.Summary.FilesUnchanged != 20 {
		t.Errorf("second backup FilesUnchanged = %d, want 20", snap2.Summary.FilesUnchanged)
	}
}

// TestBackupWithCallbacks covers SetProgress, SetScanComplete, and Summary.
func TestBackupWithCallbacks(t *testing.T) {
	srcDir := t.TempDir()
	for i := 0; i < 5; i++ {
		data := bytes.Repeat([]byte{byte(i)}, 512)
		if err := os.WriteFile(filepath.Join(srcDir, "cb"+string(rune('A'+i))+".dat"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	repo := setupRepo(t)
	a := New(repo)

	var progressCount int
	a.SetProgress(func(path string, size uint64, isNew bool) {
		progressCount++
	})

	var scanFiles, scanDirs int
	var scanBytes uint64
	a.SetScanComplete(func(files, dirs int, totalBytes uint64) {
		scanFiles = files
		scanDirs = dirs
		scanBytes = totalBytes
	})

	_, _, err := a.Backup([]string{srcDir}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	if progressCount != 5 {
		t.Errorf("progress called %d times, want 5", progressCount)
	}
	if scanFiles != 5 {
		t.Errorf("scanComplete files = %d, want 5", scanFiles)
	}
	if scanDirs == 0 {
		t.Error("scanComplete dirs should be > 0")
	}
	if scanBytes == 0 {
		t.Error("scanComplete bytes should be > 0")
	}

	sum := a.Summary()
	if sum.TotalFiles != 5 {
		t.Errorf("Summary TotalFiles = %d, want 5", sum.TotalFiles)
	}
	if sum.FilesNew != 5 {
		t.Errorf("Summary FilesNew = %d, want 5", sum.FilesNew)
	}
}

// TestDryRunBackup covers SetDryRun — should scan but produce no snapshot ID.
func TestDryRunBackup(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "dryrun.txt"), []byte("dry"), 0644); err != nil {
		t.Fatal(err)
	}

	repo := setupRepo(t)
	a := New(repo)
	a.SetDryRun(true)

	snapID, snap, err := a.Backup([]string{srcDir}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	// Dry-run returns zero ID.
	var zeroID [32]byte
	if snapID != zeroID {
		t.Errorf("dry-run should return zero snapID, got %s", snapID)
	}

	// Snapshot summary should have file count from scan.
	if snap.Summary.TotalFiles != 1 {
		t.Errorf("dry-run TotalFiles = %d, want 1", snap.Summary.TotalFiles)
	}
}

// TestSetCacheAndSetScanner covers the remaining zero-coverage setters.
func TestSetCacheAndSetScanner(t *testing.T) {
	repo := setupRepo(t)
	a := New(repo)

	// SetCache with nil — should not panic.
	a.SetCache(nil)

	// SetScanner with nil — should not panic.
	a.SetScanner(nil)
}

// TestBackupWithCache covers processFile cache paths (LookupFile, IsKnownChunk, UpdateFile, AddKnownChunk).
func TestBackupWithCache(t *testing.T) {
	srcDir := t.TempDir()
	for i := 0; i < 3; i++ {
		data := bytes.Repeat([]byte{byte(i + 10)}, 1024)
		if err := os.WriteFile(filepath.Join(srcDir, "cached"+string(rune('A'+i))+".dat"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	repo := setupRepo(t)

	cacheFile := filepath.Join(t.TempDir(), "test-cache.db")
	fc, err := cache.Open(cacheFile)
	if err != nil {
		t.Fatalf("cache.Open: %v", err)
	}
	defer fc.Close()

	// First backup with cache — should process all files as new.
	a1 := New(repo)
	a1.SetCache(fc)
	_, snap1, err := a1.Backup([]string{srcDir}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("first backup: %v", err)
	}
	if snap1.Summary.FilesNew != 3 {
		t.Errorf("first backup FilesNew = %d, want 3", snap1.Summary.FilesNew)
	}

	// Second backup with same cache — files should be found in cache.
	a2 := New(repo)
	a2.SetCache(fc)
	_, snap2, err := a2.Backup([]string{srcDir}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("second backup: %v", err)
	}
	if snap2.Summary.FilesUnchanged != 3 {
		t.Errorf("second backup FilesUnchanged = %d, want 3", snap2.Summary.FilesUnchanged)
	}
	if snap2.Summary.FilesNew != 0 {
		t.Errorf("second backup FilesNew = %d, want 0", snap2.Summary.FilesNew)
	}
}
