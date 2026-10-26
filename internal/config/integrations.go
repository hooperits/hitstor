package config

import (
	"fmt"
	"os"
	"strings"
)

const (
	DefaultGeminiModel   = "gemini-1.5-pro"
	DefaultSentinelModel = "gemini-1.5-flash"
)

// IntegrationsConfig holds DR and AI integration settings persisted in config.yaml.
type IntegrationsConfig struct {
	GeminiAPIKeyEncrypted string `yaml:"gemini_api_key_encrypted,omitempty"`
	GeminiModel           string `yaml:"gemini_model,omitempty"`
}

// SentinelConfig holds Sentinel telemetry integration settings.
type SentinelConfig struct {
	Enabled             bool   `yaml:"enabled,omitempty"`
	APIKeyEncrypted     string `yaml:"api_key_encrypted,omitempty"`
	Model               string `yaml:"model,omitempty"`
	BackendType         string `yaml:"backend_type,omitempty"`
	GCSBucket           string `yaml:"gcs_bucket,omitempty"`
	S3Bucket            string `yaml:"s3_bucket,omitempty"`
	S3Region            string `yaml:"s3_region,omitempty"`
	S3RoleARN           string `yaml:"s3_role_arn,omitempty"`
	AzureContainer      string `yaml:"azure_container,omitempty"`
	AzureStorageAccount string `yaml:"azure_storage_account,omitempty"`
	AzureTenant         string `yaml:"azure_tenant,omitempty"`
	AzureClient         string `yaml:"azure_client,omitempty"`
}

// ResolvedIntegrations contains effective integration values (decrypted where applicable).
type ResolvedIntegrations struct {
	GeminiAPIKey string
	GeminiModel  string
}

// MaskSecret returns a masked representation of a secret for API responses.
func MaskSecret(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "••••"
	}
	return "••••" + value[len(value)-4:]
}

// NormalizeGeminiModel returns the configured model or the default.
func NormalizeGeminiModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return DefaultGeminiModel
	}
	return model
}

// NormalizeSentinelModel returns the configured Sentinel model or the default.
func NormalizeSentinelModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return DefaultSentinelModel
	}
	return model
}

// ResolveIntegrations merges YAML config with environment variable overrides.
// Environment variables take precedence when set (backward compatibility).
func ResolveIntegrations(cfg Config, encKey [32]byte, hasKey bool) (ResolvedIntegrations, error) {
	ic := cfg.Integrations
	out := ResolvedIntegrations{
		GeminiModel: NormalizeGeminiModel(ic.GeminiModel),
	}

	if v := os.Getenv("GEMINI_API_KEY"); v != "" {
		out.GeminiAPIKey = v
	} else if ic.GeminiAPIKeyEncrypted != "" {
		if !hasKey {
			return out, fmt.Errorf("config: cannot decrypt integrations: no encryption key")
		}
		key, err := DecryptCredential(encKey, ic.GeminiAPIKeyEncrypted)
		if err != nil {
			return out, err
		}
		out.GeminiAPIKey = key
	}

	return out, nil
}

// GeminiConfigured reports whether a Gemini API key is available.
func GeminiConfigured(cfg Config, encKey [32]byte, hasKey bool) bool {
	resolved, err := ResolveIntegrations(cfg, encKey, hasKey)
	return err == nil && resolved.GeminiAPIKey != ""
}

// SetGeminiAPIKey encrypts and stores the Gemini API key in integrations config.
func SetGeminiAPIKey(ic *IntegrationsConfig, encKey [32]byte, plaintext string) error {
	if plaintext == "" {
		return fmt.Errorf("config: gemini api key cannot be empty")
	}
	enc, err := EncryptCredential(encKey, plaintext)
	if err != nil {
		return err
	}
	ic.GeminiAPIKeyEncrypted = enc
	return nil
}

// SetSentinelAPIKey encrypts and stores the Sentinel API key.
func SetSentinelAPIKey(sc *SentinelConfig, encKey [32]byte, plaintext string) error {
	if plaintext == "" {
		return fmt.Errorf("config: sentinel api key cannot be empty")
	}
	enc, err := EncryptCredential(encKey, plaintext)
	if err != nil {
		return err
	}
	sc.APIKeyEncrypted = enc
	return nil
}

// ImportIntegrationsFromEnv copies environment variables into integrations config (encrypted).
func ImportIntegrationsFromEnv(cfg *Config, encKey [32]byte) error {
	if cfg == nil {
		return fmt.Errorf("config: nil config")
	}
	ic := &cfg.Integrations
	if v := os.Getenv("GEMINI_API_KEY"); v != "" {
		if err := SetGeminiAPIKey(ic, encKey, v); err != nil {
			return err
		}
	}
	if ic.GeminiModel == "" {
		ic.GeminiModel = DefaultGeminiModel
	}
	return nil
}

// ApplyIntegrationsEnv exports resolved integration values to the process environment
// when the corresponding variable is not already set.
func ApplyIntegrationsEnv(cfg Config, encKey [32]byte, hasKey bool) error {
	resolved, err := ResolveIntegrations(cfg, encKey, hasKey)
	if err != nil {
		return err
	}
	setEnvIfEmpty("GEMINI_API_KEY", resolved.GeminiAPIKey)
	return nil
}

func setEnvIfEmpty(key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if strings.TrimSpace(os.Getenv(key)) != "" {
		return
	}
	_ = os.Setenv(key, value)
}
