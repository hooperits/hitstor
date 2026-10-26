package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/backend/local"
	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/server"
)

// setupTestDashboardWithRepoManager creates a dashboard with a RepoManager for multi-repo tests.
func setupTestDashboardWithRepoManager(t *testing.T) (*Dashboard, *server.RepoManager, func()) {
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
		Password: "",
		Version:  "test-1.0",
		RepoID:   repo.Cfg().ID,
	})
	if err != nil {
		be.Close()
		t.Fatalf("create dashboard: %v", err)
	}

	rm := server.NewRepoManager()
	rm.AddDirect("default", repo, be)
	dash.SetRepoManager(rm)

	// Set up a config file for CRUD tests.
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := config.Config{
		Path:   cfgPath,
		Exists: true,
	}
	dash.SetConfig(&cfg)

	cleanup := func() { be.Close() }
	return dash, rm, cleanup
}

func TestComputeStorageMetrics(t *testing.T) {
	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatalf("create backend: %v", err)
	}
	defer be.Close()

	repo, err := repository.Init(be, "test-password")
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}

	// Save a snapshot with dedup/compression data.
	snap := &repository.Snapshot{
		Time:     time.Now(),
		Hostname: "test-host",
		Paths:    []string{"/data"},
		Summary: repository.SnapshotSummary{
			FilesNew:       10,
			TotalFiles:     10,
			TotalSize:      4096,
			BytesProcessed: 10000,
			BytesAdded:     5000,
			TotalDuration:  1.5,
		},
	}
	if _, err := repo.SaveSnapshot(snap); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	mr := &server.ManagedRepo{
		Name:            "test-repo",
		Repo:            repo,
		Backend:         be,
		MaintenanceLock: server.NewMaintenanceLock(),
	}

	dash := &Dashboard{sizeCache: make(map[string]int64)}
	metrics := dash.computeStorageMetrics(mr, nil)

	if metrics.Snapshots != 1 {
		t.Errorf("Snapshots = %d, want 1", metrics.Snapshots)
	}
	if metrics.TotalBytes == 0 {
		t.Error("TotalBytes should be > 0")
	}

	// Dedup ratio: BytesProcessed/BytesAdded = 10000/5000 = 2.0
	if metrics.DedupRatio < 1.9 || metrics.DedupRatio > 2.1 {
		t.Errorf("DedupRatio = %.2f, want ~2.0", metrics.DedupRatio)
	}

	// Compression ratio: BytesAdded/TotalBytes. Should be > 0.
	if metrics.CompressionRatio <= 0 {
		t.Errorf("CompressionRatio = %.2f, want > 0", metrics.CompressionRatio)
	}

	// Local backend should have disk stats.
	if metrics.DiskTotal == 0 {
		t.Error("DiskTotal should be > 0 for local backend")
	}
	if metrics.DiskFree == 0 {
		t.Error("DiskFree should be > 0 for local backend")
	}
	if metrics.DiskUsedPercent <= 0 || metrics.DiskUsedPercent > 100 {
		t.Errorf("DiskUsedPercent = %.2f, want (0, 100]", metrics.DiskUsedPercent)
	}

	// Alert level should be ok (test filesystem won't be >80% full).
	if metrics.AlertLevel != "ok" {
		t.Errorf("AlertLevel = %q, want %q", metrics.AlertLevel, "ok")
	}

	if metrics.Name != "test-repo" {
		t.Errorf("Name = %q, want %q", metrics.Name, "test-repo")
	}

	// Maintenance lock should be off.
	if metrics.IsLocked {
		t.Error("IsLocked should be false")
	}
}

func TestComputeStorageMetricsNoSnapshots(t *testing.T) {
	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatalf("create backend: %v", err)
	}
	defer be.Close()

	repo, err := repository.Init(be, "test-password")
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}

	mr := &server.ManagedRepo{
		Name:            "empty-repo",
		Repo:            repo,
		Backend:         be,
		MaintenanceLock: server.NewMaintenanceLock(),
	}

	dash := &Dashboard{sizeCache: make(map[string]int64)}
	metrics := dash.computeStorageMetrics(mr, nil)

	if metrics.Snapshots != 0 {
		t.Errorf("Snapshots = %d, want 0", metrics.Snapshots)
	}
	if metrics.DedupRatio != 0 {
		t.Errorf("DedupRatio = %.2f, want 0", metrics.DedupRatio)
	}
	if metrics.CompressionRatio != 0 {
		t.Errorf("CompressionRatio = %.2f, want 0", metrics.CompressionRatio)
	}
}

