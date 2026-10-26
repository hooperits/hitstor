package agent

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestTranslatePathUnderRoot(t *testing.T) {
	got := translatePathUnderRoot("/data/app/file.txt", "/data", "/mnt/snap")
	want := filepath.Join("/mnt/snap", "app", "file.txt")
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("got %q want %q", got, want)
	}
	if translatePathUnderRoot("/data", "/data", "/mnt/snap") != filepath.Clean("/mnt/snap") {
		t.Fatalf("root translate failed: %q", translatePathUnderRoot("/data", "/data", "/mnt/snap"))
	}
}

func TestConsistencySessionOriginalAbsAndRelative(t *testing.T) {
	sess := &ConsistencySession{
		Method:        ConsistencyLVM,
		OriginalPaths: []string{"/var/lib/app"},
		ReadPaths:     []string{"/tmp/snap/var/lib/app"},
		snapshotRoots: []snapshotRoot{
			{originalRoot: "/var/lib/app", snapshotRoot: "/tmp/snap/var/lib/app"},
		},
	}
	// File under snapshot.
	absRead := filepath.Join("/tmp/snap/var/lib/app", "db", "x.dat")
	orig := sess.OriginalAbs(absRead)
	wantOrig := filepath.Join("/var/lib/app", "db", "x.dat")
	if filepath.Clean(orig) != filepath.Clean(wantOrig) {
		t.Fatalf("OriginalAbs = %q want %q", orig, wantOrig)
	}
	rel := sess.RelativePath(absRead)
	// makeRelativePath prepends base of root ("app")
	if rel != filepath.Join("app", "db", "x.dat") && rel != "app/db/x.dat" {
		// Windows vs unix separator
		if filepath.ToSlash(rel) != "app/db/x.dat" {
			t.Fatalf("RelativePath = %q", rel)
		}
	}
}

func TestPrepareConsistencyDisabled(t *testing.T) {
	sess, err := PrepareConsistency([]string{"/tmp"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Method != ConsistencyNone {
		t.Fatalf("method = %s", sess.Method)
	}
	if len(sess.ReadPaths) != 1 || sess.ReadPaths[0] != "/tmp" {
		t.Fatalf("read paths = %v", sess.ReadPaths)
	}
}

func TestPrepareConsistencyUnsupportedOrFallback(t *testing.T) {
	// On CI without LVM/btrfs (typical), useSnapshots=true should fall back
	// via error or none — never panic.
	sess, err := PrepareConsistency([]string{t.TempDir()}, true)
	if sess == nil {
		t.Fatal("expected non-nil session")
	}
	// Either none with error, or a real method if environment supports it.
	if err == nil && sess.Method == ConsistencyNone && runtime.GOOS == "linux" {
		// Valid: provider skipped mounts without LVM/btrfs and returned error.
		// Actually prepare returns error when nothing created.
		t.Log("unexpected success with none on linux")
	}
	if err != nil {
		t.Logf("expected fallback path: %v", err)
	}
	sess.Close()
}

func TestRelativePathForNoConsistency(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "a.txt")
	rel := relativePathFor(file, nil, []string{root})
	if filepath.ToSlash(rel) != filepath.ToSlash(filepath.Join(filepath.Base(root), "a.txt")) {
		t.Fatalf("rel = %q", rel)
	}
}
