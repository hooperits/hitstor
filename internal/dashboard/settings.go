package dashboard

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/crypto"
	"github.com/hooperits/hitstor/internal/logstore"
)

// SettingsView is the masked settings payload returned by GET /api/settings.
type SettingsView struct {
	General         GeneralSettingsView       `json:"general"`
	Security        SecuritySettingsView      `json:"security"`
	Network         NetworkSettingsView       `json:"network"`
	Notifications   NotificationsSettingsView `json:"notifications"`
	Integrations    IntegrationsSettingsView  `json:"integrations"`
	RestartRequired bool                      `json:"restart_required"`
}

type GeneralSettingsView struct {
	UpdateCheck         bool   `json:"update_check"`
	UpdateURL           string `json:"update_url"`
	Parallel            int    `json:"parallel"`
	SyncWrites          bool   `json:"sync_writes"`
	MinFreeReserveBytes uint64 `json:"min_free_reserve_bytes"`
	LogMaxEntries       int    `json:"log_max_entries"`
	LogFileEnabled      bool   `json:"log_file_enabled"`
	LogFilePath         string `json:"log_file_path"`
	LogRotateMaxMB      int    `json:"log_rotate_max_mb"`
	LogRotateMaxFiles   int    `json:"log_rotate_max_files"`
}

type SecuritySettingsView struct {
	DashboardPasswordConfigured bool   `json:"dashboard_password_configured"`
	DashboardPasswordHint       string `json:"dashboard_password_hint,omitempty"`
	TLSCertPath                 string `json:"tls_cert_path,omitempty"`
	TLSKeyPath                  string `json:"tls_key_path,omitempty"`
}

type NetworkSettingsView struct {
	ServePort          int    `json:"serve_port"`
	ServeDashboardPort int    `json:"serve_dashboard_port"`
	ServeAdvertiseAddr string `json:"serve_advertise_addr"`
}

type NotificationsSettingsView struct {
	AlertWebhookURL      string   `json:"alert_webhook_url"`
	AlertStaleThreshold  int      `json:"alert_stale_threshold"`
	AlertNotifierPlugins []string `json:"alert_notifier_plugins"`
}

type IntegrationsSettingsView struct {
	GeminiConfigured    bool   `json:"gemini_configured"`
	GeminiAPIKeyHint    string `json:"gemini_api_key_hint,omitempty"`
	GeminiModel         string `json:"gemini_model"`
	EnvOverridesPresent bool   `json:"env_overrides_present"`
}

type GeneralSettingsUpdate struct {
	UpdateCheck         *bool   `json:"update_check,omitempty"`
	UpdateURL           *string `json:"update_url,omitempty"`
	Parallel            *int    `json:"parallel,omitempty"`
	SyncWrites          *bool   `json:"sync_writes,omitempty"`
	MinFreeReserveBytes *uint64 `json:"min_free_reserve_bytes,omitempty"`
	LogMaxEntries       *int    `json:"log_max_entries,omitempty"`
	LogFileEnabled      *bool   `json:"log_file_enabled,omitempty"`
	LogFilePath         *string `json:"log_file_path,omitempty"`
	LogRotateMaxMB      *int    `json:"log_rotate_max_mb,omitempty"`
	LogRotateMaxFiles   *int    `json:"log_rotate_max_files,omitempty"`
}

type SecuritySettingsUpdate struct {
	DashboardPassword string `json:"dashboard_password,omitempty"`
}

type NetworkSettingsUpdate struct {
	ServePort          *int    `json:"serve_port,omitempty"`
	ServeDashboardPort *int    `json:"serve_dashboard_port,omitempty"`
	ServeAdvertiseAddr *string `json:"serve_advertise_addr,omitempty"`
}

type NotificationsSettingsUpdate struct {
	AlertWebhookURL      *string  `json:"alert_webhook_url,omitempty"`
	AlertStaleThreshold  *int     `json:"alert_stale_threshold,omitempty"`
	AlertNotifierPlugins []string `json:"alert_notifier_plugins,omitempty"`
}

type IntegrationsSettingsUpdate struct {
	GeminiAPIKey string `json:"gemini_api_key,omitempty"`
	GeminiModel  string `json:"gemini_model,omitempty"`
}

func (d *Dashboard) requireConfig() (*config.Config, error) {
	if d.cfg == nil {
		return nil, fmt.Errorf("server configuration is not available")
	}
	return d.cfg, nil
}

func (d *Dashboard) encryptionKey() ([32]byte, bool) {
	if d.repos != nil {
		return d.repos.EncryptKey()
	}
	if d.repo != nil {
		return d.repo.Key().EncryptKey, true
	}
	return [32]byte{}, false
}

