package local

import (
	"testing"
)

func TestDiskUsage(t *testing.T) {
	dir := t.TempDir()
	be, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	total, free, err := be.DiskUsage()
	if err != nil {
		t.Fatalf("DiskUsage: %v", err)
	}

	if total == 0 {
		t.Error("expected non-zero total")
	}
	if free == 0 {
		t.Error("expected non-zero free")
	}
	if free > total {
		t.Errorf("free (%d) > total (%d)", free, total)
	}
}

func TestDiskUsageInvalidPath(t *testing.T) {
	be := &Local{basePath: "/nonexistent/path/that/does/not/exist"}
	_, _, err := be.DiskUsage()
	if err == nil {
		t.Error("expected error for invalid path")
	}
}
