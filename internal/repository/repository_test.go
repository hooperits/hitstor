package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/backend/local"
)

func setupTestRepo(t *testing.T) (backend.Backend, string) {
	t.Helper()
	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatalf("create local backend: %v", err)
	}
	return be, dir
}

func TestInitCreatesConfigAndKey(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	repo, err := Init(be, "testpassword")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Config should have version 1 and a 64-char hex ID.
	cfg := repo.Cfg()
	if cfg.Version != 1 {
		t.Errorf("config version = %d, want 1", cfg.Version)
	}
	if len(cfg.ID) != 64 {
		t.Errorf("config ID length = %d, want 64", len(cfg.ID))
	}
	if cfg.ChunkerParams.MinSize != 256*1024 {
		t.Errorf("min_size = %d, want %d", cfg.ChunkerParams.MinSize, 256*1024)
	}

	// Key file should exist.
	keyIDs, err := be.List(context.Background(), backend.KeyFile)
	if err != nil {
		t.Fatalf("list keys: %v", err)
	}
	if len(keyIDs) != 1 {
		t.Fatalf("key count = %d, want 1", len(keyIDs))
	}

	// Master key should be non-zero.
	mk := repo.Key()
	if mk.EncryptKey == [32]byte{} {
		t.Error("encrypt key is zero")
	}
	if mk.IDKey == [32]byte{} {
		t.Error("id key is zero")
	}
}

func TestOpenWithCorrectPassword(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	orig, err := Init(be, "mypassword")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	opened, err := Open(be, "mypassword")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Config should match.
	if opened.Cfg().ID != orig.Cfg().ID {
		t.Errorf("config ID mismatch: %s vs %s", opened.Cfg().ID, orig.Cfg().ID)
	}

	// Master key should match.
	if opened.Key().EncryptKey != orig.Key().EncryptKey {
		t.Error("encrypt key mismatch after Open")
	}
	if opened.Key().IDKey != orig.Key().IDKey {
		t.Error("id key mismatch after Open")
	}
}

func TestOpenWithWrongPassword(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	if _, err := Init(be, "correctpassword"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	_, err := Open(be, "wrongpassword")
	if err == nil {
		t.Fatal("expected error with wrong password, got nil")
	}
}

func TestLockAndUnlock(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	repo, err := Init(be, "pass")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Lock.
	if err := repo.Lock(true); err != nil {
		t.Fatalf("Lock: %v", err)
	}

	// Lock file should exist.
	lockIDs, err := be.List(context.Background(), backend.LockFile)
	if err != nil {
		t.Fatalf("list locks: %v", err)
	}
	if len(lockIDs) != 1 {
		t.Fatalf("lock count = %d, want 1", len(lockIDs))
	}

	// Unlock.
	if err := repo.Unlock(); err != nil {
		t.Fatalf("Unlock: %v", err)
	}

	// Lock file should be gone.
	lockIDs, err = be.List(context.Background(), backend.LockFile)
	if err != nil {
		t.Fatalf("list locks after unlock: %v", err)
	}
	if len(lockIDs) != 0 {
		t.Errorf("lock count after unlock = %d, want 0", len(lockIDs))
	}

	// Double unlock should be no-op.
	if err := repo.Unlock(); err != nil {
		t.Fatalf("double Unlock: %v", err)
	}
}

func TestSavePackAndLoadBlob(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	repo, err := Init(be, "pass")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Build a pack with one blob.
	pw := NewPackWriter(repo.Key().EncryptKey)
	blobData := []byte("hello, repository!")
	blobID := testID("blob-42")
	if err := pw.AddBlob(blobID, DataBlob, blobData, len(blobData)); err != nil {
		t.Fatalf("AddBlob: %v", err)
	}

	_, packBytes, err := pw.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	// Save pack via repository.
	packID, err := repo.SavePack(packBytes)
	if err != nil {
		t.Fatalf("SavePack: %v", err)
	}

	// Read back the header to get the packed blob.
	blobs, err := ReadPackHeader(repo.Key().EncryptKey, packBytes)
	if err != nil {
		t.Fatalf("ReadPackHeader: %v", err)
	}
	if len(blobs) != 1 {
		t.Fatalf("blob count = %d, want 1", len(blobs))
	}

	// Load blob via repository.
	got, err := repo.LoadBlob(packID, blobs[0])
	if err != nil {
		t.Fatalf("LoadBlob: %v", err)
	}
	if string(got) != string(blobData) {
		t.Errorf("loaded blob = %q, want %q", got, blobData)
	}
}

func TestSaveAndLoadIndex(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	repo, err := Init(be, "pass")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	idx := &Index{
		Packs: []IndexPack{
			{
				PackID: testID("pack-01"),
				Blobs: []PackedBlob{
					{ID: testID("blob-10"), Type: DataBlob, Offset: 0, Length: 100, UncompressedLength: 80},
					{ID: testID("blob-11"), Type: TreeBlob, Offset: 100, Length: 50, UncompressedLength: 40},
				},
			},
		},
	}

	// Save.
	indexID, err := repo.SaveIndex(idx)
	if err != nil {
		t.Fatalf("SaveIndex: %v", err)
	}

	// Load by ID.
	loaded, err := repo.LoadIndex(indexID)
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}

	if len(loaded.Packs) != 1 {
		t.Fatalf("pack count = %d, want 1", len(loaded.Packs))
	}
	if len(loaded.Packs[0].Blobs) != 2 {
		t.Fatalf("blob count = %d, want 2", len(loaded.Packs[0].Blobs))
	}
	if loaded.Packs[0].Blobs[0].ID != testID("blob-10") {
		t.Error("blob ID mismatch")
	}
}

