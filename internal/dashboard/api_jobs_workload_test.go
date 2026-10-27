package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	job := dash.registry.GetJob(got.ID)
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
