package server

import (
	"testing"
	"time"
)

// planJob is a SQL Server-like plan: fulls on Sunday, differentials the
// other days, logs every 15 minutes.
func planJob() BackupJob {
	return BackupJob{
		ID: "plan", JobType: JobTypeWorkload, Plugin: "mssql", Selection: []string{"MSSQLSERVER/db"},
		Plan: []PlanEntry{
			{Kind: "full", Enabled: true, CronExpression: "0 1 * * 0", Timezone: "UTC"},
			{Kind: "differential", Enabled: true, CronExpression: "0 1 * * 1-6", Timezone: "UTC"},
			{Kind: "log", Enabled: true, CronExpression: "*/15 * * * *", Timezone: "UTC"},
		},
	}
}

func ptr(t time.Time) *time.Time { return &t }

func TestDueKind(t *testing.T) {
	sun := time.Date(2026, 11, 1, 1, 0, 30, 0, time.UTC) // Sunday 01:00:30
	j := planJob()
	// Never ran: Sunday 01:00 is a full and a log slot; the full goes first.
	if kind, at, ok := DueKind(j, sun); !ok || kind != "full" || !at.Equal(sun.Truncate(time.Minute)) {
		t.Fatalf("got %s %v %v, want full at 01:00", kind, at, ok)
	}
	// After the full, the log slot at 01:00 is still due.
	j.KindRuns = map[string]KindRun{"full": {LastBackup: ptr(sun)}, "differential": {LastBackup: ptr(sun)}}
	if kind, _, _ := DueKind(j, sun); kind != "log" {
		t.Fatalf("after full: got %s, want log", kind)
	}
	// Monday: the differential at 01:00 beats the log at 01:00 by rank.
	mon := sun.Add(24 * time.Hour)
	j.KindRuns["log"] = KindRun{LastBackup: ptr(mon.Add(-time.Minute))}
	if kind, _, _ := DueKind(j, mon); kind != "differential" {
		t.Fatalf("monday: got %s, want differential", kind)
	}
	// A failed differential waits for its next slot, not the next poll.
	j.KindRuns["differential"] = KindRun{LastBackup: ptr(sun), LastErrorAt: ptr(mon), LastError: "full backup required: x"}
	j.KindRuns["log"] = KindRun{LastBackup: ptr(mon)}
	if kind, at, _ := DueKind(j, mon.Add(time.Minute)); kind != "log" || !at.Equal(mon.Truncate(time.Minute).Add(15*time.Minute)) {
		t.Fatalf("after failed diff: got %s at %v, want log at 01:15", kind, at)
	}
	// A manual run takes the requested kind.
	j.RunNow, j.RunNowKind = true, "full"
	if kind, _, _ := DueKind(j, mon); kind != "full" {
		t.Fatalf("run now: got %s, want full", kind)
	}
}

func TestDueKindOnDemandOnly(t *testing.T) {
	j := planJob()
	for i := range j.Plan {
		j.Plan[i].Enabled = false
	}
	if _, _, ok := DueKind(j, time.Now()); ok {
		t.Fatal("an on-demand plan is never due")
	}
	j.RunNow = true
	if kind, _, ok := DueKind(j, time.Now()); !ok || kind != "full" {
		t.Fatalf("run now without a kind runs the plan's first kind; got %s", kind)
	}
}

func TestEffectivePlanOfLegacyJob(t *testing.T) {
	last := time.Date(2026, 10, 29, 6, 0, 0, 0, time.UTC)
	j := BackupJob{JobType: JobTypeWorkload, BackupKind: "log", ScheduleEnabled: true, CronExpression: "*/15 * * * *", Timezone: "UTC", LastBackup: &last}
	plan := EffectivePlan(j)
	if len(plan) != 1 || plan[0].Kind != "log" || !plan[0].Enabled {
		t.Fatalf("plan = %+v", plan)
	}
	if st := KindState(j, "log"); st.LastBackup == nil || !st.LastBackup.Equal(last) {
		t.Fatalf("legacy history lost: %+v", st)
	}
	if st := KindState(j, "full"); st.LastBackup != nil {
		t.Fatalf("other kinds have no history: %+v", st)
	}
	if EffectivePlan(BackupJob{JobType: JobTypeFiles}) != nil {
		t.Fatal("filesystem jobs have no plan")
	}
}

func TestValidatePlan(t *testing.T) {
	kinds := []string{"full", "differential", "log"}
	if err := ValidatePlan(planJob().Plan, kinds); err != nil {
		t.Fatal(err)
	}
	bad := [][]PlanEntry{
		{{Kind: "incremental", Enabled: true, CronExpression: "0 1 * * *"}},
		{{Kind: "full", Enabled: true, CronExpression: "0 1 * * *"}, {Kind: "full", CronExpression: ""}},
		{{Kind: "log", Enabled: true, CronExpression: "nope"}},
		{{Kind: "log", Timezone: "Mars/Olympus"}},
	}
	for _, p := range bad {
		if ValidatePlan(p, kinds) == nil {
			t.Errorf("accepted %+v", p)
		}
	}
}

func TestRecordKindRuns(t *testing.T) {
	r, err := LoadAgentRegistry([32]byte{1}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.AddAgent("sql01"); err != nil {
		t.Fatal(err)
	}
	j := planJob()
	j.Name, j.AgentName, j.RepoName = "db plan", "sql01", "default"
	if err := r.AddJob(j); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 1, 1, 5, 0, 0, time.UTC)
	if err := r.RecordKindFailure("plan", "differential", "full backup required: MSSQLSERVER/db (no-full)", at); err != nil {
		t.Fatal(err)
	}
	if err := r.RecordKindBackup("plan", "full", at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got := r.GetJob("plan")
	if st := got.KindRuns["full"]; st.LastBackup == nil || st.LastErrorAt != nil {
		t.Errorf("full: %+v", st)
	}
	// The full fills the differential's slot and clears its failure.
	if st := got.KindRuns["differential"]; st.failed() || st.LastBackup == nil {
		t.Errorf("differential after full: %+v", st)
	}
	if got.LastError != "" || got.RunNow {
		t.Errorf("job-level state not cleared: %+v", got)
	}
}
