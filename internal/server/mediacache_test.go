package server

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMediaCacheReportsNothingBeforeAnyBuild(t *testing.T) {
	cache := NewMediaCache(t.TempDir())

	_, err := cache.Get("web01")
	if !errors.Is(err, ErrNoMedia) {
		t.Fatalf("err = %v, want ErrNoMedia so callers can 404 rather than 500", err)
	}
}

func TestMediaCacheStoresAndReadsBack(t *testing.T) {
	cache := NewMediaCache(t.TempDir())

	w, err := cache.Writer("web01")
	if err != nil {
		t.Fatalf("Writer: %v", err)
	}
	if _, err := w.Write([]byte("iso-bytes")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := w.Commit()
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if info.SizeBytes != int64(len("iso-bytes")) {
		t.Errorf("SizeBytes = %d, want %d", info.SizeBytes, len("iso-bytes"))
	}
	if info.BuiltAt.IsZero() {
		t.Error("BuiltAt is zero; the dashboard flags stale media by its age")
	}

	got, err := cache.Get("web01")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	data, err := os.ReadFile(got.Path)
	if err != nil {
		t.Fatalf("read cached media: %v", err)
	}
	if string(data) != "iso-bytes" {
		t.Errorf("cached media = %q, want %q", data, "iso-bytes")
	}
}

func TestMediaCacheKeepsThePreviousImageUntilTheNewOneIsWhole(t *testing.T) {
	cache := NewMediaCache(t.TempDir())

	first, err := cache.Writer("web01")
	if err != nil {
		t.Fatal(err)
	}
	first.Write([]byte("good-iso"))
	if _, err := first.Commit(); err != nil {
		t.Fatal(err)
	}

	// A rebuild that dies partway must not destroy the media an operator
	// already has: bare-metal restore exists for when the host is gone, and
	// that is exactly when a rebuild is most likely to fail.
	second, err := cache.Writer("web01")
	if err != nil {
		t.Fatal(err)
	}
	second.Write([]byte("partial"))
	if err := second.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}

	got, err := cache.Get("web01")
	if err != nil {
		t.Fatalf("Get after aborted rebuild: %v", err)
	}
	data, _ := os.ReadFile(got.Path)
	if string(data) != "good-iso" {
		t.Errorf("cached media = %q; an aborted rebuild clobbered the good image", data)
	}
}

func TestMediaCacheOverwritesOnRebuild(t *testing.T) {
	dir := t.TempDir()
	cache := NewMediaCache(dir)

	for _, body := range []string{"first", "second"} {
		w, err := cache.Writer("web01")
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
		if _, err := w.Commit(); err != nil {
			t.Fatal(err)
		}
	}

	got, err := cache.Get("web01")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(got.Path)
	if string(data) != "second" {
		t.Errorf("cached media = %q, want the newest build", data)
	}

	// One image per host, not a version history: ISOs are large enough that
	// silently accumulating them fills the disk the repository lives on.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var isos int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".iso") {
			isos++
		}
	}
	if isos != 1 {
		t.Errorf("%d cached images for one host, want 1", isos)
	}
}

func TestMediaCacheRejectsHostNamesThatEscapeTheCacheDirectory(t *testing.T) {
	dir := t.TempDir()
	cache := NewMediaCache(dir)

	// Agent names reach this from the registry, but a path-shaped one must
	// never let a write or a download land outside the cache.
	for _, host := range []string{"../escape", "a/b", `..\escape`, "", ".", ".."} {
		t.Run(host, func(t *testing.T) {
			if _, err := cache.Writer(host); err == nil {
				t.Errorf("Writer(%q) accepted a name that escapes the cache", host)
			}
			if _, err := cache.Get(host); err == nil {
				t.Errorf("Get(%q) accepted a name that escapes the cache", host)
			}
		})
	}
}

func TestMediaCacheListsWhatItHolds(t *testing.T) {
	cache := NewMediaCache(t.TempDir())

	for _, host := range []string{"web01", "db02"} {
		w, err := cache.Writer(host)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(host))
		if _, err := w.Commit(); err != nil {
			t.Fatal(err)
		}
	}

	all := cache.List()
	if len(all) != 2 {
		t.Fatalf("List returned %d entries, want 2", len(all))
	}
	byHost := map[string]MediaInfo{}
	for _, m := range all {
		byHost[m.Host] = m
	}
	if _, ok := byHost["web01"]; !ok {
		t.Errorf("web01 missing from %v", all)
	}
	if got := byHost["db02"].SizeBytes; got != int64(len("db02")) {
		t.Errorf("db02 SizeBytes = %d, want %d", got, len("db02"))
	}
}

func TestMediaCacheRemovesAnImage(t *testing.T) {
	cache := NewMediaCache(t.TempDir())

	w, _ := cache.Writer("web01")
	w.Write([]byte("iso"))
	w.Commit()

	if err := cache.Remove("web01"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := cache.Get("web01"); !errors.Is(err, ErrNoMedia) {
		t.Errorf("err = %v after Remove, want ErrNoMedia", err)
	}
	// Removing media a host never had is the same end state, not a failure.
	if err := cache.Remove("never-built"); err != nil {
		t.Errorf("Remove of absent media: %v", err)
	}
}

func TestMediaCacheAgeIsMeasurableFromBuiltAt(t *testing.T) {
	cache := NewMediaCache(t.TempDir())

	w, _ := cache.Writer("web01")
	w.Write([]byte("iso"))
	before := time.Now().Add(-time.Second)
	info, err := w.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if info.BuiltAt.Before(before) {
		t.Errorf("BuiltAt %v predates the build", info.BuiltAt)
	}
	if filepath.Base(info.Path) == "" {
		t.Error("cached media has no path")
	}
}