func TestSaveIndexMergesIntoMemory(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	repo, err := Init(be, "pass")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Initially empty.
	if len(repo.Idx().Packs) != 0 {
		t.Fatalf("initial pack count = %d, want 0", len(repo.Idx().Packs))
	}

	// Save one index.
	idx1 := &Index{Packs: []IndexPack{{PackID: testID("pack-01")}}}
	if _, err := repo.SaveIndex(idx1); err != nil {
		t.Fatalf("SaveIndex 1: %v", err)
	}
	if len(repo.Idx().Packs) != 1 {
		t.Fatalf("pack count after 1st save = %d, want 1", len(repo.Idx().Packs))
	}

	// Save another index.
	idx2 := &Index{Packs: []IndexPack{{PackID: testID("pack-02")}, {PackID: testID("pack-03")}}}
	if _, err := repo.SaveIndex(idx2); err != nil {
		t.Fatalf("SaveIndex 2: %v", err)
	}
	if len(repo.Idx().Packs) != 3 {
		t.Fatalf("pack count after 2nd save = %d, want 3", len(repo.Idx().Packs))
	}
}

func TestSaveAndLoadSnapshot(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	repo, err := Init(be, "pass")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	snap := &Snapshot{
		Time:     time.Now().Truncate(time.Second),
		Hostname: "testhost",
		Username: "testuser",
		Paths:    []string{"/home/data"},
		Tree:     testID("tree-aa"),
		Summary: SnapshotSummary{
			FilesNew:   10,
			BytesAdded: 1024,
			TotalFiles: 10,
			TotalSize:  2048,
		},
	}

	snapID, err := repo.SaveSnapshot(snap)
	if err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	loaded, err := repo.LoadSnapshot(snapID)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}

	if loaded.Hostname != "testhost" {
		t.Errorf("hostname = %q, want %q", loaded.Hostname, "testhost")
	}
	if loaded.Summary.FilesNew != 10 {
		t.Errorf("files_new = %d, want 10", loaded.Summary.FilesNew)
	}
	if !loaded.Time.Equal(snap.Time) {
		t.Errorf("time = %v, want %v", loaded.Time, snap.Time)
	}
}

