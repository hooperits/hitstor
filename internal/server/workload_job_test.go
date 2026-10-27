package server

import (
	"context"
	"strings"
	"testing"
	"time"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
)

func TestValidateJob(t *testing.T) {
	ok := []BackupJob{
		{Paths: []string{"/data"}},
		{JobType: JobTypeFiles},
		{JobType: JobTypeWorkload, Plugin: "mssql", Selection: []string{"MSSQLSERVER/db1"}},
	}
	for _, j := range ok {
		if err := ValidateJob(j); err != nil {
			t.Errorf("%+v: %v", j, err)
		}
	}
	bad := map[string]BackupJob{
		"unknown type":    {JobType: "oracle"},
		"no plugin":       {JobType: JobTypeWorkload, Selection: []string{"x"}},
		"bad plugin id":   {JobType: JobTypeWorkload, Plugin: "../evil", Selection: []string{"x"}},
		"empty selection": {JobType: JobTypeWorkload, Plugin: "mssql"},
	}
	for name, j := range bad {
		if err := ValidateJob(j); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestWorkloadJobScheduleAndSession(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("db-agent")
	if err != nil {
		t.Fatal(err)
	}
	job := BackupJob{
		ID: "wl-job-1", Name: "db nightly", AgentName: "db-agent", RepoName: "default",
		JobType: JobTypeWorkload, Plugin: "example", Selection: []string{"demo/alpha"},
		BackupKind: "full", PluginSettings: map[string]string{"size_bytes": "10"},
		ScheduleEnabled: true, ScheduleHours: 24, CreatedAt: time.Now(),
	}
	if err := registry.AddJob(job); err != nil {
		t.Fatal(err)
	}
	// Workload jobs survive a registry reload (they are not orphaned).
	registry.orphanNonFilesystemJobs()
	if !registry.GetJob("wl-job-1").ScheduleEnabled {
		t.Fatal("workload job was disabled")
	}

	sched, err := client.GetSchedule(context.Background(), &pb.GetScheduleRequest{ApiKey: apiKey})
	if err != nil {
		t.Fatal(err)
	}
	if len(sched.Jobs) != 1 {
		t.Fatalf("jobs = %d", len(sched.Jobs))
	}
	sj := sched.Jobs[0]
	if sj.JobType != "workload" || sj.Plugin != "example" || sj.Selection[0] != "demo/alpha" ||
		sj.BackupKind != "full" || sj.PluginSettings["size_bytes"] != "10" {
		t.Fatalf("scheduled job = %+v", sj)
	}

	// Two sessions: the second must still be full (no parent snapshot).
	for i := 0; i < 2; i++ {
		begin, err := client.BeginSession(context.Background(), &pb.BeginSessionRequest{
			ApiKey: apiKey, Hostname: "db-host", Paths: []string{"workload:example"}, JobId: "wl-job-1",
		})
		if err != nil {
			t.Fatal(err)
		}
		if begin.ParentSnapshotId != "" || len(begin.PreviousFiles) != 0 {
			t.Fatalf("run %d: parent %q, %d previous files", i, begin.ParentSnapshotId, len(begin.PreviousFiles))
		}
		if _, err := client.FinalizeBackup(context.Background(), &pb.FinalizeBackupRequest{
			SessionId: begin.SessionId,
			Files:     []*pb.FileNode{{Path: "example/demo", IsDir: true, Mode: 0o755}},
		}); err != nil {
			t.Fatalf("run %d: finalize: %v", i, err)
		}
	}
}

func TestAddJobRejectsInvalidWorkload(t *testing.T) {
	_, _, registry, cleanup := testSetup(t)
	defer cleanup()
	err := registry.AddJob(BackupJob{ID: "x", JobType: JobTypeWorkload, Plugin: "example"})
	if err == nil || !strings.Contains(err.Error(), "selected object") {
		t.Fatalf("err = %v", err)
	}
}
