package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/server"
)

func postWorkloadJob(t *testing.T, dash *Dashboard, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/backups", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	dash.Handler().ServeHTTP(rec, req)
	return rec
}

func TestAPICreateWorkloadJob(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()
	if _, err := dash.registry.AddAgent("db-agent"); err != nil {
		t.Fatal(err)
	}
	body := `{"name":"db nightly","agent_name":"db-agent","repo_name":"default","job_type":"workload",
		"plugin":"example","selection":["demo/alpha"],"backup_kind":"full","plugin_settings":{"size_bytes":"1024"},
		"schedule_enabled":true,"cron_expression":"0 2 * * *"}`

	// The agent has not reported the plugin yet.
	if rec := postWorkloadJob(t, dash, body); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "plugin install") {
		t.Fatalf("without plugin: %d %s", rec.Code, rec.Body)
	}

	if err := dash.registry.SetAgentPlugins("db-agent", []server.AgentPlugin{{ID: "example", Version: "0.1.0", Status: "active"}}); err != nil {
		t.Fatal(err)
	}
	rec := postWorkloadJob(t, dash, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var got backupJobJSON
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.JobType != "workload" || got.Plugin != "example" || got.Selection[0] != "demo/alpha" ||
		got.BackupKind != "full" || got.PluginSettings["size_bytes"] != "1024" || len(got.Paths) != 0 {
		t.Fatalf("job = %+v", got)
	}

	// Updating keeps the type and plugin, and changes workload parameters.
	req := httptest.NewRequest("PUT", "/api/backups/"+got.ID, strings.NewReader(`{"selection":["demo/beta"],"schedule_enabled":true}`))
	upd := httptest.NewRecorder()
	dash.Handler().ServeHTTP(upd, req)
	if upd.Code != http.StatusOK {
		t.Fatalf("update: %d %s", upd.Code, upd.Body)
	}
	// Run now works for workload jobs, which have no paths.
	run := httptest.NewRecorder()
	dash.Handler().ServeHTTP(run, httptest.NewRequest("POST", "/api/backups/"+got.ID+"/run", nil))
	if run.Code >= 400 {
		t.Fatalf("run now: %d %s", run.Code, run.Body)
	}
	job := dash.registry.GetJob(got.ID)
	if !job.RunNow {
		t.Fatal("run now not recorded")
	}
	if job.JobType != "workload" || job.Plugin != "example" || job.Selection[0] != "demo/beta" || job.BackupKind != "full" {
		t.Fatalf("after update = %+v", job)
	}
}

func TestAPICreateWorkloadJobValidation(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()
	if _, err := dash.registry.AddAgent("db-agent"); err != nil {
		t.Fatal(err)
	}
	dash.registry.SetAgentPlugins("db-agent", []server.AgentPlugin{{ID: "example", Status: "active"}})

	cases := map[string]string{
		"no selection":  `{"name":"j","agent_name":"db-agent","repo_name":"default","job_type":"workload","plugin":"example"}`,
		"unknown type":  `{"name":"j","agent_name":"db-agent","repo_name":"default","job_type":"oracle","paths":["/x"]}`,
		"files no path": `{"name":"j","agent_name":"db-agent","repo_name":"default","job_type":"files"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if rec := postWorkloadJob(t, dash, body); rec.Code != http.StatusBadRequest {
				t.Fatalf("%d %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestFailedJobShowsError(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()
	if _, err := dash.registry.AddAgent("db-agent"); err != nil {
		t.Fatal(err)
	}
	if err := dash.registry.AddJob(server.BackupJob{ID: "jf", Name: "db full", AgentName: "db-agent", RepoName: "default",
		JobType: server.JobTypeWorkload, Plugin: "mssql", Selection: []string{"MSSQLSERVER/db"}, ScheduleEnabled: true, ScheduleHours: 24}); err != nil {
		t.Fatal(err)
	}
	dash.registry.RecordJobFailure("jf", "login failed for user", time.Now())

	rec := httptest.NewRecorder()
	dash.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/backups/jf", nil))
	var got backupJobJSON
	json.NewDecoder(rec.Body).Decode(&got)
	if got.Status != "failed" || got.LastError != "login failed for user" || got.LastErrorAt == nil {
		t.Fatalf("job = status %q, error %q", got.Status, got.LastError)
	}

	page := httptest.NewRecorder()
	dash.Handler().ServeHTTP(page, httptest.NewRequest("GET", "/backups", nil))
	if body := page.Body.String(); !strings.Contains(body, "login failed for user") || !strings.Contains(body, "badge-critical") {
		t.Fatal("backups page does not show the failure")
	}
}

// TestAPIUpdateJobKeepsOmittedFields: saving one part of a job, such as its
// selection or its schedule, leaves the other parts as they were, while a
// field that is sent replaces the old value, even when empty.
func TestAPIUpdateJobKeepsOmittedFields(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()
	if _, err := dash.registry.AddAgent("db-agent"); err != nil {
		t.Fatal(err)
	}
	if err := dash.registry.SetAgentPlugins("db-agent", []server.AgentPlugin{{ID: "example", Version: "0.1.0", Status: "active"}}); err != nil {
		t.Fatal(err)
	}
	put := func(id, body string) {
		t.Helper()
		rec := httptest.NewRecorder()
		dash.Handler().ServeHTTP(rec, httptest.NewRequest("PUT", "/api/backups/"+id, strings.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT %s: %d %s", body, rec.Code, rec.Body)
		}
	}
	create := func(body string) string {
		t.Helper()
		rec := postWorkloadJob(t, dash, body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", rec.Code, rec.Body)
		}
		var got backupJobJSON
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		return got.ID
	}

	id := create(`{"name":"files","agent_name":"db-agent","repo_name":"default","job_type":"files","paths":["/srv"],
		"excludes":["*.tmp"],"use_vss":true,"schedule_enabled":true,"cron_expression":"0 2 * * *","timezone":"UTC"}`)
	put(id, `{"paths":["/srv","/etc"]}`)
	j := dash.registry.GetJob(id)
	if len(j.Paths) != 2 || len(j.Excludes) != 1 || !j.UseVSS || !j.ScheduleEnabled || j.CronExpression != "0 2 * * *" || j.Timezone != "UTC" {
		t.Fatalf("after paths update = %+v", j)
	}
	put(id, `{"schedule_enabled":false,"cron_expression":"","excludes":[]}`)
	j = dash.registry.GetJob(id)
	if len(j.Paths) != 2 || len(j.Excludes) != 0 || !j.UseVSS || j.ScheduleEnabled || j.CronExpression != "" {
		t.Fatalf("after schedule update = %+v", j)
	}

	id = create(`{"name":"db","agent_name":"db-agent","repo_name":"default","job_type":"workload","plugin":"example",
		"selection":["demo/alpha"],"backup_kind":"full","backup_options":{"copy_only":"true"},"schedule_enabled":true,"cron_expression":"0 1 * * 0"}`)
	put(id, `{"selection":["demo/beta"]}`)
	j = dash.registry.GetJob(id)
	if j.Selection[0] != "demo/beta" || j.BackupOptions["copy_only"] != "true" || j.CronExpression != "0 1 * * 0" {
		t.Fatalf("after selection update = %+v", j)
	}
	// Unchecking an option sends the options without it.
	put(id, `{"backup_kind":"log","backup_options":{}}`)
	j = dash.registry.GetJob(id)
	if j.BackupKind != "log" || len(j.BackupOptions) != 0 || j.Selection[0] != "demo/beta" {
		t.Fatalf("after options update = %+v", j)
	}
}