func TestListSnapshots(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	repo, err := Init(be, "pass")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// No snapshots initially.
	ids, err := repo.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("initial snapshot count = %d, want 0", len(ids))
	}

	// Save two snapshots.
	snap := &Snapshot{Time: time.Now(), Hostname: "h", Paths: []string{"/"}}
	if _, err := repo.SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot 1: %v", err)
	}
	snap.Hostname = "h2"
	if _, err := repo.SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot 2: %v", err)
	}

	ids, err = repo.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("snapshot count = %d, want 2", len(ids))
	}
}

func TestLookupBlob(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	repo, err := Init(be, "pass")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	targetBlob := PackedBlob{
		ID: testID("blob-bb"), Type: DataBlob, Offset: 100, Length: 200, UncompressedLength: 150,
	}

	idx := &Index{
		Packs: []IndexPack{
			{
				PackID: testID("pack-01"),
				Blobs:  []PackedBlob{{ID: testID("blob-aa"), Type: DataBlob, Offset: 0, Length: 50}},
			},
			{
				PackID: testID("pack-02"),
				Blobs:  []PackedBlob{targetBlob},
			},
		},
	}
	if _, err := repo.SaveIndex(idx); err != nil {
		t.Fatalf("SaveIndex: %v", err)
	}

	// Found.
	blob, packID := repo.LookupBlob(testID("blob-bb"))
	if blob == nil {
		t.Fatal("expected blob, got nil")
	}
	if *packID != testID("pack-02") {
		t.Errorf("pack ID mismatch")
	}
	if blob.Offset != 100 || blob.Length != 200 {
		t.Errorf("blob offset/length = %d/%d, want 100/200", blob.Offset, blob.Length)
	}

	// Not found.
	blob, _ = repo.LookupBlob(testID("blob-ff"))
	if blob != nil {
		t.Error("expected nil for unknown blob, got non-nil")
	}
}

func TestOpenLoadsIndexes(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	repo, err := Init(be, "pass")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	idx := &Index{
		Packs: []IndexPack{
			{
				PackID: testID("pack-01"),
				Blobs: []PackedBlob{
					{ID: testID("blob-10"), Type: DataBlob, Offset: 0, Length: 100},
				},
			},
		},
	}
	if _, err := repo.SaveIndex(idx); err != nil {
		t.Fatalf("SaveIndex: %v", err)
	}

	// Re-open repository.
	reopened, err := Open(be, "pass")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Should have loaded the index.
	if len(reopened.Idx().Packs) != 1 {
		t.Fatalf("pack count = %d, want 1", len(reopened.Idx().Packs))
	}

	blob, packID := reopened.LookupBlob(testID("blob-10"))
	if blob == nil {
		t.Fatal("expected blob after re-open, got nil")
	}
	if *packID != testID("pack-01") {
		t.Errorf("pack ID mismatch after re-open")
	}
}

func TestInitRepoDir(t *testing.T) {
	be, dir := setupTestRepo(t)
	defer be.Close()

	_, err := Init(be, "pass")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Config file should exist.
	if _, err := os.Stat(dir + "/config"); err != nil {
		t.Errorf("config file missing: %v", err)
	}

	// Keys directory should have one file.
	entries, err := os.ReadDir(dir + "/keys")
	if err != nil {
		t.Fatalf("read keys dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("key file count = %d, want 1", len(entries))
	}
}

func TestIDFromHex(t *testing.T) {
	// Valid 64-char hex string.
	hexStr := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	id, err := IDFromHex(hexStr)
	if err != nil {
		t.Fatalf("IDFromHex valid: %v", err)
	}
	if id[0] != 0x01 || id[1] != 0x23 {
		t.Errorf("first bytes = %x %x, want 01 23", id[0], id[1])
	}

	// Invalid hex.
	_, err = IDFromHex("not-valid-hex")
	if err == nil {
		t.Error("expected error for invalid hex")
	}

	// Wrong length (too short).
	_, err = IDFromHex("0123456789abcdef")
	if err == nil {
		t.Error("expected error for short hex")
	}
}

func TestRemoveSnapshot(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	repo, err := Init(be, "pass")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	snap := &Snapshot{Time: time.Now(), Hostname: "h", Paths: []string{"/"}}
	snapID, err := repo.SaveSnapshot(snap)
	if err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// Verify it exists.
	ids, err := repo.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("snapshot count = %d, want 1", len(ids))
	}

	// Remove it.
	if err := repo.RemoveSnapshot(snapID); err != nil {
		t.Fatalf("RemoveSnapshot: %v", err)
	}

	// Verify it's gone.
	ids, err = repo.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots after remove: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("snapshot count after remove = %d, want 0", len(ids))
	}
}

