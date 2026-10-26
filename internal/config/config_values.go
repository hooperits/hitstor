package config

import (
	"fmt"
	"strconv"
	"strings"
)

// GetConfigValue extracts a field value from Config by key name.
func GetConfigValue(cfg Config, keyName string) string {
	switch keyName {
	case "repo":
		return cfg.Repo
	case "password_file":
		return cfg.PasswordFile
	case "s3_access_key":
		return cfg.S3AccessKey
	case "s3_secret_key":
		return cfg.S3SecretKey
	case "s3_region":
		return cfg.S3Region
	case "exclude":
		return strings.Join(cfg.Exclude, ", ")
	case "parallel":
		return strconv.Itoa(cfg.Parallel)
	case "serve_port":
		return strconv.Itoa(cfg.ServePort)
	case "serve_advertise_addr":
		return cfg.ServeAdvertiseAddr
	case "serve_tls_cert":
		return cfg.ServeTLSCert
	case "serve_tls_key":
		return cfg.ServeTLSKey
	case "serve_dashboard_port":
		return strconv.Itoa(cfg.ServeDashboardPort)
	case "serve_dashboard_password":
		return cfg.ServeDashboardPassword
	case "alert_webhook_url":
		return cfg.AlertWebhookURL
	case "alert_stale_threshold":
		return strconv.Itoa(cfg.AlertStaleThreshold)
	case "alert_notifier_plugins":
		return strings.Join(cfg.AlertNotifierPlugins, ", ")
	case "agent_server":
		return cfg.AgentServer
	case "agent_api_key":
		return cfg.AgentAPIKey
	case "agent_ca_cert":
		return cfg.AgentCACert
	case "agent_hostname":
		return cfg.AgentHostname
	case "agent_paths":
		return strings.Join(cfg.AgentPaths, ", ")
	case "agent_excludes":
		return strings.Join(cfg.AgentExcludes, ", ")
	case "hook_plugins":
		return strings.Join(cfg.HookPlugins, ", ")
	case "fs_scanner_plugin":
		return cfg.FsScannerPlugin
	case "plugin_dir":
		return cfg.PluginDir
	case "update_check":
		if cfg.UpdateCheck {
			return "true"
		}
		return "false"
	case "update_url":
		return cfg.UpdateURL
	case "sync_writes":
		if cfg.SyncWrites {
			return "true"
		}
		return "false"
	case "min_free_reserve_bytes":
		return strconv.FormatUint(cfg.MinFreeReserveBytes, 10)
	default:
		return ""
	}
}

