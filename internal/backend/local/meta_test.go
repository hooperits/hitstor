package local

import (
	"crypto/sha256"
	"testing"

	"github.com/hooperits/hitstor/internal/backend"
)

func TestIndexLatestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	lb, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer lb.Close()

	_, ok, err := lb.ReadIndexLatest()
	if err != nil {
		t.Fatalf("ReadIndexLatest missing: %v", err)
	}
	if ok {
		t.Fatal("expected no latest pointer initially")
	}

	id := sha256.Sum256([]byte("consolidated-index"))
	if err := lb.WriteIndexLatest(id); err != nil {
		t.Fatalf("WriteIndexLatest: %v", err)
	}

	got, ok, err := lb.ReadIndexLatest()
	if err != nil {
		t.Fatalf("ReadIndexLatest: %v", err)
	}
	if !ok {
		t.Fatal("expected latest pointer")
	}
	if got != backend.ID(id) {
		t.Fatalf("latest id = %s, want %s", got, backend.ID(id))
	}
}