func TestBe(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	repo, err := Init(be, "pass")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	if repo.Be() == nil {
		t.Error("Be() returned nil")
	}
}

func TestRemovePack(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	repo, err := Init(be, "pass")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Save a pack.
	pw := NewPackWriter(repo.Key().EncryptKey)
	if err := pw.AddBlob(testID("blob-01"), DataBlob, []byte("data"), 4); err != nil {
		t.Fatalf("AddBlob: %v", err)
	}
	_, packBytes, err := pw.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	packID, err := repo.SavePack(packBytes)
	if err != nil {
		t.Fatalf("SavePack: %v", err)
	}

	// Verify it exists.
	dataIDs, err := be.List(context.Background(), backend.DataFile)
	if err != nil {
		t.Fatalf("list data: %v", err)
	}
	if len(dataIDs) != 1 {
		t.Fatalf("pack count = %d, want 1", len(dataIDs))
	}

	// Remove it.
	if err := repo.RemovePack(packID); err != nil {
		t.Fatalf("RemovePack: %v", err)
	}

	// Verify it's gone.
	dataIDs, err = be.List(context.Background(), backend.DataFile)
	if err != nil {
		t.Fatalf("list data after remove: %v", err)
	}
	if len(dataIDs) != 0 {
		t.Errorf("pack count after remove = %d, want 0", len(dataIDs))
	}
}

func TestRemoveIndex(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	repo, err := Init(be, "pass")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	idx := &Index{Packs: []IndexPack{{PackID: testID("pack-01")}}}
	indexID, err := repo.SaveIndex(idx)
	if err != nil {
		t.Fatalf("SaveIndex: %v", err)
	}

	// Verify it exists.
	indexIDs, err := repo.ListIndexes()
	if err != nil {
		t.Fatalf("ListIndexes: %v", err)
	}
	if len(indexIDs) != 1 {
		t.Fatalf("index count = %d, want 1", len(indexIDs))
	}

	// Remove it.
	if err := repo.RemoveIndex(indexID); err != nil {
		t.Fatalf("RemoveIndex: %v", err)
	}

	// Verify it's gone.
	indexIDs, err = repo.ListIndexes()
	if err != nil {
		t.Fatalf("ListIndexes after remove: %v", err)
	}
	if len(indexIDs) != 0 {
		t.Errorf("index count after remove = %d, want 0", len(indexIDs))
	}
}