// SetConfigValue sets a field in the Config by key name.
func SetConfigValue(cfg *Config, keyName, value string) {
	switch keyName {
	case "repo":
		cfg.Repo = value
	case "password_file":
		cfg.PasswordFile = value
	case "s3_access_key":
		cfg.S3AccessKey = value
	case "s3_secret_key":
		cfg.S3SecretKey = value
	case "s3_region":
		cfg.S3Region = value
	case "exclude":
		if value == "" {
			cfg.Exclude = nil
		} else {
			parts := strings.Split(value, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			cfg.Exclude = parts
		}
	case "parallel":
		n, _ := strconv.Atoi(value)
		cfg.Parallel = n
	case "serve_port":
		n, _ := strconv.Atoi(value)
		cfg.ServePort = n
	case "serve_advertise_addr":
		cfg.ServeAdvertiseAddr = value
	case "serve_tls_cert":
		cfg.ServeTLSCert = value
	case "serve_tls_key":
		cfg.ServeTLSKey = value
	case "serve_dashboard_port":
		n, _ := strconv.Atoi(value)
		cfg.ServeDashboardPort = n
	case "serve_dashboard_password":
		cfg.ServeDashboardPassword = value
	case "alert_webhook_url":
		cfg.AlertWebhookURL = value
	case "alert_stale_threshold":
		n, _ := strconv.Atoi(value)
		cfg.AlertStaleThreshold = n
	case "alert_notifier_plugins":
		if value == "" {
			cfg.AlertNotifierPlugins = nil
		} else {
			parts := strings.Split(value, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			cfg.AlertNotifierPlugins = parts
		}
	case "agent_server":
		cfg.AgentServer = value
	case "agent_api_key":
		cfg.AgentAPIKey = value
	case "agent_ca_cert":
		cfg.AgentCACert = value
	case "agent_hostname":
		cfg.AgentHostname = value
	case "agent_paths":
		if value == "" {
			cfg.AgentPaths = nil
		} else {
			parts := strings.Split(value, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			cfg.AgentPaths = parts
		}
	case "agent_excludes":
		if value == "" {
			cfg.AgentExcludes = nil
		} else {
			parts := strings.Split(value, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			cfg.AgentExcludes = parts
		}
	case "hook_plugins":
		if value == "" {
			cfg.HookPlugins = nil
		} else {
			parts := strings.Split(value, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			cfg.HookPlugins = parts
		}
	case "fs_scanner_plugin":
		cfg.FsScannerPlugin = value
	case "plugin_dir":
		cfg.PluginDir = value
	case "update_check":
		cfg.UpdateCheck = value == "true"
	case "update_url":
		cfg.UpdateURL = value
	case "sync_writes":
		cfg.SyncWrites = value == "true" || value == "1"
	case "min_free_reserve_bytes":
		n, _ := strconv.ParseUint(value, 10, 64)
		cfg.MinFreeReserveBytes = n
	}
}

// ZeroConfigValue sets a field in the Config to its zero value.
func ZeroConfigValue(cfg *Config, keyName string) {
	switch keyName {
	case "repo":
		cfg.Repo = ""
	case "password_file":
		cfg.PasswordFile = ""
	case "s3_access_key":
		cfg.S3AccessKey = ""
	case "s3_secret_key":
		cfg.S3SecretKey = ""
	case "s3_region":
		cfg.S3Region = ""
	case "exclude":
		cfg.Exclude = nil
	case "parallel":
		cfg.Parallel = 0
	case "serve_port":
		cfg.ServePort = 0
	case "serve_advertise_addr":
		cfg.ServeAdvertiseAddr = ""
	case "serve_tls_cert":
		cfg.ServeTLSCert = ""
	case "serve_tls_key":
		cfg.ServeTLSKey = ""
	case "serve_dashboard_port":
		cfg.ServeDashboardPort = 0
	case "serve_dashboard_password":
		cfg.ServeDashboardPassword = ""
	case "alert_webhook_url":
		cfg.AlertWebhookURL = ""
	case "alert_stale_threshold":
		cfg.AlertStaleThreshold = 0
	case "alert_notifier_plugins":
		cfg.AlertNotifierPlugins = nil
	case "agent_server":
		cfg.AgentServer = ""
	case "agent_api_key":
		cfg.AgentAPIKey = ""
	case "agent_ca_cert":
		cfg.AgentCACert = ""
	case "agent_hostname":
		cfg.AgentHostname = ""
	case "agent_paths":
		cfg.AgentPaths = nil
	case "agent_excludes":
		cfg.AgentExcludes = nil
	case "hook_plugins":
		cfg.HookPlugins = nil
	case "fs_scanner_plugin":
		cfg.FsScannerPlugin = ""
	case "plugin_dir":
		cfg.PluginDir = ""
	case "update_check":
		cfg.UpdateCheck = false
	case "update_url":
		cfg.UpdateURL = ""
	case "sync_writes":
		cfg.SyncWrites = false
	case "min_free_reserve_bytes":
		cfg.MinFreeReserveBytes = 0
	}
}

// IsZeroValue returns true if the config field for the given key is its zero value.
func IsZeroValue(cfg Config, keyName string) bool {
	switch keyName {
	case "repo":
		return cfg.Repo == ""
	case "password_file":
		return cfg.PasswordFile == ""
	case "s3_access_key":
		return cfg.S3AccessKey == ""
	case "s3_secret_key":
		return cfg.S3SecretKey == ""
	case "s3_region":
		return cfg.S3Region == ""
	case "exclude":
		return len(cfg.Exclude) == 0
	case "parallel":
		return cfg.Parallel == 0
	case "serve_port":
		return cfg.ServePort == 0
	case "serve_advertise_addr":
		return cfg.ServeAdvertiseAddr == ""
	case "serve_tls_cert":
		return cfg.ServeTLSCert == ""
	case "serve_tls_key":
		return cfg.ServeTLSKey == ""
	case "serve_dashboard_port":
		return cfg.ServeDashboardPort == 0
	case "serve_dashboard_password":
		return cfg.ServeDashboardPassword == ""
	case "alert_webhook_url":
		return cfg.AlertWebhookURL == ""
	case "alert_stale_threshold":
		return cfg.AlertStaleThreshold == 0
	case "alert_notifier_plugins":
		return len(cfg.AlertNotifierPlugins) == 0
	case "agent_server":
		return cfg.AgentServer == ""
	case "agent_api_key":
		return cfg.AgentAPIKey == ""
	case "agent_ca_cert":
		return cfg.AgentCACert == ""
	case "agent_hostname":
		return cfg.AgentHostname == ""
	case "agent_paths":
		return len(cfg.AgentPaths) == 0
	case "agent_excludes":
		return len(cfg.AgentExcludes) == 0
	case "hook_plugins":
		return len(cfg.HookPlugins) == 0
	case "fs_scanner_plugin":
		return cfg.FsScannerPlugin == ""
	case "plugin_dir":
		return cfg.PluginDir == ""
	case "update_check":
		return !cfg.UpdateCheck
	case "update_url":
		return cfg.UpdateURL == ""
	case "sync_writes":
		return !cfg.SyncWrites
	case "min_free_reserve_bytes":
		return cfg.MinFreeReserveBytes == 0
	default:
		return true
	}
}

// ResolveAll computes the effective value and source for each key.
// Precedence: flag > env > config > default.
func ResolveAll(cfg Config, flags FlagChecker, envLookup func(string) (string, bool)) []ResolvedKey {
	result := make([]ResolvedKey, 0, len(Registry))
	for _, def := range Registry {
		rk := ResolvedKey{Def: def, Value: def.Default, Source: "default"}

		// Config overrides default (if file existed and value is non-zero).
		if cfg.Exists && !IsZeroValue(cfg, def.Name) {
			rk.Value = GetConfigValue(cfg, def.Name)
			rk.Source = "config"
		}

		// Env overrides config.
		if def.EnvVar != "" {
			if val, ok := envLookup(def.EnvVar); ok && val != "" {
				rk.Value = val
				rk.Source = "env"
			}
		}

		// Flag overrides env.
		if def.FlagName != "" && flags != nil && flags.Changed(def.FlagName) {
			rk.Value = getFlagValue(cfg, def, flags, envLookup)
			rk.Source = "flag"
		}

		result = append(result, rk)
	}
	return result
}

// getFlagValue reads the effective flag value. Since we can't access Cobra flag
// values directly from this package, we reconstruct from the final merged state
// in root.go. The flag checker only tells us if it was changed; the actual value
// comes from the same precedence chain (flag wins, so the final repo/parallel
// values are the flag values if Changed=true).
func getFlagValue(cfg Config, def KeyDef, flags FlagChecker, envLookup func(string) (string, bool)) string {
	// When a flag is changed, the effective value from the config struct
	// represents what was loaded. The actual flag value is tracked externally.
	// For ResolveAll, the caller should ensure cfg reflects the final state.
	return GetConfigValue(cfg, def.Name)
}

// Set sets a config value by key name, persisting to disk.
func Set(path, keyName, value string) (oldValue string, err error) {
	_, ok := LookupKey(keyName)
	if !ok {
		return "", fmt.Errorf("unknown config key: %s", keyName)
	}

	if err := ValidateKey(keyName, value); err != nil {
		return "", err
	}

	cfg, err := Load(path)
	if err != nil {
		return "", err
	}

	oldValue = GetConfigValue(cfg, keyName)
	SetConfigValue(&cfg, keyName, value)
	cfg.Path = path
	if cfg.Path == "" {
		cfg.Path = EffectiveDefaultPath()
	}

	if err := Save(cfg); err != nil {
		return "", err
	}
	return oldValue, nil
}

// Unset removes a config value by key name, reverting to default.
func Unset(path, keyName string) (oldValue string, wasSet bool, err error) {
	_, ok := LookupKey(keyName)
	if !ok {
		return "", false, fmt.Errorf("unknown config key: %s", keyName)
	}

	cfg, err := Load(path)
	if err != nil {
		return "", false, err
	}

	wasSet = !IsZeroValue(cfg, keyName)
	oldValue = GetConfigValue(cfg, keyName)
	ZeroConfigValue(&cfg, keyName)
	cfg.Path = path
	if cfg.Path == "" {
		cfg.Path = EffectiveDefaultPath()
	}

	if err := Save(cfg); err != nil {
		return "", false, err
	}
	return oldValue, wasSet, nil
}

// AddToList appends a value to a list-type config key.
func AddToList(path, keyName, value string) (newList []string, err error) {
	def, ok := LookupKey(keyName)
	if !ok {
		return nil, fmt.Errorf("unknown config key: %s", keyName)
	}
	if def.Type != List {
		return nil, fmt.Errorf("key %s is not a list type; use \"config set\" instead", keyName)
	}

	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}

	list := getListValue(cfg, keyName)
	list = append(list, value)
	setListValue(&cfg, keyName, list)
	cfg.Path = path
	if cfg.Path == "" {
		cfg.Path = EffectiveDefaultPath()
	}

	if err := Save(cfg); err != nil {
		return nil, err
	}
	return list, nil
}

// RemoveFromList removes a value from a list-type config key by exact match.
func RemoveFromList(path, keyName, value string) (newList []string, found bool, err error) {
	def, ok := LookupKey(keyName)
	if !ok {
		return nil, false, fmt.Errorf("unknown config key: %s", keyName)
	}
	if def.Type != List {
		return nil, false, fmt.Errorf("key %s is not a list type; use \"config unset\" instead", keyName)
	}

	cfg, err := Load(path)
	if err != nil {
		return nil, false, err
	}

	list := getListValue(cfg, keyName)
	var filtered []string
	for _, item := range list {
		if item == value {
			found = true
			continue
		}
		filtered = append(filtered, item)
	}
	setListValue(&cfg, keyName, filtered)
	cfg.Path = path
	if cfg.Path == "" {
		cfg.Path = EffectiveDefaultPath()
	}

	if err := Save(cfg); err != nil {
		return nil, false, err
	}
	return filtered, found, nil
}

func getListValue(cfg Config, keyName string) []string {
	switch keyName {
	case "exclude":
		return cfg.Exclude
	case "agent_paths":
		return cfg.AgentPaths
	case "agent_excludes":
		return cfg.AgentExcludes
	case "alert_notifier_plugins":
		return cfg.AlertNotifierPlugins
	case "hook_plugins":
		return cfg.HookPlugins
	default:
		return nil
	}
}

func setListValue(cfg *Config, keyName string, list []string) {
	switch keyName {
	case "exclude":
		cfg.Exclude = list
	case "agent_paths":
		cfg.AgentPaths = list
	case "agent_excludes":
		cfg.AgentExcludes = list
	case "alert_notifier_plugins":
		cfg.AlertNotifierPlugins = list
	case "hook_plugins":
		cfg.HookPlugins = list
	}
}

// Validation functions.
