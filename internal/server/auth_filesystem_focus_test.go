package server

import (
	"testing"
	"time"
)

func TestOrphanNonFilesystemJobsOnLoad(t *testing.T) {
	dir := t.TempDir()
	key := testEncryptKey(t)

	reg, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.AddAgent("agent-1"); err != nil {
		t.Fatal(err)
	}
	// Bypass AddJob validation by writing legacy non-files jobs directly.
	reg.mu.Lock()
	reg.jobs = append(reg.jobs,
		BackupJob{
			ID:              "m365-1",
			Name:            "mail",
			JobType:         "m365",
			RepoName:        "default",
			ScheduleEnabled: true,
			RunNow:          true,
			CreatedAt:       time.Now(),
		},
		BackupJob{
			ID:              "files-1",
			Name:            "home",
			JobType:         "files",
			AgentName:       "agent-1",
			RepoName:        "default",
			Paths:           []string{"/home"},
			ScheduleEnabled: true,
			CreatedAt:       time.Now(),
		},
	)
	if err := reg.save(); err != nil {
		reg.mu.Unlock()
		t.Fatal(err)
	}
	reg.mu.Unlock()

	reg2, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}
	jobs := reg2.ListJobs()
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2", len(jobs))
	}
	var m365, files *BackupJob
	for i := range jobs {
		switch jobs[i].ID {
		case "m365-1":
			m365 = &jobs[i]
		case "files-1":
			files = &jobs[i]
		}
	}
	if m365 == nil || files == nil {
		t.Fatalf("missing jobs: m365=%v files=%v", m365, files)
	}
	if m365.ScheduleEnabled || m365.RunNow {
		t.Fatalf("m365 job should be orphaned: schedule=%v run_now=%v", m365.ScheduleEnabled, m365.RunNow)
	}
	if !files.ScheduleEnabled {
		t.Fatal("files job should remain scheduled")
	}
}

func TestAddJobRejectsNonFilesystem(t *testing.T) {
	dir := t.TempDir()
	key := testEncryptKey(t)
	reg, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}
	err = reg.AddJob(BackupJob{ID: "x", Name: "x", JobType: "database", RepoName: "default"})
	if err == nil {
		t.Fatal("expected error for database job")
	}
}

func TestJobWantsVerifyDefaultOn(t *testing.T) {
	if !JobWantsVerify(BackupJob{}) {
		t.Fatal("nil VerifyAfterBackup should default to true")
	}
	off := false
	if JobWantsVerify(BackupJob{VerifyAfterBackup: &off}) {
		t.Fatal("explicit false should disable verify")
	}
	on := true
	if !JobWantsVerify(BackupJob{VerifyAfterBackup: &on}) {
		t.Fatal("explicit true should enable verify")
	}
}

func TestUpdateJobVerifyResult(t *testing.T) {
	dir := t.TempDir()
	key := testEncryptKey(t)
	reg, err := LoadAgentRegistry(key, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.AddAgent("a1"); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddJob(BackupJob{
		ID:        "job1",
		Name:      "home",
		AgentName: "a1",
		RepoName:  "default",
		Paths:     []string{"/home"},
		JobType:   "files",
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := reg.UpdateJobVerifyResult("job1", JobVerifyResult{
		At:       now,
		Status:   "ok",
		Snapshot: "abc",
		Detail:   "sampled 3 files",
		Files:    3,
		Bytes:    100,
	}); err != nil {
		t.Fatal(err)
	}
	job := reg.GetJob("job1")
	if job == nil || job.LastVerifyStatus != "ok" || job.LastVerifyFiles != 3 {
		t.Fatalf("unexpected job verify state: %+v", job)
	}
}
