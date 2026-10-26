package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hooperits/hitstor/internal/backend/local"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/server"
)

func setupTestMetrics(t *testing.T, addAgents bool) (*Metrics, func()) {
	t.Helper()

	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatalf("create backend: %v", err)
	}

	repo, err := repository.Init(be, "test-password")
	if err != nil {
		be.Close()
		t.Fatalf("init repo: %v", err)
	}

	registry, err := server.LoadAgentRegistry(repo.Key().EncryptKey, dir)
	if err != nil {
		be.Close()
		t.Fatalf("load registry: %v", err)
	}

	if addAgents {
		if _, err := registry.AddAgent("agent-alpha"); err != nil {
			be.Close()
			t.Fatalf("add agent: %v", err)
		}
		if _, err := registry.AddAgent("agent-beta"); err != nil {
			be.Close()
			t.Fatalf("add agent: %v", err)
		}
	}

	counters := func() (uint64, uint64, uint64, uint64) {
		return 10, 2, 1048576, 500
	}

	m := New(repo, registry, be, func() int { return 3 }, counters)
	cleanup := func() { be.Close() }
	return m, cleanup
}

func getMetricsBody(t *testing.T, m *Metrics) string {
	t.Helper()
	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want %d", rec.Code, http.StatusOK)
	}
	return rec.Body.String()
}

func TestMetricsContentType(t *testing.T) {
	m, cleanup := setupTestMetrics(t, false)
	defer cleanup()

	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != contentType {
		t.Errorf("Content-Type = %q, want %q", ct, contentType)
	}
}

func TestMetricsContainsAllMetrics(t *testing.T) {
	m, cleanup := setupTestMetrics(t, false)
	defer cleanup()

	body := getMetricsBody(t, m)

	metrics := []string{
		"hitstor_snapshots_total",
		"hitstor_packs_total",
		"hitstor_indexes_total",
		"hitstor_repository_size_bytes",
		"hitstor_active_sessions",
		"hitstor_registered_agents",
		"hitstor_agents_ok",
		"hitstor_agents_stale",
		"hitstor_agents_never",
		"hitstor_backups_completed_total",
		"hitstor_backups_failed_total",
		"hitstor_bytes_received_total",
		"hitstor_chunks_received_total",
		"hitstor_agent_schedule_enabled",
		"hitstor_agent_schedule_interval_hours",
	}

	for _, name := range metrics {
		if !strings.Contains(body, name) {
			t.Errorf("metrics output missing %q", name)
		}
	}
}

func TestMetricsTypeAnnotations(t *testing.T) {
	m, cleanup := setupTestMetrics(t, false)
	defer cleanup()

	body := getMetricsBody(t, m)

	gauges := []string{
		"hitstor_snapshots_total",
		"hitstor_packs_total",
		"hitstor_indexes_total",
		"hitstor_repository_size_bytes",
		"hitstor_active_sessions",
		"hitstor_registered_agents",
		"hitstor_agents_ok",
		"hitstor_agents_stale",
		"hitstor_agents_never",
	}
	for _, name := range gauges {
		expected := "# TYPE " + name + " gauge"
		if !strings.Contains(body, expected) {
			t.Errorf("missing type annotation: %q", expected)
		}
	}

	counters := []string{
		"hitstor_backups_completed_total",
		"hitstor_backups_failed_total",
		"hitstor_bytes_received_total",
		"hitstor_chunks_received_total",
	}
	for _, name := range counters {
		expected := "# TYPE " + name + " counter"
		if !strings.Contains(body, expected) {
			t.Errorf("missing type annotation: %q", expected)
		}
	}
}

func TestMetricsHelpAnnotations(t *testing.T) {
	m, cleanup := setupTestMetrics(t, false)
	defer cleanup()

	body := getMetricsBody(t, m)

	metrics := []string{
		"hitstor_snapshots_total",
		"hitstor_packs_total",
		"hitstor_indexes_total",
		"hitstor_repository_size_bytes",
		"hitstor_active_sessions",
		"hitstor_registered_agents",
		"hitstor_agents_ok",
		"hitstor_agents_stale",
		"hitstor_agents_never",
		"hitstor_backups_completed_total",
		"hitstor_backups_failed_total",
		"hitstor_bytes_received_total",
		"hitstor_chunks_received_total",
		"hitstor_agent_last_backup_timestamp_seconds",
		"hitstor_agent_schedule_enabled",
		"hitstor_agent_schedule_interval_hours",
	}
	for _, name := range metrics {
		expected := "# HELP " + name + " "
		if !strings.Contains(body, expected) {
			t.Errorf("missing help annotation for %q", name)
		}
	}
}

