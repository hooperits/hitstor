package dashboard

import (
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/server"
)

func TestPickSnapshotForJobPrefersJobTag(t *testing.T) {
	now := time.Now()
	all := []snapRef{
		{
			repoName: "default",
			id:       "aaaa",
			summary: cachedSnapshotSummary{
				ID:       "aaaa",
				Time:     now.Add(-2 * time.Hour),
				Hostname: "web-01",
				Tags:     []string{"job:other"},
			},
		},
		{
			repoName: "default",
			id:       "bbbb",
			summary: cachedSnapshotSummary{
				ID:       "bbbb",
				Time:     now.Add(-1 * time.Hour),
				Hostname: "web-01",
				Tags:     []string{"job:job-1"},
			},
		},
		{
			repoName: "default",
			id:       "cccc",
			summary: cachedSnapshotSummary{
				ID:       "cccc",
				Time:     now,
				Hostname: "web-01",
				Tags:     nil, // newer but no tag
			},
		},
	}
	j := server.BackupJob{ID: "job-1", AgentName: "web-01", RepoName: "default", JobType: "files"}
	got := pickSnapshotForJob(all, j)
	if got == nil || got.id != "bbbb" {
		t.Fatalf("got %#v, want snapshot bbbb (tagged job-1)", got)
	}
}

func TestPickLatestForHost(t *testing.T) {
	now := time.Now()
	all := []snapRef{
		{id: "old", summary: cachedSnapshotSummary{Hostname: "a", Time: now.Add(-time.Hour)}},
		{id: "new", summary: cachedSnapshotSummary{Hostname: "a", Time: now}},
		{id: "other", summary: cachedSnapshotSummary{Hostname: "b", Time: now.Add(time.Hour)}},
	}
	got := pickLatestForHost(all, "a")
	if got == nil || got.id != "new" {
		t.Fatalf("got %#v", got)
	}
}

func TestSanitizePathSegment(t *testing.T) {
	if sanitizePathSegment("My Job!") != "My_Job" && sanitizePathSegment("My Job!") != "MyJob" {
		// spaces → underscore
		got := sanitizePathSegment("My Job!")
		if got != "My_Job" {
			t.Fatalf("got %q", got)
		}
	}
	if sanitizePathSegment("@@@") != "unit" {
		t.Fatalf("empty sanitize = %q", sanitizePathSegment("@@@"))
	}
}

func TestDefaultDRChecklist(t *testing.T) {
	c := defaultDRChecklist("host1")
	if len(c) < 5 {
		t.Fatalf("checklist too short: %d", len(c))
	}
}

func TestDisasterManagerRoundTrip(t *testing.T) {
	m := NewDisasterManager()
	r := &DisasterRecovery{ID: "abc", SourceAgent: "a", TargetAgent: "b", Status: "running", StartTime: time.Now()}
	m.Put(r)
	if m.Get("abc") == nil {
		t.Fatal("missing")
	}
	if len(m.List()) != 1 {
		t.Fatal("list")
	}
}
