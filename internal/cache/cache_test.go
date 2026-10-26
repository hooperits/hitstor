package cache

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
)

// helper to create a temporary cache for testing.
func openTempCache(t *testing.T) (*FilesCache, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	fc, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	return fc, path
}

// helper to build a deterministic [32]byte from a string.
func hashOf(s string) [32]byte {
	return sha256.Sum256([]byte(s))
}

// helper to build a slice of chunk IDs.
func makeChunkIDs(labels ...string) [][32]byte {
	ids := make([][32]byte, len(labels))
	for i, l := range labels {
		ids[i] = hashOf(l)
	}
	return ids
}

// ---------- Tests ----------

func TestLookupFileMatch(t *testing.T) {
	fc, _ := openTempCache(t)
	defer fc.Close()

	pathHash := hashOf("/home/user/file.txt")
	var inode uint64 = 12345
	var size uint64 = 9999
	var mtimeNs int64 = 1700000000000000000

	chunks := makeChunkIDs("chunk-a", "chunk-b", "chunk-c")
	fc.UpdateFile(pathHash, inode, size, mtimeNs, chunks)

	got, ok := fc.LookupFile(pathHash, inode, size, mtimeNs)
	if !ok {
		t.Fatal("LookupFile returned ok=false, expected match")
	}
	if len(got) != len(chunks) {
		t.Fatalf("got %d chunk IDs, want %d", len(got), len(chunks))
	}
	for i := range chunks {
		if got[i] != chunks[i] {
			t.Errorf("chunk[%d] mismatch: got %x, want %x", i, got[i], chunks[i])
		}
	}
}

func TestLookupFileMismatch(t *testing.T) {
	fc, _ := openTempCache(t)
	defer fc.Close()

	pathHash := hashOf("/home/user/data.bin")
	var inode uint64 = 100
	var size uint64 = 2048
	var mtimeNs int64 = 1600000000000000000

	chunks := makeChunkIDs("c1")
	fc.UpdateFile(pathHash, inode, size, mtimeNs, chunks)

	tests := []struct {
		name    string
		inode   uint64
		size    uint64
		mtimeNs int64
	}{
		{"different inode", 999, size, mtimeNs},
		{"different size", inode, 4096, mtimeNs},
		{"different mtime", inode, size, 1700000000000000000},
		{"all different", 1, 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := fc.LookupFile(pathHash, tt.inode, tt.size, tt.mtimeNs)
			if ok {
				t.Errorf("expected ok=false for %s, got true with %d chunks", tt.name, len(got))
			}
			if got != nil {
				t.Errorf("expected nil chunk IDs for %s", tt.name)
			}
		})
	}
}

func TestUpdateFileOverwrite(t *testing.T) {
	fc, _ := openTempCache(t)
	defer fc.Close()

	pathHash := hashOf("/overwrite/test")

	// First write
	fc.UpdateFile(pathHash, 1, 100, 1000, makeChunkIDs("old-chunk"))

	// Overwrite with new metadata and new chunks
	newChunks := makeChunkIDs("new-a", "new-b")
	fc.UpdateFile(pathHash, 2, 200, 2000, newChunks)

	// Old metadata should NOT match
	_, ok := fc.LookupFile(pathHash, 1, 100, 1000)
	if ok {
		t.Error("old metadata still matches after overwrite")
	}

	// New metadata should match and return new chunk IDs
	got, ok := fc.LookupFile(pathHash, 2, 200, 2000)
	if !ok {
		t.Fatal("new metadata did not match after overwrite")
	}
	if len(got) != 2 {
		t.Fatalf("got %d chunks, want 2", len(got))
	}
	for i := range newChunks {
		if got[i] != newChunks[i] {
			t.Errorf("chunk[%d] mismatch after overwrite", i)
		}
	}
}

func TestIsKnownChunk(t *testing.T) {
	fc, _ := openTempCache(t)
	defer fc.Close()

	id1 := hashOf("known-chunk-1")
	id2 := hashOf("known-chunk-2")
	idUnknown := hashOf("unknown")

	fc.AddKnownChunk(id1, 4096)
	fc.AddKnownChunk(id2, 8192)

	if !fc.IsKnownChunk(id1) {
		t.Error("id1 should be known")
	}
	if !fc.IsKnownChunk(id2) {
		t.Error("id2 should be known")
	}
	if fc.IsKnownChunk(idUnknown) {
		t.Error("idUnknown should not be known")
	}
}

