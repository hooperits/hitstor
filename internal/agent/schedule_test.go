package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/server"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGetScheduleDefault(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	schedule, err := client.GetSchedule(ctx, apiKey)
	if err != nil {
		t.Fatal(err)
	}
	if schedule.Enabled {
		t.Error("expected scheduling disabled by default")
	}
	if schedule.IntervalHours != 0 {
		t.Errorf("interval = %d, want 0", schedule.IntervalHours)
	}
	if schedule.NextRunUnix != 0 {
		t.Errorf("next_run = %d, want 0", schedule.NextRunUnix)
	}
}

func TestGetScheduleEnabled(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.AddJob(server.BackupJob{
		ID:              "aabbccdd00112233",
		Name:            "test-backup",
		AgentName:       "test-agent",
		RepoName:        "default",
		Paths:           []string{"/data"},
		ScheduleEnabled: true,
		ScheduleHours:   6,
		CreatedAt:       time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	schedule, err := client.GetSchedule(ctx, apiKey)
	if err != nil {
		t.Fatal(err)
	}
	if !schedule.Enabled {
		t.Error("expected scheduling enabled")
	}
	if schedule.IntervalHours != 6 {
		t.Errorf("interval = %d, want 6", schedule.IntervalHours)
	}
	// No last backup → next_run should be 0 (run immediately).
	if schedule.NextRunUnix != 0 {
		t.Errorf("next_run = %d, want 0 (no last backup)", schedule.NextRunUnix)
	}
	// Should also have one job in the Jobs list.
	if len(schedule.Jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(schedule.Jobs))
	}
	if schedule.Jobs[0].JobID != "aabbccdd00112233" {
		t.Errorf("job_id = %q, want %q", schedule.Jobs[0].JobID, "aabbccdd00112233")
	}
}

func TestGetScheduleNextRun(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	if err := registry.AddJob(server.BackupJob{
		ID:              "aabbccdd00112234",
		Name:            "test-backup",
		AgentName:       "test-agent",
		RepoName:        "default",
		Paths:           []string{"/data"},
		ScheduleEnabled: true,
		ScheduleHours:   12,
		LastBackup:      &now,
		CreatedAt:       now,
	}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	schedule, err := client.GetSchedule(ctx, apiKey)
	if err != nil {
		t.Fatal(err)
	}

	expectedNext := now.Add(12 * time.Hour)
	diff := schedule.NextRunUnix - expectedNext.Unix()
	if diff < -2 || diff > 2 {
		t.Errorf("next_run = %d, want ~%d (diff=%d)", schedule.NextRunUnix, expectedNext.Unix(), diff)
	}
}

func TestGetScheduleInvalidKey(t *testing.T) {
	client, _, _, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()
	_, err := client.GetSchedule(ctx, "0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil {
		t.Fatal("expected error for invalid API key")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", err)
	}
}

func TestUpdateSchedule(t *testing.T) {
	_, _, registry, cleanup := testSetup(t)
	defer cleanup()

	if _, err := registry.AddAgent("test-agent"); err != nil {
		t.Fatal(err)
	}

	if err := registry.UpdateSchedule("test-agent", true, 24); err != nil {
		t.Fatal(err)
	}

	agents := registry.ListAgents()
	for _, a := range agents {
		if a.Name == "test-agent" {
			if !a.ScheduleEnabled {
				t.Error("expected schedule enabled")
			}
			if a.ScheduleHours != 24 {
				t.Errorf("hours = %d, want 24", a.ScheduleHours)
			}
			return
		}
	}
	t.Error("agent not found")
}

func TestUpdateScheduleNotFound(t *testing.T) {
	_, _, registry, cleanup := testSetup(t)
	defer cleanup()

	err := registry.UpdateSchedule("nonexistent", true, 12)
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

func TestGetScheduleCron(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	if err := registry.AddJob(server.BackupJob{
		ID:              "cron_test_001",
		Name:            "cron-backup",
		AgentName:       "test-agent",
		RepoName:        "default",
		Paths:           []string{"/data"},
		ScheduleEnabled: true,
		CronExpression:  "0 2 * * *",
		Timezone:        "America/New_York",
		LastBackup:      &now,
		CreatedAt:       now,
	}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	schedule, err := client.GetSchedule(ctx, apiKey)
	if err != nil {
		t.Fatal(err)
	}

	if len(schedule.Jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(schedule.Jobs))
	}
	job := schedule.Jobs[0]
	if job.CronExpression != "0 2 * * *" {
		t.Errorf("cron_expression = %q, want %q", job.CronExpression, "0 2 * * *")
	}
	if job.Timezone != "America/New_York" {
		t.Errorf("timezone = %q, want %q", job.Timezone, "America/New_York")
	}
	if job.NextRunUnix <= now.Unix() {
		t.Errorf("next_run_unix = %d, should be > %d", job.NextRunUnix, now.Unix())
	}
}

func TestGetScheduleCronMissedWindow(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	// Last backup was 25 hours ago — missed the daily 2 AM window.
	lastBackup := time.Now().Add(-25 * time.Hour)
	if err := registry.AddJob(server.BackupJob{
		ID:              "cron_test_002",
		Name:            "missed-window",
		AgentName:       "test-agent",
		RepoName:        "default",
		Paths:           []string{"/data"},
		ScheduleEnabled: true,
		CronExpression:  "0 2 * * *",
		LastBackup:      &lastBackup,
		CreatedAt:       time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	schedule, err := client.GetSchedule(ctx, apiKey)
	if err != nil {
		t.Fatal(err)
	}

	if len(schedule.Jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(schedule.Jobs))
	}

	// NextRunUnix should be a past time (triggering catch-up) or next future 2 AM.
	// Either way, it should be set (non-zero).
	if schedule.Jobs[0].NextRunUnix == 0 {
		t.Error("next_run_unix = 0, expected non-zero for cron schedule")
	}
}

func TestGetScheduleMultiDayOffline(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	// Agent offline for 3+ days with daily cron — should produce only one
	// future NextRunUnix, not three.
	lastBackup := time.Now().Add(-72 * time.Hour)
	if err := registry.AddJob(server.BackupJob{
		ID:              "cron_test_003",
		Name:            "offline-catchup",
		AgentName:       "test-agent",
		RepoName:        "default",
		Paths:           []string{"/data"},
		ScheduleEnabled: true,
		CronExpression:  "0 2 * * *",
		LastBackup:      &lastBackup,
		CreatedAt:       time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	schedule, err := client.GetSchedule(ctx, apiKey)
	if err != nil {
		t.Fatal(err)
	}

	if len(schedule.Jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(schedule.Jobs))
	}

	// Only one NextRunUnix, and it should be the next 2 AM after lastBackup.
	// The agent will see now > NextRunUnix and run one catch-up backup.
	if schedule.Jobs[0].NextRunUnix == 0 {
		t.Error("next_run_unix = 0, expected non-zero")
	}
}

func TestGetScheduleInvalidTimezone(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	if err := registry.AddJob(server.BackupJob{
		ID:              "cron_test_004",
		Name:            "bad-tz",
		AgentName:       "test-agent",
		RepoName:        "default",
		Paths:           []string{"/data"},
		ScheduleEnabled: true,
		CronExpression:  "0 2 * * *",
		Timezone:        "Invalid/Timezone",
		LastBackup:      &now,
		CreatedAt:       now,
	}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	schedule, err := client.GetSchedule(ctx, apiKey)
	if err != nil {
		t.Fatal(err)
	}

	// Should gracefully fall back to UTC and still compute a next run.
	if len(schedule.Jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(schedule.Jobs))
	}
	if schedule.Jobs[0].NextRunUnix == 0 {
		t.Error("next_run_unix = 0, expected non-zero even with invalid timezone")
	}
}

func TestAddJobRejectsDatabaseJobType(t *testing.T) {
	_, _, registry, cleanup := testSetup(t)
	defer cleanup()

	if _, err := registry.AddAgent("db-agent"); err != nil {
		t.Fatal(err)
	}
	err := registry.AddJob(server.BackupJob{
		ID:              "dbjob0011223344",
		Name:            "postgres-backup",
		AgentName:       "db-agent",
		RepoName:        "default",
		Paths:           []string{"appdb"},
		ScheduleEnabled: true,
		ScheduleHours:   24,
		CreatedAt:       time.Now(),
		JobType:         "database",
	})
	if err == nil {
		t.Fatal("expected AddJob to reject database job type")
	}
	if !strings.Contains(err.Error(), "filesystem") && !strings.Contains(err.Error(), "unsupported job type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetScheduleFilesJobStillWorks(t *testing.T) {
	client, _, registry, cleanup := testSetup(t)
	defer cleanup()

	apiKey, err := registry.AddAgent("fs-agent")
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.AddJob(server.BackupJob{
		ID:              "filesjob00112233",
		Name:            "home-backup",
		AgentName:       "fs-agent",
		RepoName:        "default",
		Paths:           []string{"/home", "/etc"},
		ScheduleEnabled: true,
		ScheduleHours:   24,
		CreatedAt:       time.Now(),
		JobType:         "files",
	}); err != nil {
		t.Fatal(err)
	}

	schedule, err := client.GetSchedule(context.Background(), apiKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(schedule.Jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(schedule.Jobs))
	}
	job := schedule.Jobs[0]
	if job.JobType != "files" {
		t.Errorf("job_type = %q, want files", job.JobType)
	}
	if got := job.Paths; len(got) != 2 || got[0] != "/home" || got[1] != "/etc" {
		t.Errorf("paths = %v", got)
	}
	if !AgentJobRunnable(job) {
		t.Error("expected files job to be runnable")
	}
}
