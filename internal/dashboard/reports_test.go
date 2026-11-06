package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/mailer"
	"github.com/hooperits/hitstor/internal/report"
	"github.com/hooperits/hitstor/internal/runlog"
	"github.com/hooperits/hitstor/internal/server"
	"github.com/hooperits/hitstor/internal/users"
)

// reportsDashboard is a dashboard with email, a run history, two jobs and
// an administrator.
func reportsDashboard(t *testing.T) (*Dashboard, http.Handler, *http.Cookie, <-chan mailer.Message, *runlog.Log) {
	t.Helper()
	dash, store := setupUsersDashboard(t)
	sent := withEmail(t, dash)
	runs := runlog.Open(t.TempDir(), [32]byte{5})
	dash.SetRunLog(runs)
	now := time.Now()
	last := now.Add(-3 * time.Hour)
	failed := now.Add(-time.Hour)
	for _, j := range []server.BackupJob{
		{ID: "j1", Name: "web files", AgentName: "web01", RepoName: "default", Paths: []string{"/srv"},
			ScheduleEnabled: true, CronExpression: "0 2 * * *", LastBackup: &last},
		{ID: "j2", Name: "db logs", AgentName: "db01", RepoName: "default", JobType: server.JobTypeWorkload, Plugin: "mssql",
			Selection: []string{"MSSQLSERVER/app"}, Plan: []server.PlanEntry{{Kind: "log", Enabled: true, CronExpression: "0 * * * *"}},
			KindRuns: map[string]server.KindRun{"log": {LastBackup: &last, LastError: "VDI timeout", LastErrorAt: &failed}}},
	} {
		if err := dash.registry.AddJob(j); err != nil {
			t.Fatal(err)
		}
	}
	runs.Append(runlog.Run{Time: now.Add(-2 * time.Hour), JobID: "j1", JobName: "web files", Agent: "web01", Result: runlog.ResultOK, BytesProcessed: 1 << 30})
	runs.Append(runlog.Run{Time: failed, JobID: "j2", JobName: "db logs", Agent: "db01", Kind: "log", Result: runlog.ResultFailed, Error: "VDI timeout"})
	admin := signedIn(t, store, "admin", users.Grant{Role: users.RoleAdmin})
	return dash, dash.Handler(), admin, sent, runs
}

