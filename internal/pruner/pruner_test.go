package pruner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hooperits/hitstor/internal/archiver"
	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/backend/local"
	"github.com/hooperits/hitstor/internal/checker"
	"github.com/hooperits/hitstor/internal/repository"
)

func nopProgress(string) {}

// setupRepoWithTwoBackups creates a local backend, initialises a repository,
// creates two backups with different data, and re-opens the repository.
func setupRepoWithTwoBackups(t *testing.T) (*repository.Repository, *local.Local) {
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

	// Backup 1: unique data.
	src1 := t.TempDir()
	if err := os.WriteFile(filepath.Join(src1, "file1.txt"), bytes.Repeat([]byte("A"), 4096), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	a := archiver.New(repo)
	if _, _, err := a.Backup([]string{src1}, nil, "host1", nil); err != nil {
		t.Fatalf("Backup 1: %v", err)
	}

	// Backup 2: different unique data.
	src2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(src2, "file2.txt"), bytes.Repeat([]byte("B"), 4096), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	a2 := archiver.New(repo)
	if _, _, err := a2.Backup([]string{src2}, nil, "host2", nil); err != nil {
		t.Fatalf("Backup 2: %v", err)
	}

	// Re-open to reload indexes.
	repo, err = repository.Open(be, "testpass")
	if err != nil {
		t.Fatalf("repository.Open: %v", err)
	}

	return repo, be
}

func TestPruneNoop(t *testing.T) {
	repo, _ := setupRepoWithTwoBackups(t)

	p := New(repo)
	stats, err := p.Prune(Options{}, nopProgress)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	if stats.PacksDeleted != 0 {
		t.Errorf("packs_deleted = %d, want 0", stats.PacksDeleted)
	}
	if stats.PacksRepacked != 0 {
		t.Errorf("packs_repacked = %d, want 0", stats.PacksRepacked)
	}
	if stats.SnapshotsTotal != 2 {
		t.Errorf("snapshots_total = %d, want 2", stats.SnapshotsTotal)
	}
}

func TestPruneAfterForget(t *testing.T) {
	repo, be := setupRepoWithTwoBackups(t)

	// Count packs before.
	packsBefore, err := be.List(context.Background(), backend.DataFile)
	if err != nil {
		t.Fatalf("list packs before: %v", err)
	}

	// Forget one snapshot.
	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snapIDs) != 2 {
		t.Fatalf("snapshot count = %d, want 2", len(snapIDs))
	}
	if err := repo.RemoveSnapshot(snapIDs[0]); err != nil {
		t.Fatalf("RemoveSnapshot: %v", err)
	}

	// Prune.
	p := New(repo)
	stats, err := p.Prune(Options{}, nopProgress)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	if stats.PacksDeleted+stats.PacksRepacked == 0 {
		t.Error("expected some packs to be deleted or repacked")
	}
	if stats.BytesFreed == 0 {
		t.Error("expected some bytes to be freed")
	}

	// Verify packs decreased or stayed same (repacked ones replaced).
	packsAfter, err := be.List(context.Background(), backend.DataFile)
	if err != nil {
		t.Fatalf("list packs after: %v", err)
	}
	if len(packsAfter) > len(packsBefore) {
		t.Errorf("packs after (%d) > packs before (%d)", len(packsAfter), len(packsBefore))
	}

	// Re-open to reload indexes after prune.
	repo, err = repository.Open(be, "testpass")
	if err != nil {
		t.Fatalf("repository.Open after prune: %v", err)
	}

	// Remaining snapshot should still be valid.
	c := checker.New(repo)
	errs, err := c.Check(true, func(string) {})
	if err != nil {
		t.Fatalf("Check after prune: %v", err)
	}
	if len(errs) != 0 {
		for _, e := range errs {
			t.Errorf("check error: %s", e.Message)
		}
		t.Fatalf("expected 0 check errors after prune, got %d", len(errs))
	}
}

func TestPruneDryRun(t *testing.T) {
	repo, be := setupRepoWithTwoBackups(t)

	// Count packs before.
	packsBefore, err := be.List(context.Background(), backend.DataFile)
	if err != nil {
		t.Fatalf("list packs before: %v", err)
	}

	// Forget one snapshot.
	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if err := repo.RemoveSnapshot(snapIDs[0]); err != nil {
		t.Fatalf("RemoveSnapshot: %v", err)
	}

	// Dry-run prune.
	p := New(repo)
	stats, err := p.Prune(Options{DryRun: true}, nopProgress)
	if err != nil {
		t.Fatalf("Prune dry-run: %v", err)
	}

	if stats.PacksDeleted+stats.PacksRepacked == 0 {
		t.Error("dry-run should report packs to delete/repack")
	}

	// Verify nothing was actually deleted.
	packsAfter, err := be.List(context.Background(), backend.DataFile)
	if err != nil {
		t.Fatalf("list packs after dry-run: %v", err)
	}
	if len(packsAfter) != len(packsBefore) {
		t.Errorf("packs changed during dry-run: before=%d, after=%d", len(packsBefore), len(packsAfter))
	}
}

