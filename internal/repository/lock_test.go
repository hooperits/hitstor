package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	localbackend "github.com/hooperits/hitstor/internal/backend/local"
)

func TestInitRejectsExistingConfig(t *testing.T) {
	dir := t.TempDir()
	be, err := localbackend.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer be.Close()

	if _, err := Init(be, "pass1"); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	if _, err := Init(be, "pass2"); err == nil {
		t.Fatal("expected error when initializing existing repository")
	}
}

func TestLockWithOptionsBreaksStaleLock(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	if _, err := Init(be, "pass"); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	stale := Lock{
		Time:      time.Now().Add(-48 * time.Hour),
		Hostname:  "crashed-host",
		PID:       999999,
		Exclusive: true,
	}
	data, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	lockID := ID(sha256.Sum256(data))
	if err := be.Save(ctx, backend.LockFile, lockID, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}

	repo, err := Open(be, "pass")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Lock(true); err == nil {
		t.Fatal("expected stale lock error without BreakStale")
	}
	if err := repo.LockWithOptions(LockOptions{Exclusive: true, BreakStale: true}); err != nil {
		t.Fatalf("LockWithOptions BreakStale: %v", err)
	}
	defer repo.Unlock()

	infos, err := repo.ListLockInfo()
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Fatalf("lock count = %d, want 1", len(infos))
	}
	if infos[0].Hostname == stale.Hostname {
		t.Fatal("expected stale lock to be replaced")
	}
}

func TestLockRejectsSecondHolder(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	repo1, err := Init(be, "pass")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo1.Lock(true); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	defer repo1.Unlock()

	repo2, err := Open(be, "pass")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo2.Lock(true); err == nil {
		t.Fatal("expected lock conflict from second repository handle")
	}
}

func TestConfigExists(t *testing.T) {
	be, _ := setupTestRepo(t)
	defer be.Close()

	exists, err := ConfigExists(be)
	if err != nil || exists {
		t.Fatalf("ConfigExists empty = %v, %v", exists, err)
	}

	if _, err := Init(be, "pass"); err != nil {
		t.Fatal(err)
	}
	exists, err = ConfigExists(be)
	if err != nil || !exists {
		t.Fatalf("ConfigExists after init = %v, %v", exists, err)
	}

	_, err = be.Stat(context.Background(), backend.ConfigFile, ID{})
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
}