func TestComputeStorageMetricsLocked(t *testing.T) {
	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatalf("create backend: %v", err)
	}
	defer be.Close()

	repo, err := repository.Init(be, "test-password")
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}

	ml := server.NewMaintenanceLock()
	if _, err := ml.Acquire("check"); err != nil {
		t.Fatalf("acquire lock: %v", err)
	}

	mr := &server.ManagedRepo{
		Name:            "locked-repo",
		Repo:            repo,
		Backend:         be,
		MaintenanceLock: ml,
	}

	dash := &Dashboard{sizeCache: make(map[string]int64)}
	metrics := dash.computeStorageMetrics(mr, nil)

	if !metrics.IsLocked {
		t.Error("IsLocked should be true")
	}
	if metrics.LockReason != "check" {
		t.Errorf("LockReason = %q, want %q", metrics.LockReason, "check")
	}
}

func TestComputeStorageMetricsJobCount(t *testing.T) {
	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatalf("create backend: %v", err)
	}
	defer be.Close()

	repo, err := repository.Init(be, "test-password")
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}

	mr := &server.ManagedRepo{
		Name:            "main",
		Repo:            repo,
		Backend:         be,
		MaintenanceLock: server.NewMaintenanceLock(),
	}

	jobCounts := map[string]int{"main": 3}
	dash := &Dashboard{sizeCache: make(map[string]int64)}
	metrics := dash.computeStorageMetrics(mr, jobCounts)

	if metrics.BackupJobCount != 3 {
		t.Errorf("BackupJobCount = %d, want 3", metrics.BackupJobCount)
	}
}

func TestAPIRepos(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/repos", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/repos status = %d, want %d", rec.Code, http.StatusOK)
	}

	var repos []repoMetricsJSON
	if err := json.NewDecoder(rec.Body).Decode(&repos); err != nil {
		t.Fatalf("decode repos: %v", err)
	}

	if len(repos) != 1 {
		t.Fatalf("repos count = %d, want 1", len(repos))
	}

	r := repos[0]
	if r.Name != "default" {
		t.Errorf("Name = %q, want %q", r.Name, "default")
	}
	if r.DiskTotal == 0 {
		t.Error("DiskTotal should be > 0 for local backend")
	}
	if r.AlertLevel != "ok" {
		t.Errorf("AlertLevel = %q, want %q", r.AlertLevel, "ok")
	}
}

