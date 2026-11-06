package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFileExportsSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.env")
	os.WriteFile(path, []byte("HITSTOR_AGENT_LISTEN_PORT=9422\r\nHITSTOR_TEST_KEEP=from-file\n"), 0600)
	t.Setenv("HITSTOR_TEST_KEEP", "from-env")
	os.Unsetenv("HITSTOR_AGENT_LISTEN_PORT")
	t.Cleanup(func() { os.Unsetenv("HITSTOR_AGENT_LISTEN_PORT") })

	loadEnvFile(path)

	if got := os.Getenv("HITSTOR_AGENT_LISTEN_PORT"); got != "9422" {
		t.Fatalf("HITSTOR_AGENT_LISTEN_PORT = %q, want 9422", got)
	}
	if got := os.Getenv("HITSTOR_TEST_KEEP"); got != "from-env" {
		t.Fatalf("existing variable overridden: %q", got)
	}
}
