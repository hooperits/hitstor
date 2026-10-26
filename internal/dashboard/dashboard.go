package dashboard

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/server"
)

// Options configures the dashboard.
type Options struct {
	Password string
	Version  string
	RepoID   string
}

// Dashboard serves the web UI for monitoring and managing HitStor.
type Dashboard struct {
	repos            *server.RepoManager
	repo             *repository.Repository // first repo (convenience)
	registry         *server.AgentRegistry
	be               backend.Backend // first repo's backend (convenience)
	cfg              *config.Config
	srv              *server.Server
	opts             Options
	templates        map[string]*template.Template
	mu               sync.RWMutex
	sessions         map[string]time.Time // token -> expiry
	loginLimits      *loginLimiter
	activeSessionsFn func() int
	metricsHandler   http.HandlerFunc
	opMu             sync.RWMutex
	operations       map[string]*operation

	// Prometheus counters for maintenance operations.
	checkTotal      atomic.Uint64
	checkErrorTotal atomic.Uint64
	pruneTotal      atomic.Uint64
	pruneFreedBytes atomic.Uint64

	// Update notification state.
	latestVersion atomic.Value // string
	updateURL     atomic.Value // string

	// Restore session manager.
	restores *RestoreManager

	// Disaster recovery orchestrator (multi-job full-system restore).
	disaster *DisasterManager

	// Server address for agent registration.
	serverGRPCAddr string

	// Size cache to avoid stat-ing backend files repeatedly.
	sizeCacheMu sync.Mutex
	sizeCache   map[string]int64 // key: repoName/fileType/id -> size

	// Snapshot summary cache (avoids repeated LoadSnapshot for growth/metrics).
	snapshotCacheMu sync.Mutex
	snapshotCache   map[string]map[string]*cachedSnapshotSummary // repoName -> snapID -> summary

	// Full repo metrics cache with TTL.
	metricsCacheMu sync.Mutex
	metricsCache   map[string]metricsCacheEntry

	// Bare-metal recovery images, one per host. Built lazily so a server that
	// never builds media touches no disk for it.
	mediaCacheOnce sync.Once
	mediaCacheVal  *server.MediaCache
	mediaCacheDir  string
}

// SetMediaCacheDir overrides where bare-metal recovery images are cached. Must
// be called before the first build or download.
func (d *Dashboard) SetMediaCacheDir(dir string) {
	d.mediaCacheDir = dir
}

// mediaCache returns the recovery-media cache, creating it on first use.
//
// Images run to hundreds of megabytes, so they are deliberately kept out of
// the repository directory, which prune and integrity check walk.
func (d *Dashboard) mediaCache() *server.MediaCache {
	d.mediaCacheOnce.Do(func() {
		dir := d.mediaCacheDir
		if dir == "" {
			dir = defaultMediaCacheDir()
		}
		d.mediaCacheVal = server.NewMediaCache(dir)
	})
	return d.mediaCacheVal
}

