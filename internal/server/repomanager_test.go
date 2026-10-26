package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hooperits/hitstor/internal/backend/local"
	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/repository"
)

// initTestRepo creates a temporary local repo and returns the dir and password file path.
func initTestRepo(t *testing.T, name string) (dir string, pwFile string) {
	t.Helper()

	dir = filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	be, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repository.Init(be, "test-password"); err != nil {
		t.Fatal(err)
	}
	be.Close()

	pwFile = filepath.Join(dir, "password.txt")
	if err := os.WriteFile(pwFile, []byte("test-password"), 0600); err != nil {
		t.Fatal(err)
	}

	return dir, pwFile
}

func TestRepoManagerOpenClose(t *testing.T) {
	dir, pwFile := initTestRepo(t, "repo1")

	rm := NewRepoManager()
	defer rm.CloseAll()

	cfg := config.RepoConfig{
		Name:         "repo1",
		Type:         "local",
		Path:         dir,
		PasswordFile: pwFile,
	}

	if err := rm.Open(cfg); err != nil {
		t.Fatalf("Open: %v", err)
	}

	mr, err := rm.Get("repo1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if mr.Name != "repo1" {
		t.Errorf("name = %q, want %q", mr.Name, "repo1")
	}
	if mr.Repo == nil {
		t.Error("expected non-nil Repo")
	}
	if mr.Backend == nil {
		t.Error("expected non-nil Backend")
	}
	if mr.MaintenanceLock == nil {
		t.Error("expected non-nil MaintenanceLock")
	}

	if err := rm.Close("repo1"); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := rm.Get("repo1"); err == nil {
		t.Error("expected error for closed repo")
	}
}

func TestRepoManagerOpenDuplicate(t *testing.T) {
	dir, pwFile := initTestRepo(t, "repo1")

	rm := NewRepoManager()
	defer rm.CloseAll()

	cfg := config.RepoConfig{
		Name:         "repo1",
		Type:         "local",
		Path:         dir,
		PasswordFile: pwFile,
	}

	if err := rm.Open(cfg); err != nil {
		t.Fatal(err)
	}

	err := rm.Open(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate Open")
	}
}

func TestRepoManagerGetNotFound(t *testing.T) {
	rm := NewRepoManager()

	_, err := rm.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent repo")
	}
}

func TestRepoManagerCloseNotFound(t *testing.T) {
	rm := NewRepoManager()

	err := rm.Close("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent repo")
	}
}

func TestRepoManagerList(t *testing.T) {
	dir1, pwFile1 := initTestRepo(t, "alpha")
	dir2, pwFile2 := initTestRepo(t, "beta")

	rm := NewRepoManager()
	defer rm.CloseAll()

	if err := rm.Open(config.RepoConfig{
		Name: "beta", Type: "local", Path: dir2, PasswordFile: pwFile2,
	}); err != nil {
		t.Fatal(err)
	}
	if err := rm.Open(config.RepoConfig{
		Name: "alpha", Type: "local", Path: dir1, PasswordFile: pwFile1,
	}); err != nil {
		t.Fatal(err)
	}

	list := rm.List()
	if len(list) != 2 {
		t.Fatalf("List length = %d, want 2", len(list))
	}
	// List should be sorted by name.
	if list[0].Name != "alpha" {
		t.Errorf("list[0].Name = %q, want %q", list[0].Name, "alpha")
	}
	if list[1].Name != "beta" {
		t.Errorf("list[1].Name = %q, want %q", list[1].Name, "beta")
	}
}

func TestRepoManagerCloseAll(t *testing.T) {
	dir1, pwFile1 := initTestRepo(t, "repo1")
	dir2, pwFile2 := initTestRepo(t, "repo2")

	rm := NewRepoManager()

	if err := rm.Open(config.RepoConfig{
		Name: "repo1", Type: "local", Path: dir1, PasswordFile: pwFile1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := rm.Open(config.RepoConfig{
		Name: "repo2", Type: "local", Path: dir2, PasswordFile: pwFile2,
	}); err != nil {
		t.Fatal(err)
	}

	rm.CloseAll()

	list := rm.List()
	if len(list) != 0 {
		t.Errorf("List after CloseAll = %d, want 0", len(list))
	}
}

func TestRepoManagerEncryptKey(t *testing.T) {
	dir, pwFile := initTestRepo(t, "repo1")

	rm := NewRepoManager()
	defer rm.CloseAll()

	// No key before opening.
	_, ok := rm.EncryptKey()
	if ok {
		t.Error("expected no encrypt key before opening repos")
	}

	if err := rm.Open(config.RepoConfig{
		Name: "repo1", Type: "local", Path: dir, PasswordFile: pwFile,
	}); err != nil {
		t.Fatal(err)
	}

	key, ok := rm.EncryptKey()
	if !ok {
		t.Fatal("expected encrypt key after opening repo")
	}
	if key == ([32]byte{}) {
		t.Error("expected non-zero encrypt key")
	}
}

func TestRepoManagerOpenBadPassword(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	be, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Init(be, "correct-password"); err != nil {
		t.Fatal(err)
	}
	be.Close()

	pwFile := filepath.Join(dir, "password.txt")
	if err := os.WriteFile(pwFile, []byte("wrong-password"), 0600); err != nil {
		t.Fatal(err)
	}

	rm := NewRepoManager()
	defer rm.CloseAll()

	err = rm.Open(config.RepoConfig{
		Name: "repo", Type: "local", Path: dir, PasswordFile: pwFile,
	})
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestRepoManagerOpenMissingPasswordFile(t *testing.T) {
	rm := NewRepoManager()

	err := rm.Open(config.RepoConfig{
		Name:         "repo",
		Type:         "local",
		Path:         "/tmp/nonexistent",
		PasswordFile: "/tmp/nonexistent-pw.txt",
	})
	if err == nil {
		t.Fatal("expected error for missing password file")
	}
}
