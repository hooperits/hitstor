package main

import (
	"path/filepath"
	"testing"

	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/crypto"
)

func TestEnsureDashboardPassword(t *testing.T) {
	tests := []struct {
		name        string
		configured  string
		exists      bool
		wantKept    bool
		wantPersist bool
	}{
		{name: "configured password is kept", configured: "already-set", exists: true, wantKept: true},
		{name: "generated and saved when config file exists", exists: true, wantPersist: true},
		{name: "generated in memory without config file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			cfg := config.Config{Path: path, ServeDashboardPassword: tt.configured, Exists: tt.exists}
			if tt.exists {
				if err := config.Save(cfg); err != nil {
					t.Fatal(err)
				}
			}

			got, err := ensureDashboardPassword(&cfg)
			if err != nil {
				t.Fatalf("ensureDashboardPassword() error = %v", err)
			}

			if tt.wantKept {
				if got != tt.configured {
					t.Errorf("password = %q, want configured %q", got, tt.configured)
				}
				return
			}
			if !crypto.IsHashedPassword(got) {
				t.Errorf("generated password %q is not stored as a hash", got)
			}
			if cfg.ServeDashboardPassword != got {
				t.Errorf("cfg.ServeDashboardPassword = %q, want %q", cfg.ServeDashboardPassword, got)
			}

			saved, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantPersist && saved.ServeDashboardPassword != got {
				t.Errorf("saved password = %q, want %q", saved.ServeDashboardPassword, got)
			}
			if !tt.wantPersist && saved.Exists {
				t.Errorf("config file was created at %s, want none", path)
			}
		})
	}
}