// New creates a new Dashboard.
func New(repo *repository.Repository, registry *server.AgentRegistry, be backend.Backend, activeSessionsFn func() int, opts Options) (*Dashboard, error) {
	funcMap := template.FuncMap{
		"formatBytes":  formatBytes,
		"formatNumber": formatNumber,
		"timeAgo":      timeAgo,
		"shortID":      shortID,
		"urlquery":     url.QueryEscape,
		"statusClass":  statusClass,
		"timeFormat":   timeFormat,
		"safeHTML":     func(s string) template.HTML { return template.HTML(s) },
		"percent":      func(val float64) string { return fmt.Sprintf("%.1f%%", val*100) },
		"hasStr": func(list []string, want string) bool {
			for _, s := range list {
				if s == want {
					return true
				}
			}
			return false
		},
		"dict": func(values ...any) (map[string]any, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("dict: odd argument count")
			}
			out := make(map[string]any, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict: key must be string")
				}
				out[key] = values[i+1]
			}
			return out, nil
		},
	}

	subFS, err := fs.Sub(templateFS, "templates")
	if err != nil {
		return nil, fmt.Errorf("dashboard: template sub: %w", err)
	}

	d := &Dashboard{
		repo:             repo,
		registry:         registry,
		be:               be,
		opts:             opts,
		templates:        make(map[string]*template.Template),
		sessions:         make(map[string]time.Time),
		loginLimits:      newLoginLimiter(),
		activeSessionsFn: activeSessionsFn,
		operations:       make(map[string]*operation),
		sizeCache:        make(map[string]int64),
	}

	// Parse page templates (each page + layout).
	pages := []string{"overview", "backups", "backup_detail", "restores", "disaster", "agents", "agent_detail", "snapshot_detail", "storage", "storage_detail", "logs", "settings"}
	for _, page := range pages {
		files := []string{"layout.html", page + ".html"}
		t, err := template.New("").Funcs(funcMap).ParseFS(subFS, files...)
		if err != nil {
			return nil, fmt.Errorf("dashboard: parse template %s: %w", page, err)
		}
		d.templates[page] = t
	}

	// Login is standalone (no layout).
	loginTmpl, err := template.New("").Funcs(funcMap).ParseFS(subFS, "login.html")
	if err != nil {
		return nil, fmt.Errorf("dashboard: parse login template: %w", err)
	}
	d.templates["login"] = loginTmpl

	return d, nil
}

// MaintenanceCounters returns cumulative counters for dashboard maintenance operations.
func (d *Dashboard) MaintenanceCounters() (checks, checkErrors, prunes, pruneFreedBytes uint64) {
	return d.checkTotal.Load(), d.checkErrorTotal.Load(), d.pruneTotal.Load(), d.pruneFreedBytes.Load()
}

// SetRepoManager sets the multi-repo manager for storage management features.
func (d *Dashboard) SetRepoManager(rm *server.RepoManager) {
	d.repos = rm
}

// SetConfig sets the config reference for CRUD operations that persist changes.
func (d *Dashboard) SetConfig(cfg *config.Config) {
	d.cfg = cfg
}

// SetServer sets the gRPC server reference for session management.
func (d *Dashboard) SetServer(srv *server.Server) {
	d.srv = srv
}

// SetMetricsHandler registers the Prometheus metrics handler.
func (d *Dashboard) SetMetricsHandler(h http.HandlerFunc) {
	d.metricsHandler = h
}

// SetServerGRPCAddr stores the server's gRPC address for agent registration.
func (d *Dashboard) SetServerGRPCAddr(addr string) {
	d.serverGRPCAddr = addr
}

// SetRestoreManager sets the restore session manager for tracking restore operations.
func (d *Dashboard) SetRestoreManager(rm *RestoreManager) {
	d.restores = rm
	if d.disaster == nil {
		d.disaster = NewDisasterManager()
	}
}

// SetUpdateInfo stores the latest available version for display in the dashboard.
func (d *Dashboard) SetUpdateInfo(version, url string) {
	d.latestVersion.Store(version)
	d.updateURL.Store(url)
}

