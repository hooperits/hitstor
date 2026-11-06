package server

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
)

func TestReportJobFailure(t *testing.T) {
	client, srv, _, registry, cleanup := testSetupWithServer(t)
	defer cleanup()
	ctx := context.Background()

	apiKey, _ := registry.AddAgent("db-agent")
	otherKey, _ := registry.AddAgent("other-agent")
	if err := registry.AddJob(BackupJob{ID: "job-f", Name: "db full", AgentName: "db-agent", RepoName: "default",
		JobType: JobTypeWorkload, Plugin: "mssql", Selection: []string{"MSSQLSERVER/db"},
		ScheduleEnabled: true, ScheduleHours: 24}); err != nil {
		t.Fatal(err)
	}
	registry.SetJobRunNow("job-f", true)

	// The agent opened a session before failing.
	if _, err := client.BeginSession(ctx, &pb.BeginSessionRequest{ApiKey: apiKey, Hostname: "h", Paths: []string{"workload:mssql"}, JobId: "job-f"}); err != nil {
		t.Fatal(err)
	}

	if srv.SessionByJobID("job-f") == nil {
		t.Fatal("session not open")
	}
	_, err := client.ReportJobFailure(ctx, &pb.ReportJobFailureRequest{ApiKey: otherKey, JobId: "job-f", Error: "x"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("other agent: %v", err)
	}

	before := time.Now()
	if _, err := client.ReportJobFailure(ctx, &pb.ReportJobFailureRequest{ApiKey: apiKey, JobId: "job-f", Error: "login failed"}); err != nil {
		t.Fatal(err)
	}
	if srv.SessionByJobID("job-f") != nil {
		t.Fatal("abandoned session was not dropped")
	}
	j := registry.GetJob("job-f")
	if j.LastError != "login failed" || j.LastErrorAt == nil || j.RunNow || !JobFailedSinceLastBackup(*j) {
		t.Fatalf("job after failure = %+v", j)
	}

	// The failed job is not due again right away.
	sched, err := client.GetSchedule(ctx, &pb.GetScheduleRequest{ApiKey: apiKey})
	if err != nil {
		t.Fatal(err)
	}
	if next := time.Unix(sched.Jobs[0].NextRunUnix, 0); next.Before(before.Add(failedJobRetryDelay - time.Second)) {
		t.Fatalf("next run %v is not after the retry delay", next)
	}

	// A successful backup clears the error.
	if err := registry.UpdateJobLastBackup("job-f", time.Now()); err != nil {
		t.Fatal(err)
	}
	if j := registry.GetJob("job-f"); j.LastError != "" || j.LastErrorAt != nil || JobFailedSinceLastBackup(*j) {
		t.Fatalf("job after success = %+v", j)
	}
}

func TestFailedCronJobWaitsForNextSlot(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()
	apiKey, _ := registry.AddAgent("db-agent")
	old := time.Now().Add(-48 * time.Hour)
	if err := registry.AddJob(BackupJob{ID: "job-c", Name: "nightly", AgentName: "db-agent", RepoName: "default",
		Paths: []string{"/data"}, ScheduleEnabled: true, CronExpression: "0 2 * * *", Timezone: "UTC", LastBackup: &old}); err != nil {
		t.Fatal(err)
	}
	failedAt := time.Now()
	registry.RecordJobFailure("job-c", "disk offline", failedAt)

	sched, err := client.GetSchedule(context.Background(), &pb.GetScheduleRequest{ApiKey: apiKey})
	if err != nil {
		t.Fatal(err)
	}
	if next := time.Unix(sched.Jobs[0].NextRunUnix, 0); !next.After(failedAt) {
		t.Fatalf("next run %v should be after the failure at %v, not overdue", next, failedAt)
	}
}
