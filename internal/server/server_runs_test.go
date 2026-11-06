package server

import (
	"context"
	"testing"
	"time"

	pb "github.com/hooperits/hitstor/internal/proto/hitstor/v1"
	"github.com/hooperits/hitstor/internal/runlog"
)

func TestRunsAreRecorded(t *testing.T) {
	client, srv, _, registry, cleanup := testSetupWithServer(t)
	defer cleanup()
	runs := runlog.Open(t.TempDir(), [32]byte{9})
	srv.SetRunLog(runs)
	ctx := context.Background()

	apiKey, _ := registry.AddAgent("web01")
	if err := registry.AddJob(BackupJob{ID: "job-files", Name: "web data", AgentName: "web01", RepoName: "default", Paths: []string{"/data"}}); err != nil {
		t.Fatal(err)
	}
	if err := registry.AddJob(BackupJob{ID: "job-sql", Name: "sql", AgentName: "web01", RepoName: "default",
		JobType: JobTypeWorkload, Plugin: "mssql", Selection: []string{"MSSQLSERVER/db"}, BackupKind: "log"}); err != nil {
		t.Fatal(err)
	}
	begin, err := client.BeginSession(ctx, &pb.BeginSessionRequest{ApiKey: apiKey, Hostname: "web01", Paths: []string{"/data"}, JobId: "job-files"})
	if err != nil {
		t.Fatal(err)
	}
	fin, err := client.FinalizeBackup(ctx, &pb.FinalizeBackupRequest{SessionId: begin.SessionId, Files: []*pb.FileNode{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReportJobFailure(ctx, &pb.ReportJobFailureRequest{ApiKey: apiKey, JobId: "job-sql", Error: "VDI timeout",
		ObjectResults: []*pb.WorkloadObjectResult{{Object: "MSSQLSERVER/db", Status: "failed", Message: "log chain broken"}}}); err != nil {
		t.Fatal(err)
	}

	got, err := runs.Range(time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("runs: %+v", got)
	}
	ok, failed := got[0], got[1]
	if ok.Result != runlog.ResultOK || ok.JobName != "web data" || ok.Agent != "web01" || ok.SnapshotID != fin.SnapshotId || ok.Repo != "default" {
		t.Errorf("successful run: %+v", ok)
	}
	if failed.Result != runlog.ResultFailed || failed.Kind != "log" || failed.Plugin != "mssql" || failed.Error != "VDI timeout" ||
		len(failed.FailedObjects) != 1 || failed.FailedObjects[0] != "MSSQLSERVER/db: log chain broken" {
		t.Errorf("failed run: %+v", failed)
	}
}
