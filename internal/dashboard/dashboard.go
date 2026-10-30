package dashboard

import (
	"context"
	"encoding/json"
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
	"github.com/hooperits/hitstor/internal/i18n"
	"github.com/hooperits/hitstor/internal/mailer"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/server"
	"github.com/hooperits/hitstor/internal/users"
)

// Options configures the dashboard.
type Options struct {
	Password string
	Version  string
	RepoID   string
}

// Dashboard serves the web UI for monitoring and managing HitStor.
type Dashboard struct {
	repos       *server.RepoManager
	repo        *repository.Repository // first repo (convenience)
	registry    *server.AgentRegistry
	be          backend.Backend // first repo's backend (convenience)
	cfg         *config.Config
	srv         *server.Server
	opts        Options
	templates   map[string]map[string]*template.Template // language → page → template
	mu          sync.RWMutex
	loginLimits *loginLimiter
	legacy      legacySessions
	users       *users.Store    // nil until SetUsers
	audit       *users.AuditLog // nil until SetUsers
	resetLimits *loginLimiter   // password reset requests per IP
	mail        mailState

	// sendMailHook replaces sending email. Tests only.
	sendMailHook     func(context.Context, mailer.Message) error
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

	// Workload plugin restores, by session ID (*WorkloadRestoreSession).
	workloadRestores sync.Map

	// pluginPackages overrides where uploaded plugin packages are kept.
	// Tests only; empty uses defaultPluginPackageDir.
	pluginPackages string

	// dialAgent replaces pinned-TLS agent dialing. Tests only.
	dialAgent func(name, addr string) (*server.AgentClient, error)

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
	baseFuncs := template.FuncMap{
		"formatBytes":  formatBytes,
		"formatNumber": formatNumber,
		"shortID":      shortID,
		"urlquery":     url.QueryEscape,
		"statusClass":  statusClass,
		"timeFormat":   timeFormat,
		"safeHTML":     func(s string) template.HTML { return template.HTML(s) },
		"toJSON": func(v any) (string, error) {
			b, err := json.Marshal(v)
			return string(b), err
		},
		// safeURL marks a URL the server built (a data: QR code) as safe for src.
		"safeURL": func(s string) template.URL { return template.URL(s) },
		"percent": func(val float64) string { return fmt.Sprintf("%.1f%%", val*100) },
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
		templates:        make(map[string]map[string]*template.Template),
		loginLimits:      newLoginLimiter(),
		resetLimits:      newLoginLimiter(),
		activeSessionsFn: activeSessionsFn,
		operations:       make(map[string]*operation),
		sizeCache:        make(map[string]int64),
	}

	// Parse page templates (each page + layout) once per language, with the
	// translation helpers bound to that language's catalog.
	pages := []string{"overview", "backups", "backup_detail", "restores", "disaster", "agents", "agent_detail", "snapshot_detail", "storage", "storage_detail", "logs", "settings", "account", "users", "user", "audit"}
	for _, lang := range i18n.Languages() {
		funcMap := template.FuncMap{}
		for k, v := range baseFuncs {
			funcMap[k] = v
		}
		for k, v := range langFuncs(i18n.Get(lang.Code)) {
			funcMap[k] = v
		}
		set := make(map[string]*template.Template, len(pages)+1)
		for _, page := range pages {
			files := []string{"layout.html", "i18n.html", page + ".html"}
			t, err := template.New("").Funcs(funcMap).ParseFS(subFS, files...)
			if err != nil {
				return nil, fmt.Errorf("dashboard: parse template %s: %w", page, err)
			}
			set[page] = t
		}

		// Login is standalone (no layout).
		loginTmpl, err := template.New("").Funcs(funcMap).ParseFS(subFS, "login.html", "i18n.html")
		if err != nil {
			return nil, fmt.Errorf("dashboard: parse login template: %w", err)
		}
		set["login"] = loginTmpl
		d.templates[lang.Code] = set
	}

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
	mux.HandleFunc("GET /login/code", d.handleCodePage)
	mux.HandleFunc("POST /login/code", d.handleCodeSubmit)
	mux.HandleFunc("GET /login/enroll", d.handleEnrollPage)
	mux.HandleFunc("POST /login/enroll", d.handleEnrollSubmit)
	mux.HandleFunc("GET /login/forgot", d.handleForgotPage)
	mux.HandleFunc("POST /login/forgot", d.handleForgotSubmit)
	mux.HandleFunc("GET /login/reset", d.handleResetPage)
	mux.HandleFunc("POST /login/reset", d.handleResetSubmit)
	mux.HandleFunc("GET /setup", d.handleSetupPage)
	mux.HandleFunc("POST /setup", d.handleSetupSubmit)
	mux.HandleFunc("POST /logout", d.handleLogout)

	// Accounts: the user's own, and (administrators) everyone's.
	for pattern, h := range map[string]http.HandlerFunc{
		"GET /account":                           d.handleAccountPage,
		"GET /account/password":                  d.handleAccountPasswordPage,
		"POST /api/account/password":             d.handleAPIAccountPassword,
		"POST /api/account/recovery-codes":       d.handleAPIAccountRecoveryCodes,
		"GET /api/account/sessions":              d.handleAPIAccountSessions,
		"GET /api/session":                       d.handleAPISession,
		"POST /api/session/activity":             d.handleAPISessionActivity,
		"DELETE /api/account/sessions/{id}":      d.handleAPIAccountEndSession,
		"POST /api/reauth":                       d.handleAPIReauth,
		"GET /users":                             d.handleUsersPage,
		"GET /api/users":                         d.handleAPIUsers,
		"POST /api/users":                        d.handleAPIAddUser,
		"PUT /api/users/{name}":                  d.handleAPIUpdateUser,
		"DELETE /api/users/{name}":               d.handleAPIRemoveUser,
		"POST /api/users/{name}/password":        d.handleAPIResetUserPassword,
		"DELETE /api/users/{name}/second-factor": d.handleAPIResetUserSecondFactor,
		"GET /api/users/{name}/sessions":         d.handleAPIUserSessions,
		"DELETE /api/users/{name}/sessions/{id}": d.handleAPIEndUserSession,
		"GET /audit":                             d.handleAuditPage,
		"GET /api/audit":                         d.handleAPIAudit,
		"GET /api/audit/export":                  d.handleAPIAuditExport,
		"GET /users/{name}":                      d.handleUserPage,
		"GET /api/users/{name}":                  d.handleAPIUser,
		"PUT /api/account/profile":               d.handleAPIAccountProfile,
		"DELETE /api/account/sessions":           d.handleAPIAccountEndOtherSessions,
	} {
		mux.Handle(pattern, d.guard(pattern, h))
	}
	mux.HandleFunc("GET /language", d.handleSetLanguage)

	// Pages.
	mux.Handle("GET /{$}", d.guard("GET /{$}", http.HandlerFunc(d.handleOverview)))
	mux.Handle("GET /backups", d.guard("GET /backups", http.HandlerFunc(d.handleBackups)))
	mux.Handle("GET /backups/{id}", d.guard("GET /backups/{id}", http.HandlerFunc(d.handleBackupDetail)))
	mux.Handle("GET /restores", d.guard("GET /restores", http.HandlerFunc(d.handleRestores)))
	mux.Handle("GET /disaster", d.guard("GET /disaster", http.HandlerFunc(d.handleDisasterPage)))
	mux.Handle("GET /agents", d.guard("GET /agents", http.HandlerFunc(d.handleAgents)))
	mux.Handle("GET /agents/{name}", d.guard("GET /agents/{name}", http.HandlerFunc(d.handleAgentDetail)))
	mux.Handle("GET /snapshots", d.guard("GET /snapshots", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/storage", http.StatusFound)
	})))
	mux.Handle("GET /snapshots/{id}", d.guard("GET /snapshots/{id}", http.HandlerFunc(d.handleSnapshotDetail)))
	mux.Handle("GET /storage", d.guard("GET /storage", http.HandlerFunc(d.handleStorage)))
	mux.Handle("GET /storage/{name}", d.guard("GET /storage/{name}", http.HandlerFunc(d.handleStorageDetail)))

	// API (JSON).
	mux.Handle("GET /api/agents", d.guard("GET /api/agents", http.HandlerFunc(d.handleAPIAgents)))
	mux.Handle("GET /api/agents/{name}/snapshots", d.guard("GET /api/agents/{name}/snapshots", http.HandlerFunc(d.handleAPIAgentSnapshots)))
	mux.Handle("POST /api/agents/{name}/schedule", d.guard("POST /api/agents/{name}/schedule", http.HandlerFunc(d.handleAPIAgentSchedule)))
	mux.Handle("POST /api/agents/{name}/repo", d.guard("POST /api/agents/{name}/repo", http.HandlerFunc(d.handleAPIAgentRepo)))
	mux.Handle("GET /api/snapshots", d.guard("GET /api/snapshots", http.HandlerFunc(d.handleAPISnapshots)))
	mux.Handle("GET /api/stats", d.guard("GET /api/stats", http.HandlerFunc(d.handleAPIStats)))
	mux.Handle("GET /api/repos", d.guard("GET /api/repos", http.HandlerFunc(d.handleAPIRepos)))
	mux.Handle("GET /api/repos/growth", d.guard("GET /api/repos/growth", http.HandlerFunc(d.handleAPIReposGrowthBatch)))
	mux.Handle("POST /api/repos", d.guard("POST /api/repos", http.HandlerFunc(d.handleAPICreateRepo)))
	mux.Handle("GET /api/repos/{name}", d.guard("GET /api/repos/{name}", http.HandlerFunc(d.handleAPIRepo)))
	mux.Handle("GET /api/repos/{name}/health", d.guard("GET /api/repos/{name}/health", http.HandlerFunc(d.handleAPIRepoHealth)))
	mux.Handle("DELETE /api/repos/{name}", d.guard("DELETE /api/repos/{name}", http.HandlerFunc(d.handleAPIDeleteRepo)))
	mux.Handle("GET /api/disk-space", d.guard("GET /api/disk-space", http.HandlerFunc(d.handleAPIDiskSpace)))
	mux.Handle("POST /api/repos/test-s3", d.guard("POST /api/repos/test-s3", http.HandlerFunc(d.handleAPITestS3)))
	mux.Handle("POST /api/repos/detect", d.guard("POST /api/repos/detect", http.HandlerFunc(d.handleAPIDetectRepo)))
	mux.Handle("POST /api/repos/validate-path", d.guard("POST /api/repos/validate-path", http.HandlerFunc(d.handleAPIValidatePath)))
	mux.Handle("POST /api/repos/detect-s3", d.guard("POST /api/repos/detect-s3", http.HandlerFunc(d.handleAPIDetectS3)))
	mux.Handle("GET /api/repos/{name}/snapshots", d.guard("GET /api/repos/{name}/snapshots", http.HandlerFunc(d.handleAPIRepoSnapshots)))
	mux.Handle("GET /api/repos/{name}/growth", d.guard("GET /api/repos/{name}/growth", http.HandlerFunc(d.handleAPIGrowth)))
	mux.Handle("POST /api/repos/{name}/check", d.guard("POST /api/repos/{name}/check", http.HandlerFunc(d.handleAPICheck)))
	mux.Handle("POST /api/repos/{name}/prune", d.guard("POST /api/repos/{name}/prune", http.HandlerFunc(d.handleAPIPrune)))
	mux.Handle("POST /api/repos/{name}/forget", d.guard("POST /api/repos/{name}/forget", http.HandlerFunc(d.handleAPIForget)))
	mux.Handle("GET /api/repos/{name}/sessions", d.guard("GET /api/repos/{name}/sessions", http.HandlerFunc(d.handleAPISessions)))
	mux.Handle("GET /api/operations/{id}/events", d.guard("GET /api/operations/{id}/events", http.HandlerFunc(d.handleAPIOperationEvents)))
	mux.Handle("GET /api/active-backups", d.guard("GET /api/active-backups", http.HandlerFunc(d.handleAPIActiveBackups)))
	mux.Handle("GET /api/backup-progress/events", d.guard("GET /api/backup-progress/events", http.HandlerFunc(d.handleAPIBackupProgressEvents)))

	// Backup jobs API.
	mux.Handle("GET /api/backups", d.guard("GET /api/backups", http.HandlerFunc(d.handleAPIBackupJobs)))
	mux.Handle("POST /api/backups", d.guard("POST /api/backups", http.HandlerFunc(d.handleAPICreateBackupJob)))
	mux.Handle("GET /api/backups/{id}", d.guard("GET /api/backups/{id}", http.HandlerFunc(d.handleAPIGetBackupJob)))
	mux.Handle("PUT /api/backups/{id}", d.guard("PUT /api/backups/{id}", http.HandlerFunc(d.handleAPIUpdateBackupJob)))
	mux.Handle("DELETE /api/backups/{id}", d.guard("DELETE /api/backups/{id}", http.HandlerFunc(d.handleAPIDeleteBackupJob)))
	mux.Handle("POST /api/backups/{id}/run", d.guard("POST /api/backups/{id}/run", http.HandlerFunc(d.handleAPIRunBackupJob)))
	mux.Handle("POST /api/backups/{id}/cancel", d.guard("POST /api/backups/{id}/cancel", http.HandlerFunc(d.handleAPICancelBackupJob)))
	mux.Handle("GET /api/backups/{id}/chains", d.guard("GET /api/backups/{id}/chains", http.HandlerFunc(d.handleAPIBackupJobChains)))
	mux.Handle("GET /api/backups/validate-cron", d.guard("GET /api/backups/validate-cron", http.HandlerFunc(d.handleAPIValidateCron)))
	mux.Handle("GET /api/timezones", d.guard("GET /api/timezones", http.HandlerFunc(d.handleAPITimezones)))

	// Restore API.
	mux.Handle("POST /api/snapshots/{id}/restore", d.guard("POST /api/snapshots/{id}/restore", http.HandlerFunc(d.handleAPIStartRestore)))
	mux.Handle("GET /api/snapshots/{id}/tree", d.guard("GET /api/snapshots/{id}/tree", http.HandlerFunc(d.handleAPISnapshotTree)))
	mux.Handle("GET /api/restore/{id}/events", d.guard("GET /api/restore/{id}/events", http.HandlerFunc(d.handleAPIRestoreEvents)))
	mux.Handle("GET /api/restores", d.guard("GET /api/restores", http.HandlerFunc(d.handleAPIRestores)))
	mux.Handle("GET /api/restores/sources", d.guard("GET /api/restores/sources", http.HandlerFunc(d.handleAPIRestoreSources)))
	mux.Handle("GET /api/disaster/plans", d.guard("GET /api/disaster/plans", http.HandlerFunc(d.handleAPIDisasterPlans)))
	mux.Handle("GET /api/disaster/plans/{agent}", d.guard("GET /api/disaster/plans/{agent}", http.HandlerFunc(d.handleAPIDisasterPlan)))
	mux.Handle("POST /api/disaster/recover", d.guard("POST /api/disaster/recover", http.HandlerFunc(d.handleAPIDisasterRecover)))
	mux.Handle("GET /api/disaster/recoveries", d.guard("GET /api/disaster/recoveries", http.HandlerFunc(d.handleAPIDisasterRecoveries)))
	mux.Handle("GET /api/disaster/recoveries/{id}", d.guard("GET /api/disaster/recoveries/{id}", http.HandlerFunc(d.handleAPIDisasterRecovery)))

	// Bare-metal recovery: media readiness, build, download.
	mux.Handle("GET /api/bmr/hosts", d.guard("GET /api/bmr/hosts", http.HandlerFunc(d.handleAPIBMRHosts)))
	mux.Handle("POST /api/bmr/media/{agent}/build", d.guard("POST /api/bmr/media/{agent}/build", http.HandlerFunc(d.handleAPIBMRBuild)))
	mux.Handle("GET /api/bmr/media/{agent}/download", d.guard("GET /api/bmr/media/{agent}/download", http.HandlerFunc(d.handleAPIBMRDownload)))
	mux.Handle("DELETE /api/bmr/media/{agent}", d.guard("DELETE /api/bmr/media/{agent}", http.HandlerFunc(d.handleAPIBMRDeleteMedia)))
	mux.Handle("POST /api/bmr/prepare/{agent}", d.guard("POST /api/bmr/prepare/{agent}", http.HandlerFunc(d.handleAPIBMRPrepare)))
	mux.Handle("GET /api/bmr/tokens", d.guard("GET /api/bmr/tokens", http.HandlerFunc(d.handleAPIBMRTokens)))
	mux.Handle("DELETE /api/bmr/tokens/{id}", d.guard("DELETE /api/bmr/tokens/{id}", http.HandlerFunc(d.handleAPIBMRRevokeToken)))

	mux.Handle("GET /logs", d.guard("GET /logs", http.HandlerFunc(d.handleLogsPage)))
	mux.Handle("GET /settings", d.guard("GET /settings", http.HandlerFunc(d.handleSettingsPage)))

	// Settings API.
	mux.Handle("GET /api/settings", d.guard("GET /api/settings", http.HandlerFunc(d.handleAPISettings)))
	mux.Handle("PUT /api/settings/general", d.guard("PUT /api/settings/general", http.HandlerFunc(d.handleAPISettingsGeneral)))
	mux.Handle("PUT /api/settings/security", d.guard("PUT /api/settings/security", http.HandlerFunc(d.handleAPISettingsSecurity)))
	mux.Handle("PUT /api/settings/network", d.guard("PUT /api/settings/network", http.HandlerFunc(d.handleAPISettingsNetwork)))
	mux.Handle("PUT /api/settings/notifications", d.guard("PUT /api/settings/notifications", http.HandlerFunc(d.handleAPISettingsNotifications)))
	mux.Handle("PUT /api/settings/integrations", d.guard("PUT /api/settings/integrations", http.HandlerFunc(d.handleAPISettingsIntegrations)))
	mux.Handle("PUT /api/settings/email", d.guard("PUT /api/settings/email", http.HandlerFunc(d.handleAPISettingsEmail)))
	mux.Handle("POST /api/settings/email/test", d.guard("POST /api/settings/email/test", http.HandlerFunc(d.handleAPISettingsEmailTest)))
	mux.Handle("POST /api/settings/integrations/import-env", d.guard("POST /api/settings/integrations/import-env", http.HandlerFunc(d.handleAPISettingsImportEnv)))
	mux.Handle("GET /api/settings/tls/status", d.guard("GET /api/settings/tls/status", http.HandlerFunc(d.handleAPITLSStatus)))
	mux.Handle("POST /api/settings/tls/upload", d.guard("POST /api/settings/tls/upload", http.HandlerFunc(d.handleAPITLSUpload)))
	mux.Handle("POST /api/settings/test/gemini", d.guard("POST /api/settings/test/gemini", http.HandlerFunc(d.handleAPISettingsTestGemini)))
	mux.Handle("POST /api/settings/test/webhook", d.guard("POST /api/settings/test/webhook", http.HandlerFunc(d.handleAPISettingsTestWebhook)))

	// Agent discovery API.
	mux.Handle("GET /api/discover/subnets", d.guard("GET /api/discover/subnets", http.HandlerFunc(d.handleAPIDiscoverSubnets)))
	mux.Handle("POST /api/discover/scan", d.guard("POST /api/discover/scan", http.HandlerFunc(d.handleAPIDiscoverScan)))
	mux.Handle("POST /api/discover/probe", d.guard("POST /api/discover/probe", http.HandlerFunc(d.handleAPIDiscoverProbe)))

	// Agent management API.
	mux.Handle("POST /api/agents", d.guard("POST /api/agents", http.HandlerFunc(d.handleAPIAddAgent)))
	mux.Handle("POST /api/agents/{name}/browse", d.guard("POST /api/agents/{name}/browse", http.HandlerFunc(d.handleAPIAgentBrowse)))
	mux.Handle("POST /api/agents/{name}/configure", d.guard("POST /api/agents/{name}/configure", http.HandlerFunc(d.handleAPIAgentConfigure)))
	mux.Handle("GET /api/agents/{name}/info", d.guard("GET /api/agents/{name}/info", http.HandlerFunc(d.handleAPIAgentInfo)))
	mux.Handle("GET /api/agents/{name}/plugins/{plugin}/browse", d.guard("GET /api/agents/{name}/plugins/{plugin}/browse", http.HandlerFunc(d.handleAPIWorkloadBrowse)))
	mux.Handle("GET /api/agents/{name}/plugins/{plugin}/object-config", d.guard("GET /api/agents/{name}/plugins/{plugin}/object-config", http.HandlerFunc(d.handleAPIGetObjectConfig)))
	mux.Handle("PUT /api/agents/{name}/plugins/{plugin}/object-config", d.guard("PUT /api/agents/{name}/plugins/{plugin}/object-config", http.HandlerFunc(d.handleAPISetObjectConfig)))
	mux.Handle("GET /api/workload/points", d.guard("GET /api/workload/points", http.HandlerFunc(d.handleAPIWorkloadPoints)))
	mux.Handle("GET /api/workload/catalog", d.guard("GET /api/workload/catalog", http.HandlerFunc(d.handleAPIWorkloadCatalog)))
	mux.Handle("POST /api/workload/restore-plan", d.guard("POST /api/workload/restore-plan", http.HandlerFunc(d.handleAPIWorkloadRestorePlan)))
	mux.Handle("GET /api/plugin-packages", d.guard("GET /api/plugin-packages", http.HandlerFunc(d.handleAPIPluginPackages)))
	mux.Handle("POST /api/plugin-packages", d.guard("POST /api/plugin-packages", http.HandlerFunc(d.handleAPIUploadPluginPackage)))
	mux.Handle("DELETE /api/plugin-packages/{file}", d.guard("DELETE /api/plugin-packages/{file}", http.HandlerFunc(d.handleAPIDeletePluginPackage)))
	mux.Handle("POST /api/agents/{name}/plugins", d.guard("POST /api/agents/{name}/plugins", http.HandlerFunc(d.handleAPIInstallAgentPlugin)))
	mux.Handle("DELETE /api/agents/{name}/plugins/{id}", d.guard("DELETE /api/agents/{name}/plugins/{id}", http.HandlerFunc(d.handleAPIRemoveAgentPlugin)))
	mux.Handle("POST /api/workload/restores", d.guard("POST /api/workload/restores", http.HandlerFunc(d.handleAPIStartWorkloadRestore)))
	mux.Handle("GET /api/workload/restores/{id}", d.guard("GET /api/workload/restores/{id}", http.HandlerFunc(d.handleAPIGetWorkloadRestore)))
	mux.Handle("DELETE /api/agents/{name}", d.guard("DELETE /api/agents/{name}", http.HandlerFunc(d.handleAPIRemoveAgent)))
	mux.Handle("POST /api/agents/{name}/suspend", d.guard("POST /api/agents/{name}/suspend", http.HandlerFunc(d.handleAPIAgentSuspend)))
	mux.Handle("POST /api/agents/{name}/unsuspend", d.guard("POST /api/agents/{name}/unsuspend", http.HandlerFunc(d.handleAPIAgentUnsuspend)))

	mux.Handle("GET /api/logs", d.guard("GET /api/logs", http.HandlerFunc(d.handleAPILogs)))
	mux.Handle("GET /api/logs/export", d.guard("GET /api/logs/export", http.HandlerFunc(d.handleAPILogsExport)))
	mux.Handle("GET /api/alerts", d.guard("GET /api/alerts", http.HandlerFunc(d.handleAPIGetAlerts)))
	mux.Handle("POST /api/alerts/{id}/resolve", d.guard("POST /api/alerts/{id}/resolve", http.HandlerFunc(d.handleAPIResolveAlert)))

	return crossOriginProtect(mux)
}
func (d *Dashboard) render(w http.ResponseWriter, r *http.Request, page string, data any) {
	t, ok := d.templates[d.language(r)][page]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Language", d.language(r))
	w.Header().Add("Vary", "Cookie, Accept-Language")

	if pd, ok := data.(pageData); ok {
		pd.Nav = navFor(r)
		pd.HasAuth = pd.HasAuth || pd.Nav.User != ""
		data = pd
	}
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
	return timeAgoIn(i18n.Get(i18n.Default), v)
}

// timeAgoIn renders a relative time ("5m ago") using catalog c.
func timeAgoIn(c *i18n.Catalog, v any) string {
	var t time.Time
	switch x := v.(type) {
	case time.Time:
		t = x
	case *time.Time:
		if x == nil {
			return c.T("never")
		}
		t = *x
	default:
		return "—"
	}
	if t.IsZero() {
		return c.T("never")
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return c.T("just now")
	case d < time.Hour:
		return c.Tf("{n}m ago", "n", int(d.Minutes()))
	case d < 24*time.Hour:
		return c.Tf("{n}h ago", "n", int(d.Hours()))
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return c.T("1 day ago")
		}
		return c.Tf("{n} days ago", "n", days)
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
