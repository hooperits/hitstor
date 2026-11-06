// Dashboard preview server with seeded demo data for visual QA screenshots.
// Usage: go run ./scripts/login-preview
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hooperits/hitstor/internal/backend/local"
	"github.com/hooperits/hitstor/internal/dashboard"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/server"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "19421"
	}

	dir, err := os.MkdirTemp("", "hitstor-dashboard-preview-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	be, err := local.New(dir)
	if err != nil {
		log.Fatal(err)
	}
	defer be.Close()

	repo, err := repository.Init(be, "preview-repo-password")
	if err != nil {
		log.Fatal(err)
	}

	registry, err := server.LoadAgentRegistry(repo.Key().EncryptKey, dir)
	if err != nil {
		log.Fatal(err)
	}

	if err := seedDemoData(registry, repo); err != nil {
		log.Fatal(err)
	}

	dash, err := dashboard.New(repo, registry, be, func() int { return 0 }, dashboard.Options{
		Password: "preview",
		Version:  "preview",
		RepoID:   repo.Cfg().ID,
	})
	if err != nil {
		log.Fatal(err)
	}

	rm := server.NewRepoManager()
	rm.AddDirect("default", repo, be)
	dash.SetRepoManager(rm)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: dash.Handler(),
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		srv.Close()
	}()

	fmt.Printf("dashboard preview http://127.0.0.1:%s/login (password: preview)\n", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func seedDemoData(registry *server.AgentRegistry, repo *repository.Repository) error {
	now := time.Now()
	lastBackup := now.Add(-2 * time.Hour)

	for _, name := range []string{"prod-server", "linux-agent"} {
		if _, err := registry.AddAgent(name); err != nil {
			return fmt.Errorf("add agent %s: %w", name, err)
		}
		if err := registry.UpdateLastBackup(name, lastBackup); err != nil {
			return fmt.Errorf("update last backup %s: %w", name, err)
		}
	}

	jobs := []server.BackupJob{
		{
			ID:              "a1b2c3d4e5f67890",
			Name:            "prod-files",
			AgentName:       "prod-server",
			RepoName:        "default",
			Paths:           []string{"/var/www", "/etc/nginx"},
			ScheduleEnabled: true,
			CronExpression:  "0 2 * * *",
			Timezone:        "UTC",
			LastBackup:      &lastBackup,
			CreatedAt:       now.Add(-72 * time.Hour),
			JobType:         "files",
		},
		{
			ID:              "b2c3d4e5f6789012",
			Name:            "nightly-etc",
			AgentName:       "linux-agent",
			RepoName:        "default",
			ScheduleEnabled: true,
			CronExpression:  "0 3 * * *",
			Timezone:        "UTC",
			LastBackup:      &lastBackup,
			CreatedAt:       now.Add(-48 * time.Hour),
			JobType:         "files",
		},
	}
	for _, job := range jobs {
		if err := registry.AddJob(job); err != nil {
			return fmt.Errorf("add job %s: %w", job.Name, err)
		}
	}
	if err := seedWorkloadJob(registry, now); err != nil {
		return err
	}

	snap := &repository.Snapshot{
		Time:     now.Add(-2 * time.Hour),
		Hostname: "prod-server",
		Paths:    []string{"/var/www", "/etc/nginx"},
		Summary: repository.SnapshotSummary{
			FilesNew:       42,
			FilesUnchanged: 1208,
			TotalFiles:     1250,
			TotalSize:      52428800,
			TotalDuration:  12.4,
		},
	}
	if _, err := repo.SaveSnapshot(snap); err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}

	return nil
}

// seedWorkloadJob adds a SQL Server agent and a job with a backup plan whose
// differential needs a full backup, to show the plan and its alerts.
func seedWorkloadJob(registry *server.AgentRegistry, now time.Time) error {
	if _, err := registry.AddAgent("sql-server"); err != nil {
		return fmt.Errorf("add agent sql-server: %w", err)
	}
	if err := registry.SetAgentPlugins("sql-server", []server.AgentPlugin{{
		ID: "mssql", Name: "Microsoft SQL Server", Version: "0.4.0", Status: "active",
		Capabilities: []string{"browse", "backup.full", "backup.differential", "backup.log"},
	}}); err != nil {
		return fmt.Errorf("set plugins: %w", err)
	}
	full, failed := now.Add(-26*time.Hour), now.Add(-90*time.Minute)
	job := server.BackupJob{
		ID: "c3d4e5f678901234", Name: "sales-db", AgentName: "sql-server", RepoName: "default",
		JobType: server.JobTypeWorkload, Plugin: "mssql", Selection: []string{"MSSQLSERVER/sales"},
		CreatedAt: now.Add(-72 * time.Hour), LastBackup: &full,
		Plan: []server.PlanEntry{
			{Kind: "full", Enabled: true, CronExpression: "0 1 * * 0", Timezone: "UTC"},
			{Kind: "differential", Enabled: true, CronExpression: "0 1 * * 1,2,3,4,5,6", Timezone: "UTC"},
			{Kind: "log", Enabled: true, CronExpression: "*/15 * * * *", Timezone: "UTC"},
		},
		KindRuns: map[string]server.KindRun{
			"full": {LastBackup: &full},
			"differential": {LastBackup: &full, LastErrorAt: &failed,
				LastError: `full backup required: MSSQLSERVER/sales (external-full: full of 2026-10-28 20:00 UTC by NT AUTHORITY\SYSTEM to E:\BackupBD\sales.bak)`},
			"log": {LastBackup: &failed},
		},
		ScheduleEnabled: true, BackupKind: "full",
	}
	if err := registry.AddJob(job); err != nil {
		return fmt.Errorf("add job %s: %w", job.Name, err)
	}
	return nil
}