func TestMetricsCounterValues(t *testing.T) {
	m, cleanup := setupTestMetrics(t, false)
	defer cleanup()

	body := getMetricsBody(t, m)

	// Stub returns: completed=10, failed=2, bytes=1048576, chunks=500
	expectations := []string{
		"hitstor_backups_completed_total 10",
		"hitstor_backups_failed_total 2",
		"hitstor_bytes_received_total 1048576",
		"hitstor_chunks_received_total 500",
	}
	for _, exp := range expectations {
		if !strings.Contains(body, exp) {
			t.Errorf("expected %q in output", exp)
		}
	}
}

func TestMetricsGaugeValues(t *testing.T) {
	m, cleanup := setupTestMetrics(t, false)
	defer cleanup()

	body := getMetricsBody(t, m)

	// Stub activeSessionsFn returns 3.
	if !strings.Contains(body, "hitstor_active_sessions 3") {
		t.Error("expected hitstor_active_sessions 3")
	}

	// Empty repo: 0 snapshots, 0 agents.
	if !strings.Contains(body, "hitstor_snapshots_total 0") {
		t.Error("expected hitstor_snapshots_total 0")
	}
	if !strings.Contains(body, "hitstor_registered_agents 0") {
		t.Error("expected hitstor_registered_agents 0")
	}
}

func TestMetricsNoAgents(t *testing.T) {
	m, cleanup := setupTestMetrics(t, false)
	defer cleanup()

	body := getMetricsBody(t, m)

	if !strings.Contains(body, "hitstor_registered_agents 0") {
		t.Error("expected hitstor_registered_agents 0")
	}
	// No per-agent lines should be present.
	if strings.Contains(body, "hitstor_agent_last_backup_timestamp_seconds{") {
		t.Error("should not have per-agent metrics when no agents registered")
	}
	if strings.Contains(body, "hitstor_agent_schedule_enabled{") {
		t.Error("should not have per-agent schedule metrics when no agents registered")
	}
}

func TestMetricsWithAgents(t *testing.T) {
	m, cleanup := setupTestMetrics(t, true)
	defer cleanup()

	body := getMetricsBody(t, m)

	if !strings.Contains(body, "hitstor_registered_agents 2") {
		t.Error("expected hitstor_registered_agents 2")
	}
	if !strings.Contains(body, `hitstor_agent_last_backup_timestamp_seconds{agent="agent-alpha"} 0`) {
		t.Error("expected per-agent metric for agent-alpha")
	}
	if !strings.Contains(body, `hitstor_agent_last_backup_timestamp_seconds{agent="agent-beta"} 0`) {
		t.Error("expected per-agent metric for agent-beta")
	}
	// Both agents never backed up — all should be "never".
	if !strings.Contains(body, "hitstor_agents_never 2") {
		t.Error("expected hitstor_agents_never 2")
	}
	// Schedule metrics for agents (default: disabled, 0 hours).
	if !strings.Contains(body, `hitstor_agent_schedule_enabled{agent="agent-alpha"} 0`) {
		t.Error("expected schedule_enabled 0 for agent-alpha")
	}
	if !strings.Contains(body, `hitstor_agent_schedule_interval_hours{agent="agent-alpha"} 0`) {
		t.Error("expected schedule_interval_hours 0 for agent-alpha")
	}
}

func TestMetricsWithScheduleEnabled(t *testing.T) {
	m, cleanup := setupTestMetrics(t, true)
	defer cleanup()

	// Enable schedule on agent-alpha.
	if err := m.registry.UpdateSchedule("agent-alpha", true, 12); err != nil {
		t.Fatalf("UpdateSchedule: %v", err)
	}

	body := getMetricsBody(t, m)

	// agent-alpha: schedule enabled, 12 hours.
	if !strings.Contains(body, `hitstor_agent_schedule_enabled{agent="agent-alpha"} 1`) {
		t.Error("expected schedule_enabled 1 for agent-alpha")
	}
	if !strings.Contains(body, `hitstor_agent_schedule_interval_hours{agent="agent-alpha"} 12`) {
		t.Error("expected schedule_interval_hours 12 for agent-alpha")
	}
	// agent-beta: schedule still disabled.
	if !strings.Contains(body, `hitstor_agent_schedule_enabled{agent="agent-beta"} 0`) {
		t.Error("expected schedule_enabled 0 for agent-beta")
	}
}
