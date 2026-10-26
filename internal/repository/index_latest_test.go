package repository

import (
	"crypto/sha256"
	"testing"

	localbackend "github.com/hooperits/hitstor/internal/backend/local"
)

func TestReadAndSetLatestIndexID(t *testing.T) {
	dir := t.TempDir()
	be, err := localbackend.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer be.Close()

	_, ok, err := ReadLatestIndexID(be)
	if err != nil {
		t.Fatalf("ReadLatestIndexID: %v", err)
	}
	if ok {
		t.Fatal("expected no latest pointer initially")
	}

	id := ID(sha256.Sum256([]byte("consolidated")))
	if err := SetLatestIndexID(be, id); err != nil {
		t.Fatalf("SetLatestIndexID: %v", err)
	}

	got, ok, err := ReadLatestIndexID(be)
	if err != nil {
		t.Fatalf("ReadLatestIndexID after set: %v", err)
	}
	if !ok || got != id {
		t.Fatalf("latest = %v ok=%v, want %v", got, ok, id)
	}
}
