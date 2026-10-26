package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/backend/local"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/server"
)

// setupDashboardWithData creates a dashboard with an agent, a snapshot, and recorded backup.
func setupDashboardWithData(t *testing.T) (*Dashboard, func()) {
	t.Helper()
	dash, cleanup := setupTestDashboard(t, "")

	// Register an agent.
	if _, err := dash.registry.AddAgent("prod-server"); err != nil {
		cleanup()
		t.Fatalf("add agent: %v", err)
	}
	dash.registry.UpdateLastBackup("prod-server", time.Now())

	// Save a snapshot so pages have data to render.
	snap := &repository.Snapshot{
		Time:     time.Now(),
		Hostname: "prod-server",
		Paths:    []string{"/data"},
		Summary: repository.SnapshotSummary{
			FilesNew:      5,
			TotalFiles:    10,
			TotalSize:     4096,
			TotalDuration: 1.5,
		},
	}
	if _, err := dash.repo.SaveSnapshot(snap); err != nil {
		cleanup()
		t.Fatalf("save snapshot: %v", err)
	}

	// Set up RepoManager so snapshots page and API find snapshots.
	rm := server.NewRepoManager()
	rm.AddDirect("default", dash.repo, dash.be)
	dash.SetRepoManager(rm)

	return dash, cleanup
}

func setupTestDashboard(t *testing.T, password string) (*Dashboard, func()) {
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

	dash, err := New(repo, registry, be, func() int { return 0 }, Options{
		Password: password,
		Version:  "test-1.0",
		RepoID:   repo.Cfg().ID,
	})
	if err != nil {
		be.Close()
		t.Fatalf("create dashboard: %v", err)
	}

	cleanup := func() { be.Close() }
	return dash, cleanup
}

func TestOverviewPage(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET / status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), "HitStor") {
		t.Error("overview page should contain 'HitStor'")
	}
}

func TestAgentsPage(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/agents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /agents status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestSnapshotsPageRedirect(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/snapshots", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("GET /snapshots status = %d, want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/storage" {
		t.Errorf("GET /snapshots redirect = %q, want /storage", loc)
	}
}

func TestStoragePage(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/storage", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /storage status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAPIStats(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/stats", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/stats status = %d, want %d", rec.Code, http.StatusOK)
	}

	var stats statsJSON
	if err := json.NewDecoder(rec.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if stats.Snapshots != 0 {
		t.Errorf("stats.Snapshots = %d, want 0", stats.Snapshots)
	}
	if stats.Agents != 0 {
		t.Errorf("stats.Agents = %d, want 0", stats.Agents)
	}
}

func TestAPIAgents(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/agents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/agents status = %d, want %d", rec.Code, http.StatusOK)
	}

	var agents []agentJSON
	if err := json.NewDecoder(rec.Body).Decode(&agents); err != nil {
		t.Fatalf("decode agents: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("agents count = %d, want 0", len(agents))
	}
}

func TestAPISnapshots(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/snapshots", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/snapshots status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAuthRequired(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "secret123")
	defer cleanup()

	handler := dash.Handler()

	// All protected routes should redirect to /login.
	for _, path := range []string{"/", "/agents", "/storage", "/api/stats"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Errorf("GET %s without auth status = %d, want %d", path, rec.Code, http.StatusSeeOther)
		}
		loc := rec.Header().Get("Location")
		if loc != "/login" {
			t.Errorf("GET %s redirect = %q, want /login", path, loc)
		}
	}
}

func TestAuthLoginSuccess(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "secret123")
	defer cleanup()

	handler := dash.Handler()

	// POST /login with correct password.
	form := url.Values{"password": {"secret123"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("POST /login status = %d, want %d", rec.Code, http.StatusSeeOther)
	}

	// Verify session cookie was set.
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("session cookie not set after login")
	}

	// GET / with session cookie should succeed.
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET / with session status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAuthLoginWrongPassword(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "secret123")
	defer cleanup()

	handler := dash.Handler()

	form := url.Values{"password": {"wrong"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Should render login page again (200 with error).
	if rec.Code != http.StatusOK {
		t.Errorf("POST /login wrong password status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "Invalid password") {
		t.Error("login page should contain error message")
	}
}

func TestNoAuthWhenPasswordEmpty(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	handler := dash.Handler()

	// All routes should be accessible without auth.
	for _, path := range []string{"/", "/agents", "/storage", "/api/stats"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s without password status = %d, want %d", path, rec.Code, http.StatusOK)
		}
	}
}

func TestStaticAssets(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	handler := dash.Handler()

	for _, path := range []string{"/static/style.css", "/static/htmx.min.js", "/static/chart.min.js", "/static/app.js"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, want %d", path, rec.Code, http.StatusOK)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("GET %s body is empty", path)
		}
	}
}