// Handler returns the HTTP handler for the dashboard.
func (d *Dashboard) Handler() http.Handler {
	mux := http.NewServeMux()

	// Static assets.
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Prometheus metrics (unauthenticated).
	if d.metricsHandler != nil {
		mux.HandleFunc("GET /metrics", d.metricsHandler)
	}

	// Login (always registered; middleware decides if auth is needed).
	mux.HandleFunc("GET /login", d.handleLoginPage)
	mux.HandleFunc("POST /login", d.handleLoginSubmit)
	mux.HandleFunc("POST /logout", d.handleLogout)

	// Pages.
	mux.Handle("GET /{$}", d.protect(http.HandlerFunc(d.handleOverview)))
	mux.Handle("GET /backups", d.protect(http.HandlerFunc(d.handleBackups)))
	mux.Handle("GET /backups/{id}", d.protect(http.HandlerFunc(d.handleBackupDetail)))
	mux.Handle("GET /restores", d.protect(http.HandlerFunc(d.handleRestores)))
	mux.Handle("GET /disaster", d.protect(http.HandlerFunc(d.handleDisasterPage)))
	mux.Handle("GET /agents", d.protect(http.HandlerFunc(d.handleAgents)))
	mux.Handle("GET /agents/{name}", d.protect(http.HandlerFunc(d.handleAgentDetail)))
	mux.Handle("GET /snapshots", d.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/storage", http.StatusFound)
	})))
	mux.Handle("GET /snapshots/{id}", d.protect(http.HandlerFunc(d.handleSnapshotDetail)))
	mux.Handle("GET /storage", d.protect(http.HandlerFunc(d.handleStorage)))
	mux.Handle("GET /storage/{name}", d.protect(http.HandlerFunc(d.handleStorageDetail)))

	// API (JSON).
	mux.Handle("GET /api/agents", d.protect(http.HandlerFunc(d.handleAPIAgents)))
	mux.Handle("GET /api/agents/{name}/snapshots", d.protect(http.HandlerFunc(d.handleAPIAgentSnapshots)))
	mux.Handle("POST /api/agents/{name}/schedule", d.protect(http.HandlerFunc(d.handleAPIAgentSchedule)))
	mux.Handle("POST /api/agents/{name}/repo", d.protect(http.HandlerFunc(d.handleAPIAgentRepo)))
	mux.Handle("GET /api/snapshots", d.protect(http.HandlerFunc(d.handleAPISnapshots)))
	mux.Handle("GET /api/stats", d.protect(http.HandlerFunc(d.handleAPIStats)))
	mux.Handle("GET /api/repos", d.protect(http.HandlerFunc(d.handleAPIRepos)))
	mux.Handle("GET /api/repos/growth", d.protect(http.HandlerFunc(d.handleAPIReposGrowthBatch)))
	mux.Handle("POST /api/repos", d.protect(http.HandlerFunc(d.handleAPICreateRepo)))
	mux.Handle("GET /api/repos/{name}", d.protect(http.HandlerFunc(d.handleAPIRepo)))
	mux.Handle("GET /api/repos/{name}/health", d.protect(http.HandlerFunc(d.handleAPIRepoHealth)))
	mux.Handle("DELETE /api/repos/{name}", d.protect(http.HandlerFunc(d.handleAPIDeleteRepo)))
	mux.Handle("GET /api/disk-space", d.protect(http.HandlerFunc(d.handleAPIDiskSpace)))
	mux.Handle("POST /api/repos/test-s3", d.protect(http.HandlerFunc(d.handleAPITestS3)))
	mux.Handle("POST /api/repos/detect", d.protect(http.HandlerFunc(d.handleAPIDetectRepo)))
	mux.Handle("POST /api/repos/validate-path", d.protect(http.HandlerFunc(d.handleAPIValidatePath)))
	mux.Handle("POST /api/repos/detect-s3", d.protect(http.HandlerFunc(d.handleAPIDetectS3)))
	mux.Handle("GET /api/repos/{name}/snapshots", d.protect(http.HandlerFunc(d.handleAPIRepoSnapshots)))
	mux.Handle("GET /api/repos/{name}/growth", d.protect(http.HandlerFunc(d.handleAPIGrowth)))
	mux.Handle("POST /api/repos/{name}/check", d.protect(http.HandlerFunc(d.handleAPICheck)))
	mux.Handle("POST /api/repos/{name}/prune", d.protect(http.HandlerFunc(d.handleAPIPrune)))
	mux.Handle("POST /api/repos/{name}/forget", d.protect(http.HandlerFunc(d.handleAPIForget)))
	mux.Handle("GET /api/repos/{name}/sessions", d.protect(http.HandlerFunc(d.handleAPISessions)))
	mux.Handle("GET /api/operations/{id}/events", d.protect(http.HandlerFunc(d.handleAPIOperationEvents)))
	mux.Handle("GET /api/active-backups", d.protect(http.HandlerFunc(d.handleAPIActiveBackups)))
	mux.Handle("GET /api/backup-progress/events", d.protect(http.HandlerFunc(d.handleAPIBackupProgressEvents)))

	// Backup jobs API.
	mux.Handle("GET /api/backups", d.protect(http.HandlerFunc(d.handleAPIBackupJobs)))
	mux.Handle("POST /api/backups", d.protect(http.HandlerFunc(d.handleAPICreateBackupJob)))
	mux.Handle("GET /api/backups/{id}", d.protect(http.HandlerFunc(d.handleAPIGetBackupJob)))
	mux.Handle("PUT /api/backups/{id}", d.protect(http.HandlerFunc(d.handleAPIUpdateBackupJob)))
	mux.Handle("DELETE /api/backups/{id}", d.protect(http.HandlerFunc(d.handleAPIDeleteBackupJob)))
	mux.Handle("POST /api/backups/{id}/run", d.protect(http.HandlerFunc(d.handleAPIRunBackupJob)))
	mux.Handle("POST /api/backups/{id}/cancel", d.protect(http.HandlerFunc(d.handleAPICancelBackupJob)))
	mux.Handle("GET /api/backups/validate-cron", d.protect(http.HandlerFunc(d.handleAPIValidateCron)))
	mux.Handle("GET /api/timezones", d.protect(http.HandlerFunc(d.handleAPITimezones)))

	// Restore API.
	mux.Handle("POST /api/snapshots/{id}/restore", d.protect(http.HandlerFunc(d.handleAPIStartRestore)))
	mux.Handle("GET /api/snapshots/{id}/tree", d.protect(http.HandlerFunc(d.handleAPISnapshotTree)))
	mux.Handle("GET /api/restore/{id}/events", d.protect(http.HandlerFunc(d.handleAPIRestoreEvents)))
	mux.Handle("GET /api/restores", d.protect(http.HandlerFunc(d.handleAPIRestores)))
	mux.Handle("GET /api/restores/sources", d.protect(http.HandlerFunc(d.handleAPIRestoreSources)))
	mux.Handle("GET /api/disaster/plans", d.protect(http.HandlerFunc(d.handleAPIDisasterPlans)))
	mux.Handle("GET /api/disaster/plans/{agent}", d.protect(http.HandlerFunc(d.handleAPIDisasterPlan)))
	mux.Handle("POST /api/disaster/recover", d.protect(http.HandlerFunc(d.handleAPIDisasterRecover)))
	mux.Handle("GET /api/disaster/recoveries", d.protect(http.HandlerFunc(d.handleAPIDisasterRecoveries)))
	mux.Handle("GET /api/disaster/recoveries/{id}", d.protect(http.HandlerFunc(d.handleAPIDisasterRecovery)))

	// Bare-metal recovery: media readiness, build, download.
	mux.Handle("GET /api/bmr/hosts", d.protect(http.HandlerFunc(d.handleAPIBMRHosts)))
	mux.Handle("POST /api/bmr/media/{agent}/build", d.protect(http.HandlerFunc(d.handleAPIBMRBuild)))
	mux.Handle("GET /api/bmr/media/{agent}/download", d.protect(http.HandlerFunc(d.handleAPIBMRDownload)))
	mux.Handle("DELETE /api/bmr/media/{agent}", d.protect(http.HandlerFunc(d.handleAPIBMRDeleteMedia)))
	mux.Handle("POST /api/bmr/prepare/{agent}", d.protect(http.HandlerFunc(d.handleAPIBMRPrepare)))
	mux.Handle("GET /api/bmr/tokens", d.protect(http.HandlerFunc(d.handleAPIBMRTokens)))
	mux.Handle("DELETE /api/bmr/tokens/{id}", d.protect(http.HandlerFunc(d.handleAPIBMRRevokeToken)))

	mux.Handle("GET /logs", d.protect(http.HandlerFunc(d.handleLogsPage)))
	mux.Handle("GET /settings", d.protect(http.HandlerFunc(d.handleSettingsPage)))

	// Settings API.
	mux.Handle("GET /api/settings", d.protect(http.HandlerFunc(d.handleAPISettings)))
	mux.Handle("PUT /api/settings/general", d.protect(http.HandlerFunc(d.handleAPISettingsGeneral)))
	mux.Handle("PUT /api/settings/security", d.protect(http.HandlerFunc(d.handleAPISettingsSecurity)))
	mux.Handle("PUT /api/settings/network", d.protect(http.HandlerFunc(d.handleAPISettingsNetwork)))
	mux.Handle("PUT /api/settings/notifications", d.protect(http.HandlerFunc(d.handleAPISettingsNotifications)))
	mux.Handle("PUT /api/settings/integrations", d.protect(http.HandlerFunc(d.handleAPISettingsIntegrations)))
	mux.Handle("POST /api/settings/integrations/import-env", d.protect(http.HandlerFunc(d.handleAPISettingsImportEnv)))
	mux.Handle("GET /api/settings/tls/status", d.protect(http.HandlerFunc(d.handleAPITLSStatus)))
	mux.Handle("POST /api/settings/tls/upload", d.protect(http.HandlerFunc(d.handleAPITLSUpload)))
	mux.Handle("POST /api/settings/test/gemini", d.protect(http.HandlerFunc(d.handleAPISettingsTestGemini)))
	mux.Handle("POST /api/settings/test/webhook", d.protect(http.HandlerFunc(d.handleAPISettingsTestWebhook)))

	// Agent discovery API.
	mux.Handle("GET /api/discover/subnets", d.protect(http.HandlerFunc(d.handleAPIDiscoverSubnets)))
	mux.Handle("POST /api/discover/scan", d.protect(http.HandlerFunc(d.handleAPIDiscoverScan)))
	mux.Handle("POST /api/discover/probe", d.protect(http.HandlerFunc(d.handleAPIDiscoverProbe)))

	// Agent management API.
	mux.Handle("POST /api/agents", d.protect(http.HandlerFunc(d.handleAPIAddAgent)))
	mux.Handle("POST /api/agents/{name}/browse", d.protect(http.HandlerFunc(d.handleAPIAgentBrowse)))
	mux.Handle("POST /api/agents/{name}/configure", d.protect(http.HandlerFunc(d.handleAPIAgentConfigure)))
	mux.Handle("GET /api/agents/{name}/info", d.protect(http.HandlerFunc(d.handleAPIAgentInfo)))
	mux.Handle("DELETE /api/agents/{name}", d.protect(http.HandlerFunc(d.handleAPIRemoveAgent)))
	mux.Handle("POST /api/agents/{name}/suspend", d.protect(http.HandlerFunc(d.handleAPIAgentSuspend)))
	mux.Handle("POST /api/agents/{name}/unsuspend", d.protect(http.HandlerFunc(d.handleAPIAgentUnsuspend)))

	mux.Handle("GET /api/logs", d.protect(http.HandlerFunc(d.handleAPILogs)))
	mux.Handle("GET /api/logs/export", d.protect(http.HandlerFunc(d.handleAPILogsExport)))
	mux.Handle("GET /api/alerts", d.protect(http.HandlerFunc(d.handleAPIGetAlerts)))
	mux.Handle("POST /api/alerts/{id}/resolve", d.protect(http.HandlerFunc(d.handleAPIResolveAlert)))

	return crossOriginProtect(mux)
}
func (d *Dashboard) render(w http.ResponseWriter, page string, data any) {
	t, ok := d.templates[page]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Login uses its own template name; pages use "layout".
	tmplName := "layout"
	if page == "login" {
		tmplName = "login.html"
	}
	if err := t.ExecuteTemplate(w, tmplName, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Template helper functions.

func formatBytes(b uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	switch {
	case b >= TB:
		return fmt.Sprintf("%.1f TiB", float64(b)/float64(TB))
	case b >= GB:
		return fmt.Sprintf("%.1f GiB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MiB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KiB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func formatNumber(n uint64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
}

func timeAgo(v any) string {
	var t time.Time
	switch x := v.(type) {
	case time.Time:
		t = x
	case *time.Time:
		if x == nil {
			return "never"
		}
		t = *x
	default:
		return "—"
	}
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

func timeFormat(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04:05")
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func statusClass(status string) string {
	switch status {
	case "ok":
		return "badge-ok"
	case "stale":
		return "badge-stale"
	default:
		return "badge-never"
	}
}