func TestAPIReposSummary(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	snap := &repository.Snapshot{
		Time:     time.Now(),
		Hostname: "test-host",
		Paths:    []string{"/data"},
		Summary: repository.SnapshotSummary{
			TotalSize:      8192,
			BytesProcessed: 20000,
			BytesAdded:     8000,
		},
	}
	if _, err := dash.repo.SaveSnapshot(snap); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/repos?summary=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var repos []repoMetricsJSON
	if err := json.NewDecoder(rec.Body).Decode(&repos); err != nil {
		t.Fatalf("decode repos: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("repos count = %d, want 1", len(repos))
	}
	r := repos[0]
	if r.Snapshots != 1 {
		t.Errorf("Snapshots = %d, want 1", r.Snapshots)
	}
	if r.MetricsLoaded {
		t.Error("MetricsLoaded should be false in summary mode")
	}
	if r.TotalBytes != 0 {
		t.Errorf("TotalBytes = %d, want 0 in summary mode", r.TotalBytes)
	}
}

func TestAPIReposWithSnapshot(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	// Save a snapshot with dedup data.
	snap := &repository.Snapshot{
		Time:     time.Now(),
		Hostname: "test-host",
		Paths:    []string{"/data"},
		Summary: repository.SnapshotSummary{
			FilesNew:       5,
			TotalFiles:     10,
			TotalSize:      8192,
			BytesProcessed: 20000,
			BytesAdded:     8000,
			TotalDuration:  2.0,
		},
	}
	if _, err := dash.repo.SaveSnapshot(snap); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/repos", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var repos []repoMetricsJSON
	if err := json.NewDecoder(rec.Body).Decode(&repos); err != nil {
		t.Fatalf("decode repos: %v", err)
	}

	if len(repos) != 1 {
		t.Fatalf("repos count = %d, want 1", len(repos))
	}

	r := repos[0]
	if r.Snapshots != 1 {
		t.Errorf("Snapshots = %d, want 1", r.Snapshots)
	}
	if r.TotalBytes == 0 {
		t.Error("TotalBytes should be > 0")
	}
	// DedupRatio: 20000/8000 = 2.5
	if r.DedupRatio < 2.4 || r.DedupRatio > 2.6 {
		t.Errorf("DedupRatio = %.2f, want ~2.5", r.DedupRatio)
	}
}

func TestAPIReposWithAgentJobCount(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	// Register agents and create backup jobs targeting the default repo.
	if _, err := dash.registry.AddAgent("agent-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := dash.registry.AddAgent("agent-2"); err != nil {
		t.Fatal(err)
	}
	if err := dash.registry.AddJob(server.BackupJob{
		ID: "job1", Name: "job-1", AgentName: "agent-1", RepoName: "default",
		Paths: []string{"/data"}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := dash.registry.AddJob(server.BackupJob{
		ID: "job2", Name: "job-2", AgentName: "agent-2", RepoName: "default",
		Paths: []string{"/home"}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/repos", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var repos []repoMetricsJSON
	if err := json.NewDecoder(rec.Body).Decode(&repos); err != nil {
		t.Fatalf("decode repos: %v", err)
	}

	if repos[0].BackupJobCount != 2 {
		t.Errorf("BackupJobCount = %d, want 2", repos[0].BackupJobCount)
	}
}

// --- Phase 4: US2 CRUD tests ---

func TestAPICreateLocalRepo(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	newRepoPath := filepath.Join(t.TempDir(), "new-repo")
	if err := os.MkdirAll(newRepoPath, 0700); err != nil {
		t.Fatal(err)
	}

	handler := dash.Handler()
	body := `{"name":"test-local","type":"local","path":"` + newRepoPath + `","password":"secret123"}`
	req := httptest.NewRequest("POST", "/api/repos", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/repos status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["name"] != "test-local" {
		t.Errorf("name = %v, want %q", resp["name"], "test-local")
	}
	if resp["action"] != "created" {
		t.Errorf("action = %v, want %q", resp["action"], "created")
	}

	// Verify repo appears in list.
	req = httptest.NewRequest("GET", "/api/repos", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var repos []repoMetricsJSON
	if err := json.NewDecoder(rec.Body).Decode(&repos); err != nil {
		t.Fatalf("decode repos: %v", err)
	}

	found := false
	for _, r := range repos {
		if r.Name == "test-local" {
			found = true
			break
		}
	}
	if !found {
		t.Error("new repo not found in list")
	}

	// Verify config was persisted.
	if dash.cfg == nil {
		t.Fatal("cfg is nil")
	}
	var cfgFound bool
	for _, rc := range dash.cfg.Repositories {
		if rc.Name == "test-local" {
			cfgFound = true
			if rc.Path != newRepoPath {
				t.Errorf("config path = %q, want %q", rc.Path, newRepoPath)
			}
		}
	}
	if !cfgFound {
		t.Error("repo not found in config.Repositories")
	}
}

func TestAPICreateRepoDuplicateName(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()
	body := `{"name":"default","type":"local","path":"/tmp/test","password":"pw"}`
	req := httptest.NewRequest("POST", "/api/repos", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("POST /api/repos duplicate status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestAPIDeleteRepo(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	// Create a second repo to delete.
	newRepoPath := filepath.Join(t.TempDir(), "delete-repo")
	if err := os.MkdirAll(newRepoPath, 0700); err != nil {
		t.Fatal(err)
	}

	handler := dash.Handler()
	body := `{"name":"to-delete","type":"local","path":"` + newRepoPath + `","password":"pw"}`
	req := httptest.NewRequest("POST", "/api/repos", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, body: %s", rec.Code, rec.Body.String())
	}

	// Delete it.
	req = httptest.NewRequest("DELETE", "/api/repos/to-delete", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /api/repos/to-delete status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Verify it's gone from list.
	req = httptest.NewRequest("GET", "/api/repos", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var repos []repoMetricsJSON
	json.NewDecoder(rec.Body).Decode(&repos)
	for _, r := range repos {
		if r.Name == "to-delete" {
			t.Error("deleted repo still appears in list")
		}
	}
}

func TestAPIDeleteRepoWithJobs(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	// Register an agent and create a backup job targeting default repo.
	if _, err := dash.registry.AddAgent("my-agent"); err != nil {
		t.Fatal(err)
	}
	if err := dash.registry.AddJob(server.BackupJob{
		ID: "job1", Name: "my-backup", AgentName: "my-agent", RepoName: "default",
		Paths: []string{"/data"}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	handler := dash.Handler()
	req := httptest.NewRequest("DELETE", "/api/repos/default", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("DELETE /api/repos/default with jobs status = %d, want %d", rec.Code, http.StatusConflict)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] == nil {
		t.Error("expected error message in response")
	}
}

func TestAPIDiskSpace(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	dir := t.TempDir()
	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/disk-space?path="+dir, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/disk-space status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["valid"] != true {
		t.Error("expected valid=true")
	}
	if resp["total_bytes"] == nil || resp["total_bytes"].(float64) == 0 {
		t.Error("expected non-zero total_bytes")
	}
	if resp["free_bytes"] == nil || resp["free_bytes"].(float64) == 0 {
		t.Error("expected non-zero free_bytes")
	}
}

func TestAPIDiskSpaceInvalidPath(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/disk-space?path=/nonexistent/path/xyz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("GET /api/disk-space invalid path status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAPIDetectRepo(t *testing.T) {
	// Create a real repo at a temp path.
	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := repository.Init(be, "pw")
	if err != nil {
		t.Fatal(err)
	}
	// Save a snapshot so we get a count > 0.
	snap := &repository.Snapshot{
		Time:     time.Now(),
		Hostname: "test",
		Paths:    []string{"/data"},
	}
	repo.SaveSnapshot(snap)
	be.Close()

	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()

	// Detect existing repo.
	body := `{"path":"` + dir + `"}`
	req := httptest.NewRequest("POST", "/api/repos/detect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/repos/detect status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["exists"] != true {
		t.Error("expected exists=true")
	}
	if resp["snapshots"] == nil || resp["snapshots"].(float64) != 1 {
		t.Errorf("snapshots = %v, want 1", resp["snapshots"])
	}
}

func TestAPIDetectRepoNoRepo(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()

	// Detect at empty dir (no repo).
	emptyDir := t.TempDir()
	body := `{"path":"` + emptyDir + `"}`
	req := httptest.NewRequest("POST", "/api/repos/detect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/repos/detect status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["exists"] != false {
		t.Error("expected exists=false")
	}
}

func TestAPIValidatePathCreate(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	dir := t.TempDir()
	handler := dash.Handler()
	body := `{"path":"` + filepath.Join(dir, "new-repo") + `","mode":"create"}`
	req := httptest.NewRequest("POST", "/api/repos/validate-path", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/repos/validate-path status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["valid"] != true {
		t.Errorf("valid = %v, want true", resp["valid"])
	}
	if resp["writable"] != true {
		t.Errorf("writable = %v, want true", resp["writable"])
	}
}

func TestAPIValidatePathConnectMissing(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	dir := t.TempDir()
	handler := dash.Handler()
	body := `{"path":"` + dir + `","mode":"connect"}`
	req := httptest.NewRequest("POST", "/api/repos/validate-path", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/repos/validate-path status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["valid"] != false {
		t.Error("expected valid=false for empty dir in connect mode")
	}
}

func TestAPIValidatePathCreateConflict(t *testing.T) {
	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Init(be, "pw"); err != nil {
		t.Fatal(err)
	}
	be.Close()

	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()
	body := `{"path":"` + dir + `","mode":"create"}`
	req := httptest.NewRequest("POST", "/api/repos/validate-path", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["valid"] != false {
		t.Error("expected valid=false when repo exists in create mode")
	}
	if resp["exists"] != true {
		t.Error("expected exists=true")
	}
}

func TestAPIConnectLocalRepo(t *testing.T) {
	dir := t.TempDir()
	be, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Init(be, "connect-pw"); err != nil {
		t.Fatal(err)
	}
	be.Close()

	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()
	body := `{"name":"connected","type":"local","mode":"connect","path":"` + dir + `","password":"connect-pw"}`
	req := httptest.NewRequest("POST", "/api/repos", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/repos connect status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["action"] != "connected" {
		t.Errorf("action = %v, want connected", resp["action"])
	}
}

func TestAPICreateS3RequiresVerified(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()
	body := `{"name":"s3-test","type":"s3","mode":"create","endpoint":"https://s3.amazonaws.com","bucket":"b","access_key":"k","secret_key":"s","password":"pw"}`
	req := httptest.NewRequest("POST", "/api/repos", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("POST /api/repos S3 without verified status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAPICheck(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()

	// Start a check.
	body := `{"deep":false}`
	req := httptest.NewRequest("POST", "/api/repos/default/check", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /api/repos/default/check status = %d, want %d; body: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	opID, ok := resp["operation_id"].(string)
	if !ok || opID == "" {
		t.Fatal("expected non-empty operation_id")
	}

	// Wait for the operation to complete.
	dash.opMu.RLock()
	op := dash.operations[opID]
	dash.opMu.RUnlock()
	if op == nil {
		t.Fatal("operation not found in tracker")
	}

	select {
	case <-op.done:
	case <-time.After(10 * time.Second):
		t.Fatal("check operation timed out")
	}

	op.mu.Lock()
	defer op.mu.Unlock()

	if op.Status != "completed" {
		t.Errorf("operation status = %q, want completed", op.Status)
	}
	result, ok := op.Result.(map[string]any)
	if !ok {
		t.Fatal("expected result map")
	}
	if result["pass"] != true {
		t.Error("expected pass=true for empty repo check")
	}
}

func TestAPICheckLocked(t *testing.T) {
	dash, rm, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()

	// Lock the repo.
	mr, _ := rm.Get("default")
	mr.MaintenanceLock.Acquire("test lock")

	body := `{"deep":false}`
	req := httptest.NewRequest("POST", "/api/repos/default/check", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("POST /api/repos/default/check status = %d, want %d", rec.Code, http.StatusConflict)
	}

	mr.MaintenanceLock.Release()
}

func TestAPISessions(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()

	// Without a server, should return empty array.
	req := httptest.NewRequest("GET", "/api/repos/default/sessions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/repos/default/sessions status = %d, want %d", rec.Code, http.StatusOK)
	}

	var sessions []any
	json.NewDecoder(rec.Body).Decode(&sessions)
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestAPIPrune(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()

	// Start a prune.
	body := `{}`
	req := httptest.NewRequest("POST", "/api/repos/default/prune", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /api/repos/default/prune status = %d, want %d; body: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	opID, ok := resp["operation_id"].(string)
	if !ok || opID == "" {
		t.Fatal("expected non-empty operation_id")
	}

	// Wait for the operation to complete.
	dash.opMu.RLock()
	op := dash.operations[opID]
	dash.opMu.RUnlock()
	if op == nil {
		t.Fatal("operation not found in tracker")
	}

	select {
	case <-op.done:
	case <-time.After(10 * time.Second):
		t.Fatal("prune operation timed out")
	}

	op.mu.Lock()
	defer op.mu.Unlock()

	if op.Status != "completed" {
		t.Errorf("operation status = %q, want completed", op.Status)
	}
	result, ok := op.Result.(map[string]any)
	if !ok {
		t.Fatal("expected result map")
	}
	// Empty repo prune should succeed with 0 deletions.
	if result["packs_deleted"] != 0 {
		t.Errorf("expected packs_deleted=0, got %v", result["packs_deleted"])
	}
}

func TestAPIPruneLocked(t *testing.T) {
	dash, rm, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()

	mr, _ := rm.Get("default")
	mr.MaintenanceLock.Acquire("test lock")

	req := httptest.NewRequest("POST", "/api/repos/default/prune", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("POST /api/repos/default/prune status = %d, want %d", rec.Code, http.StatusConflict)
	}

	mr.MaintenanceLock.Release()
}

func TestAPIForget(t *testing.T) {
	dash, rm, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()

	mr, _ := rm.Get("default")
	keepID, err := mr.Repo.SaveSnapshot(&repository.Snapshot{
		Time:     time.Now(),
		Hostname: "test-host",
		Paths:    []string{"/keep"},
	})
	if err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	forgetID, err := mr.Repo.SaveSnapshot(&repository.Snapshot{
		Time:     time.Now(),
		Hostname: "test-host",
		Paths:    []string{"/forget"},
	})
	if err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	body := `{"snapshot_ids":["` + forgetID.String() + `"]}`
	req := httptest.NewRequest("POST", "/api/repos/default/forget", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/repos/default/forget status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["forgotten"] != float64(1) {
		t.Errorf("forgotten = %v, want 1", resp["forgotten"])
	}

	remaining, err := mr.Repo.ListSnapshots()
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(remaining) != 1 || remaining[0] != keepID {
		t.Errorf("remaining snapshots = %v, want only %v", remaining, keepID)
	}
}

func TestAPIForgetWithPrune(t *testing.T) {
	dash, rm, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()

	mr, _ := rm.Get("default")
	snapID, err := mr.Repo.SaveSnapshot(&repository.Snapshot{
		Time:     time.Now(),
		Hostname: "test-host",
		Paths:    []string{"/data"},
	})
	if err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	body := `{"snapshot_ids":["` + snapID.String() + `"],"prune":true}`
	req := httptest.NewRequest("POST", "/api/repos/default/forget", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/repos/default/forget status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["forgotten"] != float64(1) {
		t.Errorf("forgotten = %v, want 1", resp["forgotten"])
	}
	pruneResult, ok := resp["prune"].(map[string]any)
	if !ok {
		t.Fatalf("expected prune result in response, got: %v", resp)
	}
	if _, ok := pruneResult["packs_deleted"]; !ok {
		t.Errorf("expected packs_deleted in prune result, got: %v", pruneResult)
	}
}

func TestAPIForgetUnknownSnapshot(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()

	body := `{"snapshot_ids":["` + strings.Repeat("ab", 32) + `"]}`
	req := httptest.NewRequest("POST", "/api/repos/default/forget", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["forgotten"] != float64(0) {
		t.Errorf("forgotten = %v, want 0", resp["forgotten"])
	}
	failed, ok := resp["failed"].([]any)
	if !ok || len(failed) != 1 {
		t.Errorf("expected 1 failed entry, got: %v", resp["failed"])
	}
}

func TestAPIForgetNoIDs(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()

	req := httptest.NewRequest("POST", "/api/repos/default/forget", strings.NewReader(`{"snapshot_ids":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAPIForgetInvalidID(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()

	req := httptest.NewRequest("POST", "/api/repos/default/forget", strings.NewReader(`{"snapshot_ids":["not-hex"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAPIForgetLocked(t *testing.T) {
	dash, rm, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()

	mr, _ := rm.Get("default")
	mr.MaintenanceLock.Acquire("test lock")

	body := `{"snapshot_ids":["` + strings.Repeat("ab", 32) + `"]}`
	req := httptest.NewRequest("POST", "/api/repos/default/forget", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}

	mr.MaintenanceLock.Release()
}

func TestAPIForgetUnknownRepo(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()

	req := httptest.NewRequest("POST", "/api/repos/does-not-exist/forget", strings.NewReader(`{"snapshot_ids":["`+strings.Repeat("ab", 32)+`"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAPIGrowth(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()

	// Empty repo should return empty array.
	req := httptest.NewRequest("GET", "/api/repos/default/growth?range=90d&granularity=daily", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/repos/default/growth status = %d, want %d", rec.Code, http.StatusOK)
	}

	var points []map[string]any
	json.NewDecoder(rec.Body).Decode(&points)
	if len(points) != 0 {
		t.Errorf("expected 0 growth points, got %d", len(points))
	}
}

func TestAPIGrowthWithSnapshot(t *testing.T) {
	dash, rm, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()

	// Add a snapshot with data.
	mr, _ := rm.Get("default")
	snap := repository.Snapshot{
		Time:     time.Now(),
		Hostname: "test-host",
		Paths:    []string{"/data"},
		Summary: repository.SnapshotSummary{
			TotalSize:      1024 * 1024,
			TotalFiles:     10,
			BytesProcessed: 2 * 1024 * 1024,
			BytesAdded:     1024 * 1024,
		},
	}
	if _, err := mr.Repo.SaveSnapshot(&snap); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/repos/default/growth?range=30d&granularity=daily", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var points []map[string]any
	json.NewDecoder(rec.Body).Decode(&points)
	if len(points) != 1 {
		t.Fatalf("expected 1 growth point, got %d", len(points))
	}
	if points[0]["date"] == "" {
		t.Error("expected non-empty date")
	}
	totalBytes, ok := points[0]["total_bytes"].(float64)
	if !ok || totalBytes == 0 {
		t.Error("expected non-zero total_bytes")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{500 * time.Millisecond, "0s"},
		{5 * time.Second, "5s"},
		{45 * time.Second, "45s"},
		{90 * time.Second, "1m 30s"},
		{5*time.Minute + 15*time.Second, "5m 15s"},
		{1*time.Hour + 30*time.Minute, "1h 30m"},
		{2*time.Hour + 15*time.Minute, "2h 15m"},
		{24 * time.Hour, "24h 0m"},
	}
	for _, tt := range tests {
		got := FormatDuration(tt.d)
		if got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestActiveBackupsNoServer(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/active-backups", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	count, ok := result["count"].(float64)
	if !ok || count != 0 {
		t.Errorf("count = %v, want 0", result["count"])
	}
}

func TestBackupProgressEventsSSE(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()
	req := httptest.NewRequest("GET", "/api/backup-progress/events", nil)

	// Cancel after short time to avoid blocking.
	ctx, cancel := context.WithTimeout(req.Context(), 3*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "data:") {
		t.Error("expected SSE data event in response")
	}
	if !strings.Contains(body, `"sessions"`) {
		t.Error("expected sessions field in SSE data")
	}
}

func TestSessionInfoToProgress(t *testing.T) {
	si := server.SessionInfo{
		ID:             "test-id",
		AgentName:      "test-agent",
		Hostname:       "test-host",
		JobID:          "job-1",
		RepoName:       "repo-1",
		BackupType:     "full",
		StartTime:      time.Now().Add(-5 * time.Minute),
		BytesAdded:     1048576,
		FilesProcessed: 100,
		FilesNew:       50,
		Phase:          "uploading",
		LastActivity:   time.Now(),
	}

	p := sessionInfoToProgress(si)
	if p.SessionID != "test-id" {
		t.Errorf("SessionID = %q, want %q", p.SessionID, "test-id")
	}
	if p.BytesUploaded != 1048576 {
		t.Errorf("BytesUploaded = %d, want 1048576", p.BytesUploaded)
	}
	if p.ElapsedSeconds < 299 || p.ElapsedSeconds > 302 {
		t.Errorf("ElapsedSeconds = %.1f, want ~300", p.ElapsedSeconds)
	}
	if p.Stale {
		t.Error("expected stale=false for recent activity")
	}
	if p.BytesHuman != "1.0 MiB" {
		t.Errorf("BytesHuman = %q, want %q", p.BytesHuman, "1.0 MiB")
	}
}

func TestAPICancelBackupJob(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	handler := dash.Handler()

	// Cancel non-existent job → 404.
	req := httptest.NewRequest("POST", "/api/backups/no-such-id/cancel", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cancel non-existent: status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	// Create a job with RunNow=true to simulate pending backup.
	err := dash.registry.AddJob(server.BackupJob{
		ID:        "cancel-test",
		Name:      "Cancel Test",
		AgentName: "test-agent",
		RepoName:  "default",
		Paths:     []string{"/data"},
	})
	if err != nil {
		t.Fatal(err)
	}
	dash.registry.SetJobRunNow("cancel-test", true)

	// Cancel the pending backup.
	req = httptest.NewRequest("POST", "/api/backups/cancel-test/cancel", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel pending: status = %d, want %d", rec.Code, http.StatusOK)
	}

	var result map[string]any
	json.NewDecoder(rec.Body).Decode(&result)
	if result["ok"] != true {
		t.Errorf("expected ok=true, got %v", result["ok"])
	}
	if msg, _ := result["message"].(string); msg != "Backup cancelled." {
		t.Errorf("message = %q, want %q", msg, "Backup cancelled.")
	}

	// RunNow should be cleared.
	job := dash.registry.GetJob("cancel-test")
	if job.RunNow {
		t.Error("RunNow should be false after cancel")
	}

	// Cancel again (idempotent) → should succeed with "no active" message.
	req = httptest.NewRequest("POST", "/api/backups/cancel-test/cancel", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel idempotent: status = %d, want %d", rec.Code, http.StatusOK)
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if msg, _ := result["message"].(string); msg != "No active backup to cancel." {
		t.Errorf("message = %q, want %q", msg, "No active backup to cancel.")
	}
}