func TestMetricsNoAuth(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "secret123")
	defer cleanup()

	// Set a simple metrics handler.
	dash.SetMetricsHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("# metrics output"))
	})

	handler := dash.Handler()

	// /metrics should be accessible without auth.
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /metrics without auth status = %d, want %d", rec.Code, http.StatusOK)
	}

	// / should still require auth.
	req = httptest.NewRequest("GET", "/", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("GET / without auth status = %d, want %d (redirect)", rec.Code, http.StatusSeeOther)
	}
}

func TestAPIAgentSchedule(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	// Register an agent.
	if _, err := dash.registry.AddAgent("test-agent"); err != nil {
		t.Fatal(err)
	}

	handler := dash.Handler()

	// Set schedule.
	body := `{"enabled":true,"interval_hours":12}`
	req := httptest.NewRequest("POST", "/api/agents/test-agent/schedule", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST schedule status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Verify it was persisted via API agents list.
	req = httptest.NewRequest("GET", "/api/agents", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var agents []agentJSON
	if err := json.NewDecoder(rec.Body).Decode(&agents); err != nil {
		t.Fatalf("decode agents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("agents count = %d, want 1", len(agents))
	}
	if !agents[0].ScheduleEnabled {
		t.Error("expected schedule_enabled=true")
	}
	if agents[0].ScheduleHours != 12 {
		t.Errorf("schedule_hours = %d, want 12", agents[0].ScheduleHours)
	}
}

func TestAPIAgentScheduleNotFound(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	handler := dash.Handler()

	body := `{"enabled":true,"interval_hours":6}`
	req := httptest.NewRequest("POST", "/api/agents/nonexistent/schedule", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("POST schedule for nonexistent agent status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAPIAgentScheduleInvalidInterval(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	if _, err := dash.registry.AddAgent("test-agent"); err != nil {
		t.Fatal(err)
	}

	handler := dash.Handler()

	body := `{"enabled":true,"interval_hours":0}`
	req := httptest.NewRequest("POST", "/api/agents/test-agent/schedule", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST schedule with 0 interval status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestLoginPageRedirectsWhenNoPassword(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("GET /login without password status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	loc := rec.Header().Get("Location")
	if loc != "/" {
		t.Errorf("redirect = %q, want /", loc)
	}
}

// --- Template helper function tests ---

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input uint64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
		{1024 * 1024 * 1024 * 1024, "1.0 TiB"},
		{2 * 1024 * 1024 * 1024 * 1024, "2.0 TiB"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.input)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input uint64
		want  string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.0K"},
		{1500, "1.5K"},
		{999999, "1000.0K"},
		{1_000_000, "1.0M"},
		{2_500_000, "2.5M"},
	}
	for _, tt := range tests {
		got := formatNumber(tt.input)
		if got != tt.want {
			t.Errorf("formatNumber(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTimeAgo(t *testing.T) {
	now := time.Now()

	// time.Time value
	if got := timeAgo(now); got != "just now" {
		t.Errorf("timeAgo(now) = %q, want %q", got, "just now")
	}

	// Minutes ago
	if got := timeAgo(now.Add(-5 * time.Minute)); got != "5m ago" {
		t.Errorf("timeAgo(-5m) = %q, want %q", got, "5m ago")
	}

	// Hours ago
	if got := timeAgo(now.Add(-3 * time.Hour)); got != "3h ago" {
		t.Errorf("timeAgo(-3h) = %q, want %q", got, "3h ago")
	}

	// 1 day ago
	if got := timeAgo(now.Add(-25 * time.Hour)); got != "1 day ago" {
		t.Errorf("timeAgo(-25h) = %q, want %q", got, "1 day ago")
	}

	// Multiple days ago
	if got := timeAgo(now.Add(-72 * time.Hour)); got != "3 days ago" {
		t.Errorf("timeAgo(-72h) = %q, want %q", got, "3 days ago")
	}

	// *time.Time pointer
	ptr := now
	if got := timeAgo(&ptr); got != "just now" {
		t.Errorf("timeAgo(&now) = %q, want %q", got, "just now")
	}

	// nil *time.Time
	var nilPtr *time.Time
	if got := timeAgo(nilPtr); got != "never" {
		t.Errorf("timeAgo(nil) = %q, want %q", got, "never")
	}

	// Zero time
	if got := timeAgo(time.Time{}); got != "never" {
		t.Errorf("timeAgo(zero) = %q, want %q", got, "never")
	}

	// Unknown type
	if got := timeAgo("string"); got != "—" {
		t.Errorf("timeAgo(string) = %q, want %q", got, "—")
	}
}

func TestTimeFormat(t *testing.T) {
	// Zero time
	if got := timeFormat(time.Time{}); got != "—" {
		t.Errorf("timeFormat(zero) = %q, want %q", got, "—")
	}

	// Non-zero time
	ts := time.Date(2025, 6, 15, 14, 30, 45, 0, time.UTC)
	want := "2025-06-15 14:30:45"
	if got := timeFormat(ts); got != want {
		t.Errorf("timeFormat = %q, want %q", got, want)
	}
}

func TestShortID(t *testing.T) {
	if got := shortID("abcdef1234567890"); got != "abcdef12" {
		t.Errorf("shortID(long) = %q, want %q", got, "abcdef12")
	}
	if got := shortID("short"); got != "short" {
		t.Errorf("shortID(short) = %q, want %q", got, "short")
	}
	if got := shortID("12345678"); got != "12345678" {
		t.Errorf("shortID(exact) = %q, want %q", got, "12345678")
	}
}

func TestStatusClass(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"ok", "badge-ok"},
		{"stale", "badge-stale"},
		{"never", "badge-never"},
		{"unknown", "badge-never"},
	}
	for _, tt := range tests {
		if got := statusClass(tt.input); got != tt.want {
			t.Errorf("statusClass(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- Handler tests ---

func TestLogout(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "secret123")
	defer cleanup()

	handler := dash.Handler()

	// Login first to get a session.
	form := url.Values{"password": {"secret123"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session cookie after login")
	}

	// POST /logout.
	req = httptest.NewRequest("POST", "/logout", nil)
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("POST /logout status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	loc := rec.Header().Get("Location")
	if loc != "/login" {
		t.Errorf("POST /logout redirect = %q, want /login", loc)
	}

	// Verify cookie was cleared (MaxAge -1).
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.MaxAge != -1 {
			t.Errorf("session cookie MaxAge = %d, want -1", c.MaxAge)
		}
	}

	// GET / with old session should now redirect to /login.
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessionCookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("GET / after logout status = %d, want %d (redirect)", rec.Code, http.StatusSeeOther)
	}
}

func TestLogoutWithoutCookie(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "secret123")
	defer cleanup()

	handler := dash.Handler()

	// POST /logout without a cookie should still work.
	req := httptest.NewRequest("POST", "/logout", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("POST /logout without cookie status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
}

func TestAgentDetailPage(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	// Register an agent.
	if _, err := dash.registry.AddAgent("web-server"); err != nil {
		t.Fatal(err)
	}

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/agents/web-server", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /agents/web-server status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestAgentDetailPageNotFound(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/agents/nonexistent", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /agents/nonexistent status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestLoginSubmitWhenNoPassword(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	handler := dash.Handler()

	form := url.Values{"password": {"anything"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("POST /login without password config status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("redirect = %q, want /", loc)
	}
}

func TestAPISnapshotsWithHostFilter(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	handler := dash.Handler()

	// Request with host filter — empty result is fine, just covers the code path.
	req := httptest.NewRequest("GET", "/api/snapshots?host=web-server&limit=5", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/snapshots?host=web-server status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAPIAgentScheduleInvalidJSON(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	if _, err := dash.registry.AddAgent("test-agent"); err != nil {
		t.Fatal(err)
	}

	handler := dash.Handler()

	req := httptest.NewRequest("POST", "/api/agents/test-agent/schedule", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST schedule with invalid JSON status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAPIAgentsWithBackup(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	// Register an agent and record a last backup time.
	if _, err := dash.registry.AddAgent("active-agent"); err != nil {
		t.Fatal(err)
	}
	dash.registry.UpdateLastBackup("active-agent", time.Now())

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/agents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/agents status = %d, want %d", rec.Code, http.StatusOK)
	}

	var agents []agentJSON
	if err := json.NewDecoder(rec.Body).Decode(&agents); err != nil {
		t.Fatalf("decode agents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("agents count = %d, want 1", len(agents))
	}
	if agents[0].Status != "ok" {
		t.Errorf("status = %q, want %q", agents[0].Status, "ok")
	}
	if agents[0].LastBackup == nil {
		t.Error("expected last_backup to be set")
	}
}

func TestOverviewPageWithData(t *testing.T) {
	dash, cleanup := setupDashboardWithData(t)
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET / status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAgentsPageWithData(t *testing.T) {
	dash, cleanup := setupDashboardWithData(t)
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/agents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /agents status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestStorageDetailPageWithData(t *testing.T) {
	dash, cleanup := setupDashboardWithData(t)
	defer cleanup()

	handler := dash.Handler()

	// Without filter.
	req := httptest.NewRequest("GET", "/storage/default", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /storage/default status = %d, want %d", rec.Code, http.StatusOK)
	}

	// With host filter.
	req = httptest.NewRequest("GET", "/storage/default?host=prod-server", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /storage/default?host=prod-server status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Non-existent repo returns 404.
	req = httptest.NewRequest("GET", "/storage/nonexistent", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /storage/nonexistent status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestStoragePageWithData(t *testing.T) {
	dash, cleanup := setupDashboardWithData(t)
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/storage", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /storage status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAPIStatsWithData(t *testing.T) {
	dash, cleanup := setupDashboardWithData(t)
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/stats", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var stats statsJSON
	if err := json.NewDecoder(rec.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if stats.Snapshots != 1 {
		t.Errorf("Snapshots = %d, want 1", stats.Snapshots)
	}
	if stats.Agents != 1 {
		t.Errorf("Agents = %d, want 1", stats.Agents)
	}
	if stats.TotalBytes == 0 {
		t.Error("TotalBytes should be > 0")
	}
}

func TestAPISnapshotsWithData(t *testing.T) {
	dash, cleanup := setupDashboardWithData(t)
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/snapshots", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var snaps []snapshotJSON
	if err := json.NewDecoder(rec.Body).Decode(&snaps); err != nil {
		t.Fatalf("decode snapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("snapshot count = %d, want 1", len(snaps))
	}
	if snaps[0].Hostname != "prod-server" {
		t.Errorf("hostname = %q, want %q", snaps[0].Hostname, "prod-server")
	}
	if snaps[0].TotalFiles != 10 {
		t.Errorf("total_files = %d, want 10", snaps[0].TotalFiles)
	}
}

func TestAPIAgentsWithStaleAgent(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	if _, err := dash.registry.AddAgent("stale-agent"); err != nil {
		t.Fatal(err)
	}
	// Set last backup to 26 hours ago (beyond 25h stale threshold).
	dash.registry.UpdateLastBackup("stale-agent", time.Now().Add(-26*time.Hour))

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/agents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var agents []agentJSON
	if err := json.NewDecoder(rec.Body).Decode(&agents); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("count = %d, want 1", len(agents))
	}
	if agents[0].Status != "stale" {
		t.Errorf("status = %q, want %q", agents[0].Status, "stale")
	}
}

func TestOverviewWithMixedAgentStatuses(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	// ok agent
	dash.registry.AddAgent("ok-agent")
	dash.registry.UpdateLastBackup("ok-agent", time.Now())

	// stale agent
	dash.registry.AddAgent("stale-agent")
	dash.registry.UpdateLastBackup("stale-agent", time.Now().Add(-26*time.Hour))

	// never agent (no backup)
	dash.registry.AddAgent("never-agent")

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET / status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAgentDetailWithSnapshots(t *testing.T) {
	dash, cleanup := setupDashboardWithData(t)
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/agents/prod-server", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /agents/prod-server status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "prod-server") {
		t.Error("agent detail page should contain agent name")
	}
}

func TestActiveSessionsFn(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	// Override the active sessions function.
	dash.activeSessionsFn = func() int { return 42 }

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/stats", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var stats statsJSON
	if err := json.NewDecoder(rec.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if stats.ActiveSessions != 42 {
		t.Errorf("ActiveSessions = %d, want 42", stats.ActiveSessions)
	}
}

func TestAPISentinelEndpoints(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "")
	defer cleanup()

	// Register an agent first.
	_, err := dash.registry.AddAgent("test-agent")
	if err != nil {
		t.Fatal(err)
	}

	handler := dash.Handler()

	// 1. Suspend the agent.
	req := httptest.NewRequest("POST", "/api/agents/test-agent/suspend", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /api/agents/test-agent/suspend status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify agent is suspended in registry.
	agent := dash.registry.GetAgent("test-agent")
	if agent == nil || !agent.Suspended {
		t.Error("expected test-agent to be suspended in registry")
	}

	// 2. Unsuspend the agent.
	req = httptest.NewRequest("POST", "/api/agents/test-agent/unsuspend", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /api/agents/test-agent/unsuspend status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify agent is no longer suspended.
	agent = dash.registry.GetAgent("test-agent")
	if agent == nil || agent.Suspended {
		t.Error("expected test-agent to be unsuspended in registry")
	}

	// 3. Add an alert.
	alert := server.SentinelAlert{
		ID:             "alert-123",
		AgentName:      "test-agent",
		Hostname:       "test-host",
		Time:           time.Now(),
		ThreatDetected: true,
		Confidence:     0.99,
		Reason:         "Ransomware telemetry",
		Status:         "active",
	}
	if err := dash.registry.AddAlert(alert); err != nil {
		t.Fatal(err)
	}

	// Verify alerts list via API.
	req = httptest.NewRequest("GET", "/api/alerts", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/alerts status = %d, want %d", rec.Code, http.StatusOK)
	}

	var alerts []server.SentinelAlert
	if err := json.NewDecoder(rec.Body).Decode(&alerts); err != nil {
		t.Fatalf("decode alerts: %v", err)
	}
	if len(alerts) != 1 || alerts[0].ID != "alert-123" {
		t.Errorf("unexpected alerts: %+v", alerts)
	}

	// 4. Resolve the alert.
	req = httptest.NewRequest("POST", "/api/alerts/alert-123/resolve", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /api/alerts/alert-123/resolve status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify alert is resolved in registry.
	alerts = dash.registry.ListAlerts()
	if len(alerts) != 1 || alerts[0].Status != "resolved" {
		t.Errorf("expected resolved alert, got: %+v", alerts)
	}
}

func TestRestoresPage(t *testing.T) {
	dash, cleanup := setupDashboardWithData(t)
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/restores", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /restores status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "restores-app") {
		t.Error("restores page should contain restores-app container")
	}
	if !strings.Contains(body, "restores.js") {
		t.Error("restores page should load restores.js")
	}
}

func TestAPIAgentSnapshots(t *testing.T) {
	dash, cleanup := setupDashboardWithData(t)
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/agents/prod-server/snapshots", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/agents/prod-server/snapshots status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct {
		Agent     string         `json:"agent"`
		Repos     []string       `json:"repos"`
		Snapshots []snapshotJSON `json:"snapshots"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Agent != "prod-server" {
		t.Errorf("agent = %q, want prod-server", resp.Agent)
	}
	if len(resp.Snapshots) == 0 {
		t.Fatal("expected at least one snapshot for prod-server")
	}
	if resp.Snapshots[0].RepoName == "" {
		t.Error("snapshot should include repo_name")
	}
	if len(resp.Repos) == 0 {
		t.Error("expected at least one repo in repos list")
	}
}

func TestAPIRestoreSources(t *testing.T) {
	dash, cleanup := setupDashboardWithData(t)
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/restores/sources", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/restores/sources status = %d, want %d", rec.Code, http.StatusOK)
	}

	var sources []restoreSourceJSON
	if err := json.NewDecoder(rec.Body).Decode(&sources); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("expected at least one restore source")
	}
	found := false
	for _, s := range sources {
		if s.ID == "prod-server" && s.BackupType == "files" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected prod-server files source, got %+v", sources)
	}
}

func TestAPIAgentSnapshotsNotFound(t *testing.T) {
	dash, cleanup := setupDashboardWithData(t)
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/agents/unknown-agent/snapshots", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct {
		Snapshots []snapshotJSON `json:"snapshots"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Snapshots) != 0 {
		t.Errorf("expected empty snapshots, got %d", len(resp.Snapshots))
	}
}