func TestListIndexes(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	repo, err := Init(be, "pass")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Initially empty.
	ids, err := repo.ListIndexes()
	if err != nil {
		t.Fatalf("ListIndexes: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("initial index count = %d, want 0", len(ids))
	}

	// Save two indexes.
	idx1 := &Index{Packs: []IndexPack{{PackID: testID("pack-01")}}}
	idx2 := &Index{Packs: []IndexPack{{PackID: testID("pack-02")}}}
	if _, err := repo.SaveIndex(idx1); err != nil {
		t.Fatalf("SaveIndex 1: %v", err)
	}
	if _, err := repo.SaveIndex(idx2); err != nil {
		t.Fatalf("SaveIndex 2: %v", err)
	}

	ids, err = repo.ListIndexes()
	if err != nil {
		t.Fatalf("ListIndexes: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("index count = %d, want 2", len(ids))
	}
}

func TestReplaceIndex(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	repo, err := Init(be, "pass")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Save an initial index.
	idx := &Index{Packs: []IndexPack{{PackID: testID("pack-01")}}}
	if _, err := repo.SaveIndex(idx); err != nil {
		t.Fatalf("SaveIndex: %v", err)
	}
	if len(repo.Idx().Packs) != 1 {
		t.Fatalf("pack count = %d, want 1", len(repo.Idx().Packs))
	}

	// Replace with a different index.
	newIdx := &Index{
		Packs: []IndexPack{
			{PackID: testID("pack-new-1")},
			{PackID: testID("pack-new-2")},
		},
	}
	repo.ReplaceIndex(newIdx)

	if len(repo.Idx().Packs) != 2 {
		t.Fatalf("pack count after replace = %d, want 2", len(repo.Idx().Packs))
	}
	if repo.Idx().Packs[0].PackID != testID("pack-new-1") {
		t.Error("pack ID mismatch after replace")
	}
}

func TestLoadPack(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	repo, err := Init(be, "pass")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Save a pack.
	pw := NewPackWriter(repo.Key().EncryptKey)
	if err := pw.AddBlob(testID("blob-01"), DataBlob, []byte("test-data"), 9); err != nil {
		t.Fatalf("AddBlob: %v", err)
	}
	_, packBytes, err := pw.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	packID, err := repo.SavePack(packBytes)
	if err != nil {
		t.Fatalf("SavePack: %v", err)
	}

	// Load the pack.
	loaded, err := repo.LoadPack(packID)
	if err != nil {
		t.Fatalf("LoadPack: %v", err)
	}

	if len(loaded) != len(packBytes) {
		t.Errorf("loaded size = %d, want %d", len(loaded), len(packBytes))
	}

	// Verify we can read the header from the loaded data.
	blobs, err := ReadPackHeader(repo.Key().EncryptKey, loaded)
	if err != nil {
		t.Fatalf("ReadPackHeader from LoadPack: %v", err)
	}
	if len(blobs) != 1 {
		t.Errorf("blob count = %d, want 1", len(blobs))
	}
}

func BenchmarkLookupBlob(b *testing.B) {
	sizes := []int{100, 1000, 10000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			repo := &Repository{
				idx: &Index{},
			}
			idx := &Index{}
			var targetID ID

			for p := 0; p < size/100; p++ {
				pack := IndexPack{
					PackID: ID{byte(p)},
				}
				for bl := 0; bl < 100; bl++ {
					var id ID
					id[0] = byte(p)
					id[1] = byte(bl)
					pack.Blobs = append(pack.Blobs, PackedBlob{
						ID: id,
					})
					targetID = id
				}
				idx.Packs = append(idx.Packs, pack)
			}

			repo.ReplaceIndex(idx)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = repo.LookupBlob(targetID)
			}
		})
	}
}

func BenchmarkLoadBlob(b *testing.B) {
	dir := b.TempDir()
	be, err := local.New(dir)
	if err != nil {
		b.Fatalf("create local backend: %v", err)
	}
	defer be.Close()

	repo, err := Init(be, "pass")
	if err != nil {
		b.Fatalf("Init: %v", err)
	}

	// Write a pack with a couple of blobs.
	pw := NewPackWriter(repo.Key().EncryptKey)
	blob1Data := []byte("hello, blob 1 data!")
	blob2Data := []byte("hello, blob 2 data!")
	blob1ID := testID("blob-1")
	blob2ID := testID("blob-2")
	_ = pw.AddBlob(blob1ID, DataBlob, blob1Data, len(blob1Data))
	_ = pw.AddBlob(blob2ID, DataBlob, blob2Data, len(blob2Data))

	_, packBytes, _ := pw.Finalize()
	packID, _ := repo.SavePack(packBytes)

	// Read back the header to get the packed blobs.
	blobs, _ := ReadPackHeader(repo.Key().EncryptKey, packBytes)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Alternately load Blob 1 and Blob 2 from the same pack.
		_, _ = repo.LoadBlob(packID, blobs[0])
		_, _ = repo.LoadBlob(packID, blobs[1])
	}
}
