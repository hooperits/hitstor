package checker

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hooperits/hitstor/internal/archiver"
	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/backend/local"
	"github.com/hooperits/hitstor/internal/repository"
)

// setupRepoWithBackup creates a local backend, initialises a repository,
// creates test data, runs a backup, then re-opens the repository so indexes
// are loaded fresh. It returns the re-opened repo and the backend.
func setupRepoWithBackup(t *testing.T) (*repository.Repository, *local.Local) {
	t.Helper()

	// Create test source data.
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("hello world"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "subdir", "data.bin"), bytes.Repeat([]byte{0xAB}, 2048), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Init repo.
	repoDir := t.TempDir()
	be, err := local.New(repoDir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}

	repo, err := repository.Init(be, "testpass")
	if err != nil {
		t.Fatalf("repository.Init: %v", err)
	}

	// Run backup.
	a := archiver.New(repo)
	_, _, err = a.Backup([]string{srcDir}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	// Re-open repo to reload indexes from disk.
	repo, err = repository.Open(be, "testpass")
	if err != nil {
		t.Fatalf("repository.Open: %v", err)
	}

	return repo, be
}

// TestCheckHealthyRepo verifies that a freshly backed-up repository passes
// all structural checks (readData=false).
func TestCheckHealthyRepo(t *testing.T) {
	repo, _ := setupRepoWithBackup(t)

	c := New(repo)
	var steps []string
	errs, err := c.Check(false, func(step string) {
		steps = append(steps, step)
	})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if len(errs) != 0 {
		for _, e := range errs {
			t.Errorf("unexpected check error: %s (pack=%s, blob=%s)", e.Message, e.Pack, e.Blob)
		}
		t.Fatalf("expected 0 check errors, got %d", len(errs))
	}

	// Verify progress was reported for the structural checks.
	if len(steps) < 3 {
		t.Errorf("expected at least 3 progress steps, got %d: %v", len(steps), steps)
	}
}

// TestCheckHealthyRepoWithReadData verifies that a freshly backed-up repository
// passes all checks including full data verification (readData=true).
func TestCheckHealthyRepoWithReadData(t *testing.T) {
	repo, _ := setupRepoWithBackup(t)

	c := New(repo)
	errs, err := c.Check(true, func(step string) {})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if len(errs) != 0 {
		for _, e := range errs {
			t.Errorf("unexpected check error: %s (pack=%s, blob=%s)", e.Message, e.Pack, e.Blob)
		}
		t.Fatalf("expected 0 check errors, got %d", len(errs))
	}
}

// TestCheckCorruptedPack corrupts a pack file on disk, then verifies that
// Check(true, ...) detects the corruption.
func TestCheckCorruptedPack(t *testing.T) {
	repo, be := setupRepoWithBackup(t)

	ctx := context.Background()

	// Find pack files.
	packIDs, err := be.List(ctx, backend.DataFile)
	if err != nil {
		t.Fatalf("List packs: %v", err)
	}
	if len(packIDs) == 0 {
		t.Fatal("no pack files found")
	}

	// Corrupt the first pack by flipping a byte near the middle.
	packID := packIDs[0]
	rc, err := be.Load(ctx, backend.DataFile, packID)
	if err != nil {
		t.Fatalf("Load pack: %v", err)
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("ReadAll pack: %v", err)
	}

	data[len(data)/2] ^= 0xFF

	if err := be.Save(ctx, backend.DataFile, packID, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save corrupted pack: %v", err)
	}

	// Re-open repo to ensure clean state.
	repo, err = repository.Open(be, "testpass")
	if err != nil {
		t.Fatalf("repository.Open: %v", err)
	}

	c := New(repo)
	errs, err := c.Check(true, func(step string) {})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected at least one check error for corrupted pack, got 0")
	}

	// At least one error should be reported.
	found := false
	for _, e := range errs {
		t.Logf("check error: %s (pack=%s, blob=%s)", e.Message, e.Pack, e.Blob)
		found = true
	}
	if !found {
		t.Fatal("no errors reported for corrupted pack")
	}
}

// TestCheckMissingPack deletes a pack file from the backend, then verifies
// that Check(false, ...) detects the missing pack.
func TestCheckMissingPack(t *testing.T) {
	repo, be := setupRepoWithBackup(t)

	ctx := context.Background()

	// Find pack files.
	packIDs, err := be.List(ctx, backend.DataFile)
	if err != nil {
		t.Fatalf("List packs: %v", err)
	}
	if len(packIDs) == 0 {
		t.Fatal("no pack files found")
	}

	// Delete the first pack.
	packID := packIDs[0]
	if err := be.Remove(ctx, backend.DataFile, packID); err != nil {
		t.Fatalf("Remove pack: %v", err)
	}

	// Re-open repo to ensure clean state.
	repo, err = repository.Open(be, "testpass")
	if err != nil {
		t.Fatalf("repository.Open: %v", err)
	}

	c := New(repo)
	errs, err := c.Check(false, func(step string) {})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected at least one check error for missing pack, got 0")
	}

	// Verify that an error mentions the missing pack.
	foundMissing := false
	for _, e := range errs {
		t.Logf("check error: %s (pack=%s, blob=%s)", e.Message, e.Pack, e.Blob)
		if e.Pack == packID {
			foundMissing = true
		}
	}
	if !foundMissing {
		t.Errorf("expected an error referencing pack %s, but none found", packID)
	}
}

// TestCheckSnapshotTreeResolvable verifies that snapshot tree blob references
// resolve successfully in a healthy repository.
func TestCheckSnapshotTreeResolvable(t *testing.T) {
	repo, _ := setupRepoWithBackup(t)

	// Verify snapshots exist.
	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snapIDs) == 0 {
		t.Fatal("no snapshots found")
	}

	c := New(repo)
	errs, err := c.Check(false, func(step string) {})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	// No errors should occur -- specifically no "snapshot tree blob missing" errors.
	for _, e := range errs {
		t.Errorf("unexpected check error: %s (pack=%s, blob=%s)", e.Message, e.Pack, e.Blob)
	}
	if len(errs) != 0 {
		t.Fatalf("expected 0 check errors, got %d", len(errs))
	}
}