func TestCleanup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "cleanup.db")

	// Generation 1: store file-a and file-b
	fc1, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	gen1 := fc1.Generation()
	if gen1 != 1 {
		t.Fatalf("first open generation = %d, want 1", gen1)
	}

	pathA := hashOf("file-a")
	pathB := hashOf("file-b")
	fc1.UpdateFile(pathA, 1, 100, 1000, makeChunkIDs("a1"))
	fc1.UpdateFile(pathB, 2, 200, 2000, makeChunkIDs("b1"))
	fc1.Close()

	// Generation 2: only touch file-a
	fc2, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	gen2 := fc2.Generation()
	if gen2 != 2 {
		t.Fatalf("second open generation = %d, want 2", gen2)
	}
	// Lookup file-a (this updates its last_seen to gen 2)
	_, ok := fc2.LookupFile(pathA, 1, 100, 1000)
	if !ok {
		t.Fatal("file-a should match")
	}
	// file-b is NOT looked up, so its last_seen stays at gen 1
	fc2.Close()

	// Generation 3: cleanup entries not seen for 2+ generations
	fc3, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer fc3.Close()
	gen3 := fc3.Generation()
	if gen3 != 3 {
		t.Fatalf("third open generation = %d, want 3", gen3)
	}

	// maxGenerations=2 means remove entries where (current_gen - last_seen) >= 2
	// file-a: last_seen=2, current=3 => age=1 => keep
	// file-b: last_seen=1, current=3 => age=2 => remove
	if err := fc3.Cleanup(2); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	// file-a should still be present
	_, ok = fc3.LookupFile(pathA, 1, 100, 1000)
	if !ok {
		t.Error("file-a should survive cleanup")
	}

	// file-b should be gone
	_, ok = fc3.LookupFile(pathB, 2, 200, 2000)
	if ok {
		t.Error("file-b should have been removed by cleanup")
	}
}

func TestGenerationIncrement(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gen.db")

	// First open: generation should be 1
	fc1, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if g := fc1.Generation(); g != 1 {
		t.Fatalf("first open: generation = %d, want 1", g)
	}
	fc1.Close()

	// Second open: generation should be 2
	fc2, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if g := fc2.Generation(); g != 2 {
		t.Fatalf("second open: generation = %d, want 2", g)
	}
	fc2.Close()

	// Third open: generation should be 3
	fc3, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer fc3.Close()
	if g := fc3.Generation(); g != 3 {
		t.Fatalf("third open: generation = %d, want 3", g)
	}
}

func TestLookupFileNotFound(t *testing.T) {
	fc, _ := openTempCache(t)
	defer fc.Close()

	pathHash := hashOf("nonexistent")
	got, ok := fc.LookupFile(pathHash, 1, 1, 1)
	if ok {
		t.Error("expected ok=false for nonexistent path")
	}
	if got != nil {
		t.Error("expected nil for nonexistent path")
	}
}

func TestUpdateFileEmptyChunks(t *testing.T) {
	fc, _ := openTempCache(t)
	defer fc.Close()

	pathHash := hashOf("empty-file")
	fc.UpdateFile(pathHash, 1, 0, 1000, nil)

	got, ok := fc.LookupFile(pathHash, 1, 0, 1000)
	if !ok {
		t.Fatal("LookupFile should match for empty-chunk file")
	}
	if len(got) != 0 {
		t.Errorf("expected 0 chunks, got %d", len(got))
	}
}

func TestOpenInvalidPath(t *testing.T) {
	// Opening a DB at a path where the parent can't be created should fail.
	_, err := Open("/dev/null/impossible/cache.db")
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}

func TestCloseNilDB(t *testing.T) {
	// Covers the c.db == nil branch in Close.
	fc := &FilesCache{}
	if err := fc.Close(); err != nil {
		t.Fatalf("Close on nil db: %v", err)
	}
}

func TestOpenCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "cache.db")
	fc, err := Open(nested)
	if err != nil {
		t.Fatalf("Open with nested path: %v", err)
	}
	defer fc.Close()

	// Verify the file exists
	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("db file not created: %v", err)
	}
}

func BenchmarkFilesCache(b *testing.B) {
	dir := b.TempDir()
	dbPath := filepath.Join(dir, "bench.db")
	fc, err := Open(dbPath)
	if err != nil {
		b.Fatal(err)
	}
	defer fc.Close()

	pathHash := hashOf("/bench/file.txt")
	var inode uint64 = 12345
	var size uint64 = 9999
	var mtimeNs int64 = 1700000000000000000
	chunks := makeChunkIDs("chunk-a", "chunk-b", "chunk-c")

	b.Run("UpdateFile", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fc.UpdateFile(pathHash, inode, size, mtimeNs, chunks)
		}
	})

	b.Run("LookupFile", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = fc.LookupFile(pathHash, inode, size, mtimeNs)
		}
	})

	chunkID := hashOf("bench-chunk")
	b.Run("AddKnownChunk", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fc.AddKnownChunk(chunkID, 4096)
		}
	})

	b.Run("IsKnownChunk", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = fc.IsKnownChunk(chunkID)
		}
	})
}