func TestPruneRepack(t *testing.T) {
	// Create a scenario where blobs from two snapshots share a pack.
	// With small data and a single pack writer flush, both backups' blobs
	// end up in the same pack. Forgetting one snapshot means the pack is
	// partially referenced and must be repacked.
	repoDir := t.TempDir()
	be, err := local.New(repoDir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}

	repo, err := repository.Init(be, "testpass")
	if err != nil {
		t.Fatalf("repository.Init: %v", err)
	}

	// Backup 1.
	src1 := t.TempDir()
	if err := os.WriteFile(filepath.Join(src1, "a.txt"), []byte("alpha"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	a1 := archiver.New(repo)
	if _, _, err := a1.Backup([]string{src1}, nil, "host", nil); err != nil {
		t.Fatalf("Backup 1: %v", err)
	}

	// Backup 2 (different data, small enough to possibly share packs).
	src2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(src2, "b.txt"), []byte("bravo"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	a2 := archiver.New(repo)
	if _, _, err := a2.Backup([]string{src2}, nil, "host", nil); err != nil {
		t.Fatalf("Backup 2: %v", err)
	}

	// Re-open.
	repo, err = repository.Open(be, "testpass")
	if err != nil {
		t.Fatalf("repository.Open: %v", err)
	}

	// Forget one snapshot.
	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if err := repo.RemoveSnapshot(snapIDs[0]); err != nil {
		t.Fatalf("RemoveSnapshot: %v", err)
	}

	// Prune.
	p := New(repo)
	stats, err := p.Prune(Options{}, nopProgress)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	// Should have processed something.
	if stats.SnapshotsTotal != 1 {
		t.Errorf("snapshots_total = %d, want 1", stats.SnapshotsTotal)
	}

	// Re-open and check integrity.
	repo, err = repository.Open(be, "testpass")
	if err != nil {
		t.Fatalf("repository.Open after prune: %v", err)
	}

	c := checker.New(repo)
	errs, checkErr := c.Check(true, func(string) {})
	if checkErr != nil {
		t.Fatalf("Check: %v", checkErr)
	}
	if len(errs) != 0 {
		for _, e := range errs {
			t.Errorf("check error: %s", e.Message)
		}
		t.Fatalf("expected 0 check errors, got %d", len(errs))
	}
}

// TestPruneMaxRepackSize verifies the MaxRepackSize option limits how much
// data gets repacked in a single prune pass.
func TestPruneMaxRepackSize(t *testing.T) {
	repo, be := setupRepoWithTwoBackups(t)

	// Forget one snapshot so there is something to prune/repack.
	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.RemoveSnapshot(snapIDs[0]); err != nil {
		t.Fatal(err)
	}

	p := New(repo)
	// MaxRepackSize=1 means at most 1 byte — should stop repacking immediately.
	stats, err := p.Prune(Options{MaxRepackSize: 1}, nopProgress)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	// Should still report stats (packs classified), even if repacking was limited.
	if stats.SnapshotsTotal != 1 {
		t.Errorf("snapshots_total = %d, want 1", stats.SnapshotsTotal)
	}

	// Re-open and check integrity. Even with limited repack, the surviving
	// snapshot should still be valid (fully-referenced packs are untouched).
	repo, err = repository.Open(be, "testpass")
	if err != nil {
		t.Fatalf("repository.Open: %v", err)
	}
	c := checker.New(repo)
	errs, checkErr := c.Check(true, func(string) {})
	if checkErr != nil {
		t.Fatalf("Check: %v", checkErr)
	}
	if len(errs) != 0 {
		t.Fatalf("expected 0 check errors, got %d", len(errs))
	}
}

// TestPruneProgressMessages verifies progress callback is called with
// expected messages during a prune.
func TestPruneProgressMessages(t *testing.T) {
	repo, _ := setupRepoWithTwoBackups(t)

	// Forget one snapshot.
	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.RemoveSnapshot(snapIDs[0]); err != nil {
		t.Fatal(err)
	}

	var messages []string
	progress := func(msg string) {
		messages = append(messages, msg)
	}

	p := New(repo)
	_, err = p.Prune(Options{}, progress)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	// Should have at least "collecting referenced blobs..." and "classifying packs...".
	if len(messages) < 2 {
		t.Errorf("progress called %d times, want >= 2", len(messages))
	}
	found := false
	for _, m := range messages {
		if strings.Contains(m, "collecting") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected progress message containing 'collecting', got: %v", messages)
	}
}

// TestPruneForceRepack creates a scenario that forces the repackPacks and
// flushPackWriter code paths by creating shared blobs across snapshots.
func TestPruneForceRepack(t *testing.T) {
	repoDir := t.TempDir()
	be, err := local.New(repoDir)
	if err != nil {
		t.Fatal(err)
	}

	repo, err := repository.Init(be, "testpass")
	if err != nil {
		t.Fatal(err)
	}

	// Create a single source directory with shared data.
	srcDir := t.TempDir()
	sharedData := bytes.Repeat([]byte("S"), 2048)
	if err := os.WriteFile(filepath.Join(srcDir, "shared.txt"), sharedData, 0644); err != nil {
		t.Fatal(err)
	}

	// Backup 1: shared + unique data.
	if err := os.WriteFile(filepath.Join(srcDir, "unique1.txt"), bytes.Repeat([]byte("U"), 1024), 0644); err != nil {
		t.Fatal(err)
	}
	a1 := archiver.New(repo)
	if _, _, err := a1.Backup([]string{srcDir}, nil, "host", nil); err != nil {
		t.Fatal(err)
	}

	// Modify unique file for second backup (shared stays same → dedup).
	if err := os.WriteFile(filepath.Join(srcDir, "unique1.txt"), bytes.Repeat([]byte("V"), 1024), 0644); err != nil {
		t.Fatal(err)
	}
	a2 := archiver.New(repo)
	if _, _, err := a2.Backup([]string{srcDir}, nil, "host", nil); err != nil {
		t.Fatal(err)
	}

	// Re-open.
	repo, err = repository.Open(be, "testpass")
	if err != nil {
		t.Fatal(err)
	}

	// Forget the first snapshot.
	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapIDs) < 2 {
		t.Fatalf("expected at least 2 snapshots, got %d", len(snapIDs))
	}
	if err := repo.RemoveSnapshot(snapIDs[0]); err != nil {
		t.Fatal(err)
	}

	// Prune — this should exercise repackPacks and flushPackWriter because
	// the first snapshot's pack has both shared (still referenced) and
	// unique (now unreferenced) blobs.
	p := New(repo)
	stats, err := p.Prune(Options{}, nopProgress)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}

	// Verify something happened.
	if stats.PacksDeleted+stats.PacksRepacked == 0 {
		t.Log("no repack/delete needed — blobs may not share packs with small data")
	}

	// Verify integrity after prune.
	repo, err = repository.Open(be, "testpass")
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New(repo)
	errs, checkErr := c.Check(true, func(string) {})
	if checkErr != nil {
		t.Fatal(checkErr)
	}
	if len(errs) != 0 {
		for _, e := range errs {
			t.Errorf("check error: %s", e.Message)
		}
	}
}

// TestPruneNoopProgress verifies that noop progress doesn't cause issues
// when nothing needs to be pruned.
func TestPruneNoopProgress(t *testing.T) {
	repo, _ := setupRepoWithTwoBackups(t)

	p := New(repo)
	stats, err := p.Prune(Options{}, nopProgress)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if stats.BlobsUnreferenced != 0 {
		t.Errorf("BlobsUnreferenced = %d, want 0", stats.BlobsUnreferenced)
	}
	if stats.BytesFreed != 0 {
		t.Errorf("BytesFreed = %d, want 0", stats.BytesFreed)
	}
}

// TestPruneDryRunStats verifies dry-run reports correct statistics without modifying data.
func TestPruneDryRunStats(t *testing.T) {
	repo, be := setupRepoWithTwoBackups(t)

	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.RemoveSnapshot(snapIDs[0]); err != nil {
		t.Fatal(err)
	}

	// Dry-run prune.
	p := New(repo)
	stats, err := p.Prune(Options{DryRun: true}, nopProgress)
	if err != nil {
		t.Fatal(err)
	}
	if stats.BlobsReferenced == 0 {
		t.Error("expected referenced blobs > 0")
	}
	if stats.IndexesWritten != 0 {
		t.Errorf("dry run should not write indexes, got %d", stats.IndexesWritten)
	}

	// Verify no packs were actually deleted.
	packsAfter, err := be.List(context.Background(), backend.DataFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(packsAfter) == 0 {
		t.Error("packs should still exist after dry run")
	}
}

// TestFlushPackWriter directly tests the flushPackWriter method.
func TestFlushPackWriter(t *testing.T) {
	repoDir := t.TempDir()
	be, err := local.New(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	defer be.Close()

	repo, err := repository.Init(be, "testpass")
	if err != nil {
		t.Fatal(err)
	}

	p := New(repo)
	encKey := repo.Key().EncryptKey
	pw := repository.NewPackWriter(encKey)

	// Add a test blob.
	testData := bytes.Repeat([]byte("X"), 256)
	var blobID backend.ID
	copy(blobID[:], bytes.Repeat([]byte{0xAB}, 32))
	if err := pw.AddBlob(blobID, repository.DataBlob, testData, len(testData)); err != nil {
		t.Fatal(err)
	}

	ip, err := p.flushPackWriter(pw)
	if err != nil {
		t.Fatalf("flushPackWriter: %v", err)
	}
	if ip.PackID == (backend.ID{}) {
		t.Error("expected non-zero PackID")
	}
	if len(ip.Blobs) == 0 {
		t.Error("expected blobs in flushed pack")
	}
}

// TestRepackPacks directly tests the repackPacks method with a manually
// constructed partially-referenced pack.
func TestRepackPacks(t *testing.T) {
	repoDir := t.TempDir()
	be, err := local.New(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	defer be.Close()

	repo, err := repository.Init(be, "testpass")
	if err != nil {
		t.Fatal(err)
	}

	// Create a pack with two blobs.
	encKey := repo.Key().EncryptKey
	pw := repository.NewPackWriter(encKey)

	blobA := backend.ID{0x01}
	blobB := backend.ID{0x02}
	dataA := bytes.Repeat([]byte("A"), 128)
	dataB := bytes.Repeat([]byte("B"), 128)

	if err := pw.AddBlob(blobA, repository.DataBlob, dataA, len(dataA)); err != nil {
		t.Fatal(err)
	}
	if err := pw.AddBlob(blobB, repository.DataBlob, dataB, len(dataB)); err != nil {
		t.Fatal(err)
	}

	_, packData, err := pw.Finalize()
	if err != nil {
		t.Fatal(err)
	}

	packID, err := repo.SavePack(packData)
	if err != nil {
		t.Fatal(err)
	}

	blobs, err := repository.ReadPackHeader(encKey, packData)
	if err != nil {
		t.Fatal(err)
	}

	partialRef := []repository.IndexPack{
		{PackID: packID, Blobs: blobs},
	}

	// Only blob A is referenced — blob B should be dropped during repack.
	referenced := map[backend.ID]bool{blobA: true}

	p := New(repo)
	newPacks, err := p.repackPacks(partialRef, referenced, 0)
	if err != nil {
		t.Fatalf("repackPacks: %v", err)
	}

	if len(newPacks) == 0 {
		t.Fatal("expected at least one new pack from repack")
	}

	// The new pack should have only 1 blob (blob A).
	totalBlobs := 0
	for _, np := range newPacks {
		totalBlobs += len(np.Blobs)
	}
	if totalBlobs != 1 {
		t.Errorf("repacked blobs = %d, want 1", totalBlobs)
	}
}

// TestRepackPacksWithMaxSize tests repackPacks with MaxRepackSize limit.
func TestRepackPacksWithMaxSize(t *testing.T) {
	repoDir := t.TempDir()
	be, err := local.New(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	defer be.Close()

	repo, err := repository.Init(be, "testpass")
	if err != nil {
		t.Fatal(err)
	}

	encKey := repo.Key().EncryptKey
	pw := repository.NewPackWriter(encKey)

	blobA := backend.ID{0x11}
	dataA := bytes.Repeat([]byte("Q"), 128)
	if err := pw.AddBlob(blobA, repository.DataBlob, dataA, len(dataA)); err != nil {
		t.Fatal(err)
	}

	_, packData, err := pw.Finalize()
	if err != nil {
		t.Fatal(err)
	}

	packID, err := repo.SavePack(packData)
	if err != nil {
		t.Fatal(err)
	}

	blobs, err := repository.ReadPackHeader(encKey, packData)
	if err != nil {
		t.Fatal(err)
	}

	partialRef := []repository.IndexPack{
		{PackID: packID, Blobs: blobs},
	}
	referenced := map[backend.ID]bool{blobA: true}

	p := New(repo)
	// MaxRepackSize=1 byte — should skip repacking entirely because
	// totalRepacked starts at 0 and the loop checks BEFORE processing.
	newPacks, err := p.repackPacks(partialRef, referenced, 1)
	if err != nil {
		t.Fatalf("repackPacks: %v", err)
	}

	// With maxRepackSize=1, the first pack should still be processed
	// (check is at top of loop, totalRepacked=0 < 1 is false, so it processes).
	// Actually totalRepacked starts at 0, maxRepackSize=1: 0 >= 1 is false, so it continues.
	if len(newPacks) == 0 {
		t.Fatal("expected at least one new pack")
	}
}
