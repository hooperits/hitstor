package verifyrestore

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/hooperits/hitstor/internal/archiver"
	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/backend/local"
	"github.com/hooperits/hitstor/internal/repository"
)

func setupRepo(t *testing.T) *repository.Repository {
	t.Helper()
	be, err := local.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := repository.Init(be, "testpass")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { be.Close() })
	return repo
}

func backupDir(t *testing.T, repo *repository.Repository, srcDir string) backend.ID {
	t.Helper()
	a := archiver.New(repo)
	snapID, _, err := a.Backup([]string{srcDir}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	return snapID
}

func writeFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyHealthySnapshot(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "hello.txt"), []byte("Hello, HitStor verify!"), 0644)
	if err := os.MkdirAll(filepath.Join(src, "data"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(src, "data", "blob.bin"), bytes.Repeat([]byte{0xCD}, 4096), 0644)
	writeFile(t, filepath.Join(src, "empty.txt"), nil, 0644)
	if err := os.Symlink("hello.txt", filepath.Join(src, "link-to-hello")); err != nil {
		t.Fatal(err)
	}

	repo := setupRepo(t)
	snapID := backupDir(t, repo, src)

	res := Verify(repo, snapID, Options{})
	if res.Error != "" {
		t.Fatalf("verify error: %s", res.Error)
	}
	if !res.OK {
		t.Fatalf("verify not ok: mismatches=%v", res.Mismatches)
	}
	if res.FilesChecked < 3 {
		t.Fatalf("files checked = %d, want >= 3", res.FilesChecked)
	}
	if res.SymlinksOK < 1 {
		t.Fatalf("symlinks ok = %d, want >= 1", res.SymlinksOK)
	}
	if res.Sampled {
		t.Fatal("full verify should not report sampled")
	}
}

func TestVerifyDetectsMissingPack(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.txt"), bytes.Repeat([]byte("x"), 8192), 0644)

	repo := setupRepo(t)
	snapID := backupDir(t, repo, src)

	// Drop every data pack so the test restore cannot materialize files.
	idx := repo.Idx()
	if idx == nil || len(idx.Packs) == 0 {
		t.Fatal("expected packs in index")
	}
	for _, p := range idx.Packs {
		if err := repo.RemovePack(p.PackID); err != nil {
			t.Fatalf("remove pack: %v", err)
		}
	}

	res := Verify(repo, snapID, Options{})
	if res.OK {
		t.Fatal("expected verification to fail after pack removal")
	}
	if res.Error == "" && len(res.Mismatches) == 0 {
		t.Fatal("expected error or mismatches after pack removal")
	}
}

func TestVerifyDetectsContentTamper(t *testing.T) {
	// Direct unit coverage for checksum mismatch: restore succeeds, then
	// re-hash disagrees because we compare against a forged Content list.
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.txt"), []byte("good-bytes"), 0644)
	repo := setupRepo(t)
	snapID := backupDir(t, repo, src)

	// Load tree, find file node, and rewrite Content to garbage via a second
	// in-memory walk using verifyFileContent directly.
	snap, err := repo.LoadSnapshot(snapID)
	if err != nil {
		t.Fatal(err)
	}
	root, err := loadTree(repo, snap.Tree)
	if err != nil {
		t.Fatal(err)
	}
	var files []fileItem
	var symlinks []fileItem
	if err := walkCollect(repo, root, "", &files, &symlinks); err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no files")
	}
	item := files[0]
	// Write a temp file with different content than the snapshot recorded.
	tmp := filepath.Join(t.TempDir(), "tampered.txt")
	writeFile(t, tmp, []byte("TAMPERED"), 0644)
	item.node.Size = uint64(len("TAMPERED"))
	// Keep original Content IDs — size may match if we keep same length:
	writeFile(t, tmp, []byte("BAD-BYTES!"), 0644) // 10 bytes vs "good-bytes" 10 bytes
	item.node.Size = 10

	mk := repo.Key()
	cfg := repo.Cfg()
	err = verifyFileContent(tmp, item, mk.IDKey,
		uint(cfg.ChunkerParams.MinSize),
		uint(cfg.ChunkerParams.AvgSize),
		uint(cfg.ChunkerParams.MaxSize),
	)
	if err == nil {
		t.Fatal("expected checksum mismatch for tampered file")
	}
}

func TestVerifySampleLimits(t *testing.T) {
	src := t.TempDir()
	for i := 0; i < 20; i++ {
		writeFile(t, filepath.Join(src, "f"+strconv.Itoa(i)+".txt"), []byte("content-"+strconv.Itoa(i)), 0644)
	}
	repo := setupRepo(t)
	snapID := backupDir(t, repo, src)

	res := Verify(repo, snapID, Options{MaxFiles: 5})
	if res.Error != "" {
		t.Fatalf("error: %s", res.Error)
	}
	if !res.OK {
		t.Fatalf("mismatches: %v", res.Mismatches)
	}
	if !res.Sampled {
		t.Fatal("expected sampled=true")
	}
	if res.FilesChecked != 5 {
		t.Fatalf("files checked = %d, want 5", res.FilesChecked)
	}
}
