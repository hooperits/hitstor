package config

import (
	"os"
	"testing"
)

func TestMaskSecret(t *testing.T) {
	if got := MaskSecret(""); got != "" {
		t.Fatalf("empty = %q", got)
	}
	if got := MaskSecret("abc"); got != "••••" {
		t.Fatalf("short = %q", got)
	}
	if got := MaskSecret("sk-test-key-1234"); got != "••••1234" {
		t.Fatalf("long = %q", got)
	}
}

func TestResolveIntegrationsEnvOverride(t *testing.T) {
	os.Setenv("GEMINI_API_KEY", "env-key")
	defer os.Unsetenv("GEMINI_API_KEY")

	cfg := Config{
		Integrations: IntegrationsConfig{
			GeminiModel: "gemini-2.0",
		},
	}

	resolved, err := ResolveIntegrations(cfg, [32]byte{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.GeminiAPIKey != "env-key" {
		t.Fatalf("gemini key = %q", resolved.GeminiAPIKey)
	}
	if resolved.GeminiModel != "gemini-2.0" {
		t.Fatalf("model = %q", resolved.GeminiModel)
	}
}

func TestSetAndResolveGeminiAPIKey(t *testing.T) {
	var encKey [32]byte
	copy(encKey[:], []byte("0123456789abcdef0123456789abcdef"))

	ic := IntegrationsConfig{}
	if err := SetGeminiAPIKey(&ic, encKey, "secret-gemini-key"); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Integrations: ic}
	resolved, err := ResolveIntegrations(cfg, encKey, true)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.GeminiAPIKey != "secret-gemini-key" {
		t.Fatalf("resolved = %q", resolved.GeminiAPIKey)
	}
}

func TestImportIntegrationsFromEnv(t *testing.T) {
	var encKey [32]byte
	copy(encKey[:], []byte("0123456789abcdef0123456789abcdef"))

	os.Setenv("GEMINI_API_KEY", "import-key")
	defer os.Unsetenv("GEMINI_API_KEY")

	cfg := &Config{}
	if err := ImportIntegrationsFromEnv(cfg, encKey); err != nil {
		t.Fatal(err)
	}
	if cfg.Integrations.GeminiAPIKeyEncrypted == "" {
		t.Fatal("expected encrypted gemini key")
	}
	resolved, err := ResolveIntegrations(*cfg, encKey, true)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.GeminiAPIKey != "import-key" {
		t.Fatalf("gemini = %q", resolved.GeminiAPIKey)
	}
}