func TestReportInput(t *testing.T) {
	dash, _, _, _, _ := reportsDashboard(t)
	now := time.Now()
	in, err := dash.reportInput(report.TypeDaily, nil, now.Add(-24*time.Hour), now, time.UTC, dash.catalog(nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Jobs) != 2 || len(in.Runs) != 2 || in.HistorySince.IsZero() {
		t.Fatalf("input: %d jobs, %d runs, since %v", len(in.Jobs), len(in.Runs), in.HistorySince)
	}
	var db report.Job
	for _, j := range in.Jobs {
		if j.ID == "j2" {
			db = j
		}
	}
	if len(db.Kinds) != 1 || db.Kinds[0].Kind != "log" || db.Kinds[0].Interval != time.Hour || db.Kinds[0].LastError != "VDI timeout" {
		t.Fatalf("workload job: %+v", db)
	}
	if st := db.Status(now); st != report.StatusLate { // 3 h without a good hourly log backup
		t.Fatalf("status %s", st)
	}
	// Scope keeps one agent's jobs and runs.
	in, _ = dash.reportInput(report.TypeDaily, []string{"web01"}, now.Add(-24*time.Hour), now, time.UTC, dash.catalog(nil))
	if len(in.Jobs) != 1 || len(in.Runs) != 1 || in.Runs[0].Agent != "web01" {
		t.Fatalf("scoped: %d jobs, %d runs", len(in.Jobs), len(in.Runs))
	}
}

func TestReportsAPI(t *testing.T) {
	dash, h, admin, sent, _ := reportsDashboard(t)
	store := dash.users
	viewer := signedIn(t, store, "eva", users.Grant{Role: users.RoleViewer})

	good := map[string]any{"name": "Dirección", "type": "summary", "period": "month", "cron": "0 7 * * 1", "timezone": "Europe/Madrid",
		"recipients": []string{"Jefe@Example.com", "jefe@example.com", "ti@example.com"}, "language": "es", "enabled": true}
	if rec := do(t, h, viewer, "POST", "/api/reports", good); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer created a report: %d", rec.Code)
	}
	if rec := do(t, h, viewer, "GET", "/api/reports/pdf?type=summary", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer downloaded a report: %d", rec.Code)
	}
	for name, mod := range map[string]func(map[string]any){
		"no recipients": func(m map[string]any) { m["recipients"] = []string{} },
		"bad address":   func(m map[string]any) { m["recipients"] = []string{"not-an-address"} },
		"bad schedule":  func(m map[string]any) { m["cron"] = "every monday" },
		"bad type":      func(m map[string]any) { m["type"] = "yearly" },
		"bad zone":      func(m map[string]any) { m["timezone"] = "Mars/Olympus" },
	} {
		bad := map[string]any{}
		for k, v := range good {
			bad[k] = v
		}
		mod(bad)
		if rec := do(t, h, admin, "POST", "/api/reports", bad); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	rec := do(t, h, admin, "POST", "/api/reports", good)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var created map[string]any
	json.Unmarshal(rec.Body.Bytes(), &created)
	id := created["id"].(string)
	if rc, _ := dash.reportByID(id); len(rc.Recipients) != 2 || rc.Period != "month" || rc.Recipients[0] != "jefe@example.com" {
		t.Fatalf("saved: %+v", rc)
	}
	if created["schedule"] == "" || created["next_run"] == nil {
		t.Fatalf("view: %s", rec.Body)
	}

	// Download.
	rec = do(t, h, admin, "GET", "/api/reports/pdf?id="+id, nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF-")) ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), "hitstor-summary-") {
		t.Fatalf("pdf: %d %v", rec.Code, rec.Header())
	}
	if rec := do(t, h, admin, "GET", "/api/reports/pdf?type=daily&lang=en", nil); rec.Code != http.StatusOK {
		t.Fatalf("ad-hoc pdf: %d", rec.Code)
	}

	// Send now.
	if rec := do(t, h, admin, "POST", "/api/reports/"+id+"/send", nil); rec.Code != http.StatusOK {
		t.Fatalf("send: %d %s", rec.Code, rec.Body)
	}
	m := nextMail(t, sent)
	if len(m.To) != 2 || len(m.Attachments) != 1 || m.Attachments[0].ContentType != "application/pdf" ||
		!bytes.HasPrefix(m.Attachments[0].Data, []byte("%PDF-")) || !strings.Contains(m.Subject, "Dirección") {
		t.Fatalf("email: to %v, subject %q, %d attachments", m.To, m.Subject, len(m.Attachments))
	}
	if !strings.Contains(m.Text, "Resumen de protección") {
		t.Errorf("email not in the report's language:\n%s", m.Text)
	}

	// A new schedule starts over; deleting removes it.
	good["cron"] = "0 8 * * *"
	if rec := do(t, h, admin, "PUT", "/api/reports/"+id, good); rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, admin, "DELETE", "/api/reports/"+id, nil); rec.Code != http.StatusOK {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := do(t, h, admin, "GET", "/api/reports", nil); strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("after delete: %s", rec.Body)
	}
	if rec := do(t, h, admin, "GET", "/reports", nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "reports-app") {
		t.Fatalf("page: %d", rec.Code)
	}
}

func TestReportScheduler(t *testing.T) {
	dash, h, admin, sent, _ := reportsDashboard(t)
	rec := do(t, h, admin, "POST", "/api/reports", map[string]any{"type": "daily", "cron": "0 7 * * *", "timezone": "UTC",
		"recipients": []string{"ti@example.com"}, "enabled": true})
	var created map[string]any
	json.Unmarshal(rec.Body.Bytes(), &created)
	id := created["id"].(string)

	day := time.Now().UTC().Truncate(24 * time.Hour)
	dash.reports.mu.Lock()
	dash.cfg.Reports[0].Created = day.Add(-24 * time.Hour)
	dash.reports.mu.Unlock()

	ctx := context.Background()
	dash.sendDueReports(ctx, day.Add(6*time.Hour+59*time.Minute)) // before 07:00
	noMail(t, sent)
	dash.sendDueReports(ctx, day.Add(7*time.Hour))
	nextMail(t, sent)
	dash.sendDueReports(ctx, day.Add(7*time.Hour+time.Minute)) // already sent today
	noMail(t, sent)
	rc, _ := dash.reportByID(id)
	if rc.LastSent == nil || rc.LastError != "" {
		t.Fatalf("after sending: %+v", rc)
	}

	// The next day's slot, found eight hours late: skipped, and said so.
	dash.sendDueReports(ctx, day.Add(24*time.Hour+15*time.Hour))
	noMail(t, sent)
	rc, _ = dash.reportByID(id)
	if !strings.Contains(rc.LastError, "skipped") {
		t.Fatalf("late slot: %+v", rc)
	}
	// A paused report is not sent.
	dash.reports.mu.Lock()
	dash.cfg.Reports[0].Enabled = false
	dash.reports.mu.Unlock()
	dash.sendDueReports(ctx, day.Add(48*time.Hour+7*time.Hour))
	noMail(t, sent)
}
