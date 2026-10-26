//go:build linux

package fs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// writeProbe sets a user xattr and reports whether the filesystem supports it.
// Not every filesystem backing t.TempDir() carries user xattrs (tmpfs only
// gained them in Linux 6.6), so tests skip rather than fail.
func writeProbe(t *testing.T, path string) bool {
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

func TestReadXattrsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !writeProbe(t, path) {
		t.Skip("filesystem does not support user xattrs")
	}

	want := map[string][]byte{
		"user.one": []byte("first"),
		"user.two": []byte("second"),
	}
	if err := WriteXattrs(path, want); err != nil {
		t.Fatalf("WriteXattrs: %v", err)
	}

	got, err := ReadXattrs(path)
	if err != nil {
		t.Fatalf("ReadXattrs: %v", err)
	}
	for k, v := range want {
		gv, ok := got[k]
		if !ok {
			t.Errorf("xattr %q missing after round-trip", k)
			continue
		}
		if string(gv) != string(v) {
			t.Errorf("xattr %q = %q, want %q", k, gv, v)
		}
	}
}

func TestReadXattrsNoneReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bare.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadXattrs(path)
	if err != nil {
		t.Fatalf("ReadXattrs on file without xattrs: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no xattrs, got %v", got)
	}
}

func TestReadXattrsMissingFile(t *testing.T) {
	_, err := ReadXattrs(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

// WriteXattrs is best-effort: an attribute the filesystem rejects must not
// prevent the other attributes from being applied.
func TestWriteXattrsBestEffort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !writeProbe(t, path) {
		t.Skip("filesystem does not support user xattrs")
	}

	attrs := map[string][]byte{
		"user.keepme": []byte("kept"),
		// Unprivileged processes cannot set trusted.*; it must be skipped.
		"trusted.rejected": []byte("nope"),
	}
	if err := WriteXattrs(path, attrs); err != nil {
		t.Fatalf("WriteXattrs should tolerate rejected attributes: %v", err)
	}

	got, err := ReadXattrs(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got["user.keepme"]) != "kept" {
		t.Errorf("supported xattr was not applied: got %q", got["user.keepme"])
	}
}

// Symlinks must be read without following, matching the Lstat-based walk.
func TestReadXattrsDoesNotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !writeProbe(t, target) {
		t.Skip("filesystem does not support user xattrs")
	}
	if err := WriteXattrs(target, map[string][]byte{"user.target": []byte("yes")}); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	got, err := ReadXattrs(link)
	if err != nil {
		t.Fatalf("ReadXattrs on symlink: %v", err)
	}
	if _, ok := got["user.target"]; ok {
		t.Error("ReadXattrs followed the symlink; it must use the L* variants")
	}
}

func TestWriteXattrsNilIsNoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteXattrs(path, nil); err != nil {
		t.Errorf("WriteXattrs(nil) should be a no-op, got %v", err)
	}
}
