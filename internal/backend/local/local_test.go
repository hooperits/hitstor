package local

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hooperits/hitstor/internal/backend"
)

// testID creates a deterministic backend.ID from a string by hashing it.
func testID(s string) backend.ID {
	return sha256.Sum256([]byte(s))
}

func TestSaveThenLoad(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	id := testID("test-save-load")
	data := []byte("hello, hitstor backup data")

	if err := b.Save(ctx, backend.DataFile, id, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	rc, err := b.Load(ctx, backend.DataFile, id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if !bytes.Equal(got, data) {
		t.Errorf("Load returned %q, want %q", got, data)
	}
}

func TestStatReturnsCorrectSize(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	id := testID("test-stat")
	data := []byte("some test data for stat")

	if err := b.Save(ctx, backend.DataFile, id, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	fi, err := b.Stat(ctx, backend.DataFile, id)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	if fi.Size != int64(len(data)) {
		t.Errorf("Stat size = %d, want %d", fi.Size, len(data))
	}
}

func TestListReturnsSavedIDs(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	id1 := testID("list-item-1")
	id2 := testID("list-item-2")
	id3 := testID("list-item-3")

	for _, id := range []backend.ID{id1, id2, id3} {
		if err := b.Save(ctx, backend.IndexFile, id, bytes.NewReader([]byte("data"))); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	ids, err := b.List(ctx, backend.IndexFile)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(ids) != 3 {
		t.Fatalf("List returned %d IDs, want 3", len(ids))
	}

	expected := map[backend.ID]bool{id1: true, id2: true, id3: true}
	for _, id := range ids {
		if !expected[id] {
			t.Errorf("List returned unexpected ID %s", id)
		}
		delete(expected, id)
	}
	for id := range expected {
		t.Errorf("List missing expected ID %s", id)
	}
}

func TestRemoveDeletesFile(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	id := testID("test-remove")
	data := []byte("data to remove")

	if err := b.Save(ctx, backend.DataFile, id, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify it exists first.
	rc, err := b.Load(ctx, backend.DataFile, id)
	if err != nil {
		t.Fatalf("Load before Remove: %v", err)
	}
	rc.Close()

	if err := b.Remove(ctx, backend.DataFile, id); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Load should now fail.
	_, err = b.Load(ctx, backend.DataFile, id)
	if err == nil {
		t.Error("Load after Remove should have returned error, got nil")
	}
}

func TestDataFileUsesFanOutSubdirectories(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	id := testID("test-fanout")
	idHex := id.String()
	data := []byte("fan-out test data")

	if err := b.Save(ctx, backend.DataFile, id, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify the file is stored at data/{first_two_hex_chars}/{full_hex_id}.
	expectedPath := filepath.Join(dir, "data", idHex[:2], idHex)
	if _, err := os.Stat(expectedPath); err != nil {
		t.Errorf("expected file at %s, got error: %v", expectedPath, err)
	}

	// Also verify the fan-out subdirectory exists.
	fanOutDir := filepath.Join(dir, "data", idHex[:2])
	fi, err := os.Stat(fanOutDir)
	if err != nil {
		t.Fatalf("fan-out directory %s: %v", fanOutDir, err)
	}
	if !fi.IsDir() {
		t.Errorf("%s is not a directory", fanOutDir)
	}
}

func TestSaveToAllFileTypes(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()

	fileTypes := []struct {
		ft   backend.FileType
		name string
	}{
		{backend.DataFile, "data"},
		{backend.IndexFile, "index"},
		{backend.SnapshotFile, "snapshot"},
		{backend.KeyFile, "key"},
		{backend.LockFile, "lock"},
		{backend.ConfigFile, "config"},
	}

	for _, tc := range fileTypes {
		t.Run(tc.name, func(t *testing.T) {
			id := testID("filetype-" + tc.name)
			data := []byte("data for " + tc.name)

			if err := b.Save(ctx, tc.ft, id, bytes.NewReader(data)); err != nil {
				t.Fatalf("Save(%s): %v", tc.name, err)
			}

			rc, err := b.Load(ctx, tc.ft, id)
			if err != nil {
				t.Fatalf("Load(%s): %v", tc.name, err)
			}
			defer rc.Close()

			got, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("ReadAll(%s): %v", tc.name, err)
			}

			if !bytes.Equal(got, data) {
				t.Errorf("Load(%s) = %q, want %q", tc.name, got, data)
			}
		})
	}
}

func TestLoadNonExistentReturnsError(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	id := testID("does-not-exist")

	_, err = b.Load(ctx, backend.DataFile, id)
	if err == nil {
		t.Error("Load of non-existent ID should return error, got nil")
	}
}

func TestListDataFilesWithFanOut(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	id1 := testID("data-list-1")
	id2 := testID("data-list-2")

	for _, id := range []backend.ID{id1, id2} {
		if err := b.Save(ctx, backend.DataFile, id, bytes.NewReader([]byte("data"))); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	ids, err := b.List(ctx, backend.DataFile)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(ids) != 2 {
		t.Fatalf("List returned %d IDs, want 2", len(ids))
	}

	expected := map[backend.ID]bool{id1: true, id2: true}
	for _, id := range ids {
		if !expected[id] {
			t.Errorf("List returned unexpected ID %s", id)
		}
		delete(expected, id)
	}
	for id := range expected {
		t.Errorf("List missing expected ID %s", id)
	}
}

func TestBasePath(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := b.BasePath(); got != dir {
		t.Errorf("BasePath() = %q, want %q", got, dir)
	}
}

func TestRemoveNonExistent(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	id := testID("does-not-exist-remove")
	err = b.Remove(ctx, backend.DataFile, id)
	if err == nil {
		t.Error("Remove of non-existent ID should return error")
	}
}

func TestListEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	ids, err := b.List(ctx, backend.SnapshotFile)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("List empty = %d IDs, want 0", len(ids))
	}
}

func TestListConfigFile(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	ids, err := b.List(ctx, backend.ConfigFile)
	if err != nil {
		t.Fatalf("List ConfigFile: %v", err)
	}
	if ids != nil {
		t.Errorf("List ConfigFile should return nil, got %v", ids)
	}
}

func TestSaveOverwrite(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	id := testID("overwrite-test")

	if err := b.Save(ctx, backend.DataFile, id, bytes.NewReader([]byte("original"))); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := b.Save(ctx, backend.DataFile, id, bytes.NewReader([]byte("updated"))); err != nil {
		t.Fatalf("Save overwrite: %v", err)
	}

	rc, err := b.Load(ctx, backend.DataFile, id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if string(got) != "updated" {
		t.Errorf("after overwrite, got %q, want %q", got, "updated")
	}
}

func TestStatNonExistent(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	id := testID("no-such-stat")
	_, err = b.Stat(ctx, backend.DataFile, id)
	if err == nil {
		t.Error("Stat of non-existent ID should return error")
	}
}

func TestListDataWithJunkFile(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	ctx := context.Background()

	// Save one valid data file.
	id := testID("valid-data-junk")
	if err := b.Save(ctx, backend.DataFile, id, bytes.NewReader([]byte("data"))); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Create a junk non-hex file in a fanout dir to trigger "skip" path.
	fanOutDir := filepath.Join(dir, "data", id.String()[:2])
	if err := os.WriteFile(filepath.Join(fanOutDir, "not-a-hex-id"), []byte("junk"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a stray file (not a dir) directly in data/ to trigger !IsDir() skip.
	if err := os.WriteFile(filepath.Join(dir, "data", "stray-file"), []byte("stray"), 0644); err != nil {
		t.Fatal(err)
	}

	ids, err := b.List(ctx, backend.DataFile)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Should only return the one valid ID.
	if len(ids) != 1 {
		t.Errorf("List = %d IDs, want 1", len(ids))
	}
}
