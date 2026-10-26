package config

import (
	"fmt"
	"strconv"
	"strings"
)

// KeyType classifies configuration value types.
type KeyType int

const (
	String KeyType = iota
	Int
	List
)

// KeyDef defines a known configuration key with its metadata and validation.
type KeyDef struct {
	Name        string
	Type        KeyType
	Default     string
	Description string
	Sensitive   bool
	EnvVar      string
	FlagName    string
	ValidateFn  func(string) error
	InitGroup   string // "common" or "s3"
}

// Registry is the ordered list of all known configuration keys.
var Registry = []KeyDef{
	{
		Name:        "repo",
		Type:        String,
		Default:     "",
		Description: "Repository location (path or s3:endpoint/bucket)",
		EnvVar:      "HITSTOR_REPO",
		FlagName:    "repo",
		ValidateFn:  validateRepo,
		InitGroup:   "common",
	},
	{
		Name:        "password_file",
		Type:        String,
		Default:     "",
		Description: "Path to password file",
		FlagName:    "password-file",
		ValidateFn:  validateNonEmpty,
		InitGroup:   "common",
	},
	{
		Name:        "s3_access_key",
		Type:        String,
		Default:     "",
		Description: "S3 access key",
		Sensitive:   true,
		FlagName:    "s3-access-key",
		ValidateFn:  validateNonEmpty,
		InitGroup:   "s3",
	},
	{
		Name:        "s3_secret_key",
		Type:        String,
		Default:     "",
		Description: "S3 secret key",
		Sensitive:   true,
		FlagName:    "s3-secret-key",
		ValidateFn:  validateNonEmpty,
		InitGroup:   "s3",
	},
	{
		Name:        "s3_region",
		Type:        String,
		Default:     "us-east-1",
		Description: "S3 region",
		FlagName:    "s3-region",
		ValidateFn:  validateNonEmpty,
		InitGroup:   "s3",
	},
	{
		Name:        "exclude",
		Type:        List,
		Default:     "",
		Description: "Exclude patterns",
		InitGroup:   "common",
	},
	{
		Name:        "parallel",
		Type:        Int,
		Default:     "0",
		Description: "Number of parallel workers (0 = auto)",
		FlagName:    "parallel",
		ValidateFn:  validateParallel,
		InitGroup:   "common",
	},
	{
		Name:        "sync_writes",
		Type:        String,
		Default:     "false",
		Description: "Fsync local repository files after atomic rename (true/false)",
		EnvVar:      "HITSTOR_SYNC_WRITES",
		ValidateFn:  validateBool,
		InitGroup:   "common",
	},
	{
		Name:        "min_free_reserve_bytes",
		Type:        Int,
		Default:     "1073741824",
		Description: "Minimum free disk space (bytes) required before writing packs",
		ValidateFn:  validateNonNegativeInt,
		InitGroup:   "common",
	},
	{
		Name:        "serve_port",
		Type:        Int,
		Default:     "8420",
		Description: "gRPC server listen port",
		FlagName:    "port",
		ValidateFn:  validatePort,
	},
	{
		Name:        "serve_tls_cert",
		Type:        String,
		Default:     "",
		Description: "Path to TLS certificate file for server",
		FlagName:    "tls-cert",
		ValidateFn:  validateNonEmpty,
	},
	{
		Name:        "serve_tls_key",
		Type:        String,
		Default:     "",
		Description: "Path to TLS private key file for server",
		FlagName:    "tls-key",
		ValidateFn:  validateNonEmpty,
	},
	{
		Name:        "serve_advertise_addr",
		Type:        String,
		Default:     "",
		Description: "Server address sent to agents during registration (host:port, auto-detected if empty)",
	},
	{
		Name:        "serve_dashboard_port",
		Type:        Int,
		Default:     "8421",
		Description: "HTTP dashboard listen port",
		FlagName:    "dashboard-port",
		ValidateFn:  validatePort,
	},
	{
		Name:        "serve_dashboard_password",
		Type:        String,
		Default:     "",
		Description: "Dashboard login password (empty = open access)",
		Sensitive:   true,
	},
	{
		Name:        "alert_webhook_url",
		Type:        String,
		Default:     "",
		Description: "Webhook URL for alert notifications (POST JSON)",
	},
	{
		Name:        "alert_stale_threshold",
		Type:        Int,
		Default:     "25",
		Description: "Hours before an agent is considered stale",
		ValidateFn:  validateStaleThreshold,
	},
	{
		Name:        "alert_notifier_plugins",
		Type:        List,
		Default:     "",
		Description: "Notifier plugin names to load (comma-separated)",
	},
	{
		Name:        "agent_server",
		Type:        String,
		Default:     "",
		Description: "Remote server address (host:port)",
		EnvVar:      "HITSTOR_AGENT_SERVER",
		FlagName:    "server",
		ValidateFn:  validateNonEmpty,
	},
	{
		Name:        "agent_api_key",
		Type:        String,
		Default:     "",
		Description: "Agent API key for authentication",
		Sensitive:   true,
		EnvVar:      "HITSTOR_AGENT_API_KEY",
		ValidateFn:  validateAPIKey,
	},
	{
		Name:        "agent_ca_cert",
		Type:        String,
		Default:     "",
		Description: "Path to CA certificate for TLS verification",
		ValidateFn:  validateNonEmpty,
	},
	{
		Name:        "agent_hostname",
		Type:        String,
		Default:     "",
		Description: "Override hostname sent to server",
	},
	{
		Name:        "agent_paths",
		Type:        List,
		Default:     "",
		Description: "Default backup paths",
	},
	{
		Name:        "agent_excludes",
		Type:        List,
		Default:     "",
		Description: "Default exclude patterns for agent",
	},
	{
		Name:        "hook_plugins",
		Type:        List,
		Default:     "",
		Description: "Hook plugin names to load (comma-separated)",
	},
	{
		Name:        "fs_scanner_plugin",
		Type:        String,
		Default:     "",
		Description: "Filesystem scanner plugin name to use for scanning",
	},
	{
		Name:        "plugin_dir",
		Type:        String,
		Default:     "",
		Description: "Directory to search for plugin binaries",
	},
	{
		Name:        "update_check",
		Type:        String,
		Default:     "true",
		Description: "Enable automatic version checks (true/false)",
	},
	{
		Name:        "update_url",
		Type:        String,
		Default:     "",
		Description: "Custom update check URL (empty = GitHub Releases)",
	},
}