func (d *Dashboard) buildSettingsView() (SettingsView, error) {
	cfg, err := d.requireConfig()
	if err != nil {
		return SettingsView{}, err
	}

	encKey, hasKey := d.encryptionKey()
	resolved, resolveErr := config.ResolveIntegrations(*cfg, encKey, hasKey)

	staleThreshold := cfg.AlertStaleThreshold
	if staleThreshold == 0 {
		staleThreshold = 25
	}

	parallel := cfg.Parallel
	syncWrites := cfg.SyncWrites
	minReserve := cfg.MinFreeReserveBytes
	if minReserve == 0 {
		minReserve = 1073741824
	}

	servePort := cfg.ServePort
	if servePort == 0 {
		servePort = 8420
	}
	dashPort := cfg.ServeDashboardPort
	if dashPort == 0 {
		dashPort = 8421
	}

	geminiHint := ""
	if resolveErr == nil && resolved.GeminiAPIKey != "" {
		geminiHint = config.MaskSecret(resolved.GeminiAPIKey)
	}

	logCfg := config.LogStoreConfig(*cfg)

	return SettingsView{
		General: GeneralSettingsView{
			UpdateCheck:         cfg.UpdateCheck || !cfg.Exists,
			UpdateURL:           cfg.UpdateURL,
			Parallel:            parallel,
			SyncWrites:          syncWrites,
			MinFreeReserveBytes: minReserve,
			LogMaxEntries:       logCfg.MaxEntries,
			LogFileEnabled:      logCfg.FileEnabled,
			LogFilePath:         logCfg.FilePath,
			LogRotateMaxMB:      logCfg.RotateMaxMB,
			LogRotateMaxFiles:   logCfg.RotateMaxFiles,
		},
		Security: SecuritySettingsView{
			DashboardPasswordConfigured: cfg.ServeDashboardPassword != "" || d.opts.Password != "",
			DashboardPasswordHint:       config.MaskSecret(firstNonEmpty(cfg.ServeDashboardPassword, d.opts.Password)),
			TLSCertPath:                 cfg.ServeTLSCert,
			TLSKeyPath:                  cfg.ServeTLSKey,
		},
		Network: NetworkSettingsView{
			ServePort:          servePort,
			ServeDashboardPort: dashPort,
			ServeAdvertiseAddr: cfg.ServeAdvertiseAddr,
		},
		Notifications: NotificationsSettingsView{
			AlertWebhookURL:      cfg.AlertWebhookURL,
			AlertStaleThreshold:  staleThreshold,
			AlertNotifierPlugins: append([]string(nil), cfg.AlertNotifierPlugins...),
		},
		Integrations: IntegrationsSettingsView{
			GeminiConfigured:    resolveErr == nil && resolved.GeminiAPIKey != "",
			GeminiAPIKeyHint:    geminiHint,
			GeminiModel:         config.NormalizeGeminiModel(cfg.Integrations.GeminiModel),
			EnvOverridesPresent: integrationsEnvOverridesPresent(),
		},
	}, nil
}

