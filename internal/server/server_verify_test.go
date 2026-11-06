package server

import (
	"strings"
	"testing"

	"github.com/hooperits/hitstor/internal/verifyrestore"
)

// FinalizeBackup reports a short snapshot ID; the verification after a
// job's backup must still find the snapshot.
func TestRestoreVerifyWithShortSnapshotID(t *testing.T) {
	client, srv, repo, registry, cleanup := testSetupWithServer(t)
	defer cleanup()
	apiKey, err := registry.AddAgent("verify-host")
	if err != nil {
		t.Fatal(err)
	}
	short := simulateAgentBackup(t, client, apiKey, "verify-host", map[string][]byte{
		"a.txt": []byte("alpha contents"),
		"b.txt": []byte("bravo contents, a little longer"),
	})
	if len(short) >= 64 {
		t.Fatalf("expected a short ID, got %q", short)
	}
	if err := registry.AddJob(BackupJob{ID: "jv", Name: "v", AgentName: "verify-host", RepoName: "default", Paths: []string{"/data"}}); err != nil {
		t.Fatal(err)
	}

	srv.runRestoreVerify("jv", "default", short)

	job := registry.GetJob("jv")
	if job.LastVerifyStatus != verifyrestore.StatusOK || job.LastVerifyFiles != 2 {
		t.Fatalf("verify = %q (%s), files %d", job.LastVerifyStatus, job.LastVerifyDetail, job.LastVerifyFiles)
	}
	if len(job.LastVerifySnap) != 64 || !strings.HasPrefix(job.LastVerifySnap, short) {
		t.Fatalf("recorded snapshot %q", job.LastVerifySnap)
	}

	for _, bad := range []string{"zz", "", "0000000000000000"} {
		if _, err := resolveSnapshotID(repo, bad); err == nil {
			t.Errorf("resolveSnapshotID(%q) accepted", bad)
		}
	}
}