// LookupKey finds a key definition by name.
func LookupKey(name string) (KeyDef, bool) {
	for _, k := range Registry {
		if k.Name == name {
			return k, true
		}
	}
	return KeyDef{}, false
}

// IsListKey returns true if the named key is a list type.
func IsListKey(name string) bool {
	k, ok := LookupKey(name)
	return ok && k.Type == List
}

// ValidateKey validates a value for the named key.
func ValidateKey(name, value string) error {
	k, ok := LookupKey(name)
	if !ok {
		return fmt.Errorf("unknown config key: %s", name)
	}
	if k.ValidateFn != nil {
		return k.ValidateFn(value)
	}
	return nil
}

// MaskValue masks a sensitive value, showing only the last 4 characters.
func MaskValue(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	return "****" + value[len(value)-4:]
}

func validateRepo(value string) error {
	if value == "" {
		return fmt.Errorf("repo must not be empty")
	}
	if strings.HasPrefix(value, "s3:") {
		rest := strings.TrimPrefix(value, "s3:")
		idx := strings.LastIndex(rest, "/")
		if idx < 0 || rest[:idx] == "" || rest[idx+1:] == "" {
			return fmt.Errorf("invalid S3 repo format: expected s3:endpoint/bucket")
		}
	}
	return nil
}

func validateNonEmpty(value string) error {
	if value == "" {
		return fmt.Errorf("value must not be empty")
	}
	return nil
}

func validateParallel(value string) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("invalid value for parallel: expected non-negative integer, got %q", value)
	}
	if n < 0 {
		return fmt.Errorf("invalid value for parallel: expected non-negative integer, got %q", value)
	}
	return nil
}

func validateAPIKey(value string) error {
	if len(value) != 64 {
		return fmt.Errorf("invalid API key: expected 64-character hex string, got %d characters", len(value))
	}
	for _, c := range value {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return fmt.Errorf("invalid API key: contains non-hex character %q", c)
		}
	}
	return nil
}

func validateStaleThreshold(value string) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("invalid stale threshold: expected positive integer (hours), got %q", value)
	}
	if n < 1 {
		return fmt.Errorf("invalid stale threshold: must be at least 1 hour, got %d", n)
	}
	return nil
}

func validateBool(value string) error {
	switch value {
	case "true", "false", "1", "0":
		return nil
	default:
		return fmt.Errorf("invalid value: expected true/false or 1/0, got %q", value)
	}
}

func validateNonNegativeInt(value string) error {
	if _, err := strconv.ParseUint(value, 10, 64); err != nil {
		return fmt.Errorf("invalid value: expected non-negative integer, got %q", value)
	}
	return nil
}

func validatePort(value string) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("invalid port: expected integer, got %q", value)
	}
	if n < 1 || n > 65535 {
		return fmt.Errorf("invalid port: expected 1-65535, got %d", n)
	}
	return nil
}