func integrationsEnvOverridesPresent() bool {
	for _, key := range []string{
		"GEMINI_API_KEY",
	} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

func (d *Dashboard) saveConfig() error {
	cfg, err := d.requireConfig()
	if err != nil {
		return err
	}
	if cfg.Path == "" {
		cfg.Path = config.DefaultPath()
	}
	cfg.Exists = true
	return config.Save(*cfg)
}

func (d *Dashboard) applyGeneralSettings(req GeneralSettingsUpdate) error {
	cfg, err := d.requireConfig()
	if err != nil {
		return err
	}
	if req.UpdateCheck != nil {
		cfg.UpdateCheck = *req.UpdateCheck
	}
	if req.UpdateURL != nil {
		cfg.UpdateURL = strings.TrimSpace(*req.UpdateURL)
	}
	if req.Parallel != nil {
		if err := validateParallelInt(*req.Parallel); err != nil {
			return err
		}
		cfg.Parallel = *req.Parallel
	}
	if req.SyncWrites != nil {
		cfg.SyncWrites = *req.SyncWrites
	}
	if req.MinFreeReserveBytes != nil {
		cfg.MinFreeReserveBytes = *req.MinFreeReserveBytes
	}
	if req.LogMaxEntries != nil {
		cfg.LogMaxEntries = *req.LogMaxEntries
	}
	if req.LogFileEnabled != nil {
		cfg.LogFileEnabled = req.LogFileEnabled
	}
	if req.LogFilePath != nil {
		cfg.LogFilePath = strings.TrimSpace(*req.LogFilePath)
	}
	if req.LogRotateMaxMB != nil {
		cfg.LogRotateMaxMB = *req.LogRotateMaxMB
	}
	if req.LogRotateMaxFiles != nil {
		cfg.LogRotateMaxFiles = *req.LogRotateMaxFiles
	}
	if err := d.saveConfig(); err != nil {
		return err
	}
	return logstore.ReloadConfig(config.LogStoreConfig(*cfg))
}

func (d *Dashboard) applySecuritySettings(req SecuritySettingsUpdate) (restartRequired bool, err error) {
	cfg, err := d.requireConfig()
	if err != nil {
		return false, err
	}
	if req.DashboardPassword != "" {
		// Store an scrypt hash so the config file never holds the plaintext.
		hash, hashErr := crypto.HashPassword(req.DashboardPassword)
		if hashErr != nil {
			return false, fmt.Errorf("hash dashboard password: %w", hashErr)
		}
		cfg.ServeDashboardPassword = hash
		d.opts.Password = hash
		restartRequired = true
	}
	if err := d.saveConfig(); err != nil {
		return false, err
	}
	return restartRequired, nil
}

func (d *Dashboard) applyNetworkSettings(req NetworkSettingsUpdate) (restartRequired bool, err error) {
	cfg, err := d.requireConfig()
	if err != nil {
		return false, err
	}
	if req.ServePort != nil {
		if err := validatePortInt(*req.ServePort); err != nil {
			return false, err
		}
		cfg.ServePort = *req.ServePort
		restartRequired = true
	}
	if req.ServeDashboardPort != nil {
		if err := validatePortInt(*req.ServeDashboardPort); err != nil {
			return false, err
		}
		cfg.ServeDashboardPort = *req.ServeDashboardPort
		restartRequired = true
	}
	if req.ServeAdvertiseAddr != nil {
		cfg.ServeAdvertiseAddr = strings.TrimSpace(*req.ServeAdvertiseAddr)
		restartRequired = true
	}
	if err := d.saveConfig(); err != nil {
		return false, err
	}
	return restartRequired, nil
}

func (d *Dashboard) applyNotificationsSettings(req NotificationsSettingsUpdate) error {
	cfg, err := d.requireConfig()
	if err != nil {
		return err
	}
	if req.AlertWebhookURL != nil {
		cfg.AlertWebhookURL = strings.TrimSpace(*req.AlertWebhookURL)
	}
	if req.AlertStaleThreshold != nil {
		if *req.AlertStaleThreshold < 1 || *req.AlertStaleThreshold > 8760 {
			return fmt.Errorf("alert_stale_threshold must be between 1 and 8760 hours")
		}
		cfg.AlertStaleThreshold = *req.AlertStaleThreshold
	}
	if req.AlertNotifierPlugins != nil {
		cfg.AlertNotifierPlugins = append([]string(nil), req.AlertNotifierPlugins...)
	}
	return d.saveConfig()
}

func (d *Dashboard) applyIntegrationsSettings(req IntegrationsSettingsUpdate) error {
	cfg, err := d.requireConfig()
	if err != nil {
		return err
	}
	ic := &cfg.Integrations

	if req.GeminiAPIKey != "" {
		encKey, hasKey := d.encryptionKey()
		if !hasKey {
			return fmt.Errorf("no encryption key available (open a local repo first)")
		}
		if err := config.SetGeminiAPIKey(ic, encKey, req.GeminiAPIKey); err != nil {
			return err
		}
	}
	if req.GeminiModel != "" {
		ic.GeminiModel = config.NormalizeGeminiModel(req.GeminiModel)
	}

	if ic.GeminiModel == "" {
		ic.GeminiModel = config.DefaultGeminiModel
	}
	if err := d.saveConfig(); err != nil {
		return err
	}
	return d.refreshIntegrationsRuntime()
}

func (d *Dashboard) StaleThresholdDuration() time.Duration {
	cfg, err := d.requireConfig()
	if err != nil {
		return 25 * time.Hour
	}
	hours := cfg.AlertStaleThreshold
	if hours <= 0 {
		hours = 25
	}
	return time.Duration(hours) * time.Hour
}

func (d *Dashboard) refreshIntegrationsRuntime() error {
	cfg, err := d.requireConfig()
	if err != nil {
		return err
	}
	encKey, hasKey := d.encryptionKey()
	return config.ApplyIntegrationsEnv(*cfg, encKey, hasKey)
}

func validatePortInt(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

func validateParallelInt(parallel int) error {
	if parallel < 0 {
		return fmt.Errorf("parallel must be >= 0")
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
