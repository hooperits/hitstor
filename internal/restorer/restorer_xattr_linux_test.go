//go:build linux

package restorer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/hooperits/hitstor/internal/archiver"
	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/scanner"
)

// xattrSupported reports whether path's filesystem carries user xattrs.
func xattrSupported(t *testing.T, path string) bool {
	t.Helper()
	err := unix.Lsetxattr(path, "user.hitstor_probe", []byte("1"), 0)
	if err == nil {
		if rmErr := unix.Lremovexattr(path, "user.hitstor_probe"); rmErr != nil {
			t.Logf("cleanup probe xattr: %v", rmErr)
		}
		return true
	}
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EPERM) {
		return false
	}
	t.Fatalf("probing xattr support: %v", err)
	return false
}

// backupDirWithXattrs backs up srcDir with extended-attribute capture enabled.
func backupDirWithXattrs(t *testing.T, repo *repository.Repository, srcDir string) backend.ID {
	t.Helper()
	a := archiver.New(repo)
	a.SetScanner(scanner.NewDefaultWithXattrs())
	snapID, _, err := a.Backup([]string{srcDir}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	return snapID
}

// TestRestorePreservesXattrs is the core bare-metal-restore guarantee: metadata
// carried in extended attributes (SELinux contexts, capabilities, POSIX ACLs)
// must survive a full backup/restore cycle.
func TestRestorePreservesXattrs(t *testing.T) {
	srcDir := t.TempDir()
	dirName := filepath.Base(srcDir)

	filePath := filepath.Join(srcDir, "labeled.txt")
	if err := os.WriteFile(filePath, []byte("system file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !xattrSupported(t, filePath) {
		t.Skip("filesystem does not support user xattrs")
	}
	if err := unix.Lsetxattr(filePath, "user.hitstor_label", []byte("bmr-value"), 0); err != nil {
		t.Fatalf("set xattr on source: %v", err)
	}

	// A directory attribute must survive too — SELinux labels directories.
	subDir := filepath.Join(srcDir, "sub")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := unix.Lsetxattr(subDir, "user.hitstor_dir", []byte("dir-value"), 0); err != nil {
		t.Fatalf("set xattr on source dir: %v", err)
	}

	repo := setupRepo(t)
	snapID := backupDirWithXattrs(t, repo, srcDir)

	targetDir := t.TempDir()
	r := New(repo)
	if err := r.Restore(snapID, targetDir, nil); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	restoredFile := filepath.Join(targetDir, dirName, "labeled.txt")
	got, err := readXattr(restoredFile, "user.hitstor_label")
	if err != nil {
		t.Fatalf("reading restored file xattr: %v", err)
	}
	if string(got) != "bmr-value" {
		t.Errorf("file xattr = %q, want %q", got, "bmr-value")
	}

	restoredDir := filepath.Join(targetDir, dirName, "sub")
	got, err = readXattr(restoredDir, "user.hitstor_dir")
	if err != nil {
		t.Fatalf("reading restored dir xattr: %v", err)
	}
	if string(got) != "dir-value" {
		t.Errorf("dir xattr = %q, want %q", got, "dir-value")
	}
}

// Snapshots taken without xattr capture must still restore cleanly — the field
// is optional and older snapshots simply omit it.
func TestRestoreWithoutXattrCaptureStillWorks(t *testing.T) {
	srcDir := t.TempDir()
	dirName := filepath.Base(srcDir)

	filePath := filepath.Join(srcDir, "plain.txt")
	if err := os.WriteFile(filePath, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	repo := setupRepo(t)
	// Default scanner: no xattr capture.
	snapID := backupDir(t, repo, srcDir)

	targetDir := t.TempDir()
	r := New(repo)
	if err := r.Restore(snapID, targetDir, nil); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(targetDir, dirName, "plain.txt"))
	if err != nil {
		t.Fatalf("reading restored file: %v", err)
	}
	if string(data) != "payload" {
		t.Errorf("content = %q, want %q", data, "payload")
	}
	if r.XattrsSkipped != 0 {
		t.Errorf("XattrsSkipped = %d, want 0 when nothing was captured", r.XattrsSkipped)
	}
}

func readXattr(path, name string) ([]byte, error) {
	size, err := unix.Lgetxattr(path, name, nil)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, size)
	if _, err := unix.Lgetxattr(path, name, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
