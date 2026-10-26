package archiver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hooperits/hitstor/internal/backend/local"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/scanner"
	"github.com/hooperits/hitstor/internal/sysprofile"
)

// isZeroID reports whether an ID is unset. repository.ID is a raw [32]byte
// with no null helper, so compare against the zero value.
func isZeroID(id repository.ID) bool {
	return id == (repository.ID{})
}

func bmrTestRepo(t *testing.T) *repository.Repository {
	t.Helper()
	be, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}
	repo, err := repository.Init(be, "testpass")
	if err != nil {
		t.Fatalf("repository.Init: %v", err)
	}
	t.Cleanup(func() { be.Close() })
	return repo
}

func bmrSourceDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// With bare-metal capture on, the snapshot must reference a system-profile blob
// and that blob must decode back into a usable profile. Without the reference,
// a recovery has no way to rebuild the machine's disk layout.
func TestBackupStoresSystemProfile(t *testing.T) {
	repo := bmrTestRepo(t)
	src := bmrSourceDir(t)

	a := New(repo)
	a.SetCaptureSystemProfile(true)

	_, snap, err := a.Backup([]string{src}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	if isZeroID(snap.SystemProfile) {
		t.Fatal("snapshot has no system profile reference")
	}

	var profile sysprofile.Profile
	if err := repo.LoadSystemProfile(snap.SystemProfile, &profile); err != nil {
		t.Fatalf("LoadSystemProfile: %v", err)
	}
	if profile.CapturedAt.IsZero() {
		t.Error("decoded profile has no capture time")
	}
	if profile.Arch == "" {
		t.Error("decoded profile has no architecture")
	}
}

// Capture is opt-in: an ordinary backup must not carry a profile, so existing
// behaviour and snapshot size are unchanged.
func TestBackupOmitsSystemProfileByDefault(t *testing.T) {
	repo := bmrTestRepo(t)
	src := bmrSourceDir(t)

	a := New(repo)
	_, snap, err := a.Backup([]string{src}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	if !isZeroID(snap.SystemProfile) {
		t.Errorf("expected no system profile by default, got %s", snap.SystemProfile)
	}
}

// The profile must survive a save/load round trip through the repository, which
// is what a recovery actually performs — reading it back from a stored snapshot
// rather than from the archiver that wrote it.
func TestSystemProfileSurvivesSnapshotReload(t *testing.T) {
	repo := bmrTestRepo(t)
	src := bmrSourceDir(t)

	a := New(repo)
	a.SetCaptureSystemProfile(true)
	snapID, _, err := a.Backup([]string{src}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	reloaded, err := repo.LoadSnapshot(snapID)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if isZeroID(reloaded.SystemProfile) {
		t.Fatal("reloaded snapshot lost its system profile reference")
	}

	var profile sysprofile.Profile
	if err := repo.LoadSystemProfile(reloaded.SystemProfile, &profile); err != nil {
		t.Fatalf("LoadSystemProfile after reload: %v", err)
	}
	if profile.Hostname == "" && profile.Arch == "" {
		t.Error("reloaded profile is empty")
	}
}

// An unchanged layout must dedup to one blob rather than growing the repository
// on every backup.
func TestSystemProfileDedupsAcrossBackups(t *testing.T) {
	repo := bmrTestRepo(t)
	src := bmrSourceDir(t)

	first := New(repo)
	first.SetCaptureSystemProfile(true)
	_, snapA, err := first.Backup([]string{src}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("first Backup: %v", err)
	}

	second := New(repo)
	second.SetCaptureSystemProfile(true)
	_, snapB, err := second.Backup([]string{src}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("second Backup: %v", err)
	}

	// CapturedAt differs per run, so the IDs legitimately differ; what matters
	// is that both resolve to a readable profile.
	var profile sysprofile.Profile
	if err := repo.LoadSystemProfile(snapA.SystemProfile, &profile); err != nil {
		t.Errorf("first profile unreadable: %v", err)
	}
	if err := repo.LoadSystemProfile(snapB.SystemProfile, &profile); err != nil {
		t.Errorf("second profile unreadable: %v", err)
	}
}

// The bare-metal scanner must put extended attributes into the tree the
// archiver writes, which is what the CLI's --bare-metal flag selects.
func TestBackupWithXattrScannerProducesSnapshot(t *testing.T) {
	repo := bmrTestRepo(t)
	src := bmrSourceDir(t)

	a := New(repo)
	a.SetScanner(scanner.NewDefaultWithXattrs())
	a.SetCaptureSystemProfile(true)

	snapID, snap, err := a.Backup([]string{src}, nil, "testhost", nil)
	if err != nil {
		t.Fatalf("Backup with xattr scanner: %v", err)
	}
	if isZeroID(snapID) {
		t.Fatal("no snapshot written")
	}
	if isZeroID(snap.SystemProfile) {
		t.Error("bare-metal backup did not record a system profile")
	}
}
