package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// repoNameRe validates repository names: alphanumeric + hyphens, 1-64 chars.
var repoNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,63}$`)

// RepoConfig defines a single repository entry in the multi-repo config.
type RepoConfig struct {
	Name               string `yaml:"name"`
	Type               string `yaml:"type"`           // "local" or "s3"
	Path               string `yaml:"path,omitempty"` // local only
	PasswordFile       string `yaml:"password_file,omitempty"`
	Endpoint           string `yaml:"endpoint,omitempty"`             // s3 only
	Bucket             string `yaml:"bucket,omitempty"`               // s3 only
	Region             string `yaml:"region,omitempty"`               // s3 only, defaults to "us-east-1"
	AccessKeyEncrypted string `yaml:"access_key_encrypted,omitempty"` // s3 only, base64(AES-256-GCM)
	SecretKeyEncrypted string `yaml:"secret_key_encrypted,omitempty"` // s3 only, base64(AES-256-GCM)
}

// ValidateRepoConfig validates a single repository configuration entry.
func ValidateRepoConfig(rc RepoConfig) error {
	if !repoNameRe.MatchString(rc.Name) {
		return fmt.Errorf("config: invalid repo name %q: must be 1-64 alphanumeric chars or hyphens, starting with alphanumeric", rc.Name)
	}
	switch rc.Type {
	case "local":
		if rc.Path == "" {
			return fmt.Errorf("config: repo %q: path is required for local type", rc.Name)
		}
		if !filepath.IsAbs(rc.Path) {
			return fmt.Errorf("config: repo %q: path must be absolute, got %q", rc.Name, rc.Path)
		}
	case "s3":
		if rc.Endpoint == "" {
			return fmt.Errorf("config: repo %q: endpoint is required for s3 type", rc.Name)
		}
		if rc.Bucket == "" {
			return fmt.Errorf("config: repo %q: bucket is required for s3 type", rc.Name)
		}
	default:
		return fmt.Errorf("config: repo %q: type must be \"local\" or \"s3\", got %q", rc.Name, rc.Type)
	}
	if rc.PasswordFile == "" {
		return fmt.Errorf("config: repo %q: password_file is required", rc.Name)
	}
	return nil
}

// ValidateRepositories validates the full list of repository configs.
func ValidateRepositories(repos []RepoConfig) error {
	names := make(map[string]bool, len(repos))
	for _, rc := range repos {
		if err := ValidateRepoConfig(rc); err != nil {
			return err
		}
		if names[rc.Name] {
			return fmt.Errorf("config: duplicate repository name %q", rc.Name)
		}
		names[rc.Name] = true
	}
	return nil
}

// Config represents the parsed configuration file contents.
type Config struct {
	Repo                   string   `yaml:"repo,omitempty"`
	PasswordFile           string   `yaml:"password_file,omitempty"`
	S3AccessKey            string   `yaml:"s3_access_key,omitempty"`
	S3SecretKey            string   `yaml:"s3_secret_key,omitempty"`
	S3Region               string   `yaml:"s3_region,omitempty"`
	Exclude                []string `yaml:"exclude,omitempty"`
	Parallel               int      `yaml:"parallel,omitempty"`
	ServePort              int      `yaml:"serve_port,omitempty"`
	ServeAdvertiseAddr     string   `yaml:"serve_advertise_addr,omitempty"`
	ServeTLSCert           string   `yaml:"serve_tls_cert,omitempty"`
	ServeTLSKey            string   `yaml:"serve_tls_key,omitempty"`
	ServeDashboardPort     int      `yaml:"serve_dashboard_port,omitempty"`
	ServeDashboardPassword string   `yaml:"serve_dashboard_password,omitempty"`
	AlertWebhookURL        string   `yaml:"alert_webhook_url,omitempty"`
	AlertStaleThreshold    int      `yaml:"alert_stale_threshold,omitempty"`
	AlertNotifierPlugins   []string `yaml:"alert_notifier_plugins,omitempty"`
	AgentServer            string   `yaml:"agent_server,omitempty"`
	AgentAPIKey            string   `yaml:"agent_api_key,omitempty"`
	AgentCACert            string   `yaml:"agent_ca_cert,omitempty"`
	AgentHostname          string   `yaml:"agent_hostname,omitempty"`
	AgentPaths             []string `yaml:"agent_paths,omitempty"`
	AgentExcludes          []string `yaml:"agent_excludes,omitempty"`
	HookPlugins            []string `yaml:"hook_plugins,omitempty"`
	FsScannerPlugin        string   `yaml:"fs_scanner_plugin,omitempty"`
	PluginDir              string   `yaml:"plugin_dir,omitempty"`
	UpdateCheck            bool     `yaml:"update_check,omitempty"`
	UpdateURL              string   `yaml:"update_url,omitempty"`

	// Repositories is the multi-repo configuration list.
	Repositories []RepoConfig `yaml:"repositories,omitempty"`

	// SyncWrites fsyncs local repository files after atomic rename (durability).
	SyncWrites bool `yaml:"sync_writes,omitempty"`
	// MinFreeReserveBytes is the minimum free disk space required before writing packs (local repos).
	MinFreeReserveBytes uint64 `yaml:"min_free_reserve_bytes,omitempty"`

	// Log retention and rotation (View Logs + on-disk circular files).
	LogMaxEntries     int    `yaml:"log_max_entries,omitempty"`
	LogFileEnabled    *bool  `yaml:"log_file_enabled,omitempty"`
	LogFilePath       string `yaml:"log_file_path,omitempty"`
	LogRotateMaxMB    int    `yaml:"log_rotate_max_mb,omitempty"`
	LogRotateMaxFiles int    `yaml:"log_rotate_max_files,omitempty"`

	// Integrations holds DR and cloud provider settings (secrets encrypted at rest).
	Integrations IntegrationsConfig `yaml:"integrations,omitempty"`

	// Sentinel holds Sentinel telemetry integration settings.
	Sentinel SentinelConfig `yaml:"sentinel,omitempty"`

	// Path is the resolved file path (not persisted in YAML).
	Path string `yaml:"-"`
	// Exists indicates whether the config file existed on disk.
	Exists bool `yaml:"-"`
}

// ResolvedKey is a configuration key with its effective value and source.
type ResolvedKey struct {
	Def    KeyDef
	Value  string
	Source string // "default", "config", "env", "flag"
}

// FlagChecker reports whether a CLI flag was explicitly set by the user.
type FlagChecker interface {
	Changed(name string) bool
}

// DefaultPath returns the default *user* config file path (~/.config/hitstor/config.yaml or equivalent).
// Use this as the creation target for new user configs.
func DefaultPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		home, err2 := os.UserHomeDir()
		if err2 != nil {
			return filepath.Join(".config", "hitstor", "config.yaml")
		}
		return filepath.Join(home, ".config", "hitstor", "config.yaml")
	}
	return filepath.Join(configDir, "hitstor", "config.yaml")
}

// SystemConfigPath is the system-wide config location used by packages,
// installers, the systemd service (hitstor-server), and dashboard.
const SystemConfigPath = "/etc/hitstor/config.yaml"

// EffectiveDefaultPath returns the config path to use when --config is not provided.
// We strongly prefer the system-wide /etc/hitstor/config.yaml (used by the service,
// dashboard, and packaging) when it exists on disk. This makes bare `hitstor` commands
// "just work" for admins on the server VM.
//
// Search order:
//  1. Explicit --config flag (handled by caller).
//  2. System config (/etc/hitstor/config.yaml) if it exists on disk.
//  3. User config (~/.config/hitstor/config.yaml) if it exists on disk.
//  4. User default path (for first-time creation on desktop).
func EffectiveDefaultPath() string {
	user := DefaultPath()

	// Strongly prefer the system-wide config when it exists *and* is readable.
	// This makes bare `hitstor` on a server (as the 'hits' user) reliably use
	// the config managed by the service + dashboard.
	if f, err := os.Open(SystemConfigPath); err == nil {
		f.Close()
		return SystemConfigPath
	}

	if f, err := os.Open(user); err == nil {
		f.Close()
		return user
	}
	return user
}

// Load reads and parses the YAML config file at the given path.
// If path is empty, EffectiveDefaultPath() is used (user config if present,
// falling back to /etc/hitstor/config.yaml on servers).
// Returns an empty Config with Exists=false if the file doesn't exist.
func Load(path string) (Config, error) {
	if path == "" {
		path = EffectiveDefaultPath()
	}

	var cfg Config
	cfg.Path = path

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		if os.IsPermission(err) {
			return cfg, fmt.Errorf("config: permission denied reading %s (try: sudo hitstor --config %s ... or fix ownership/permissions on /etc/hitstor)", path, path)
		}
		return cfg, fmt.Errorf("config: read %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("config: invalid YAML in %s: %w", path, err)
	}

	cfg.Path = path
	cfg.Exists = true

	// Backward compatibility: if repositories list is empty but legacy
	// repo field exists AND a password_file is configured, synthesize a
	// single-repo entry.  When password_file is absent the caller uses
	// the HITSTOR_PASSWORD env var via the legacy single-repo path in
	// cmd_serve.go, so we must NOT synthesize a multi-repo entry.
	if len(cfg.Repositories) == 0 && cfg.Repo != "" && cfg.PasswordFile != "" {
		legacy := RepoConfig{
			Name:         "default",
			PasswordFile: cfg.PasswordFile,
		}
		if strings.HasPrefix(cfg.Repo, "s3:") {
			legacy.Type = "s3"
			// Parse s3:endpoint/bucket format.
			rest := strings.TrimPrefix(cfg.Repo, "s3:")
			idx := strings.LastIndex(rest, "/")
			if idx > 0 {
				legacy.Endpoint = "https://" + rest[:idx]
				legacy.Bucket = rest[idx+1:]
			}
			legacy.Region = cfg.S3Region
			if legacy.Region == "" {
				legacy.Region = "us-east-1"
			}
		} else {
			legacy.Type = "local"
			legacy.Path = cfg.Repo
		}
		cfg.Repositories = []RepoConfig{legacy}
	}

	return cfg, nil
}

// Save writes the config to disk atomically via temp file + rename.
// Creates parent directories with 0700 if needed.
func Save(cfg Config) error {
	path := cfg.Path
	if path == "" {
		path = EffectiveDefaultPath()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("config: create directory %s: %w", dir, err)
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}

	// Atomic write: temp file in same directory, then rename.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("config: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("config: rename %s -> %s: %w", tmp, path, err)
	}

	return nil
}
