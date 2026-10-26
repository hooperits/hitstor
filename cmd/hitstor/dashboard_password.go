package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"

	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/crypto"
)

// ensureDashboardPassword returns the dashboard password from cfg. When none
// is configured it generates a random one so the dashboard is never served
// without authentication: the scrypt hash is stored in cfg (and persisted when
// a config file exists) and the plaintext is logged once for the operator,
// who can change it from Settings.
func ensureDashboardPassword(cfg *config.Config) (string, error) {
	if cfg.ServeDashboardPassword != "" {
		return cfg.ServeDashboardPassword, nil
	}

	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate dashboard password: %w", err)
	}
	plain := base64.RawURLEncoding.EncodeToString(buf)
	hash, err := crypto.HashPassword(plain)
	if err != nil {
		return "", fmt.Errorf("hash dashboard password: %w", err)
	}
	cfg.ServeDashboardPassword = hash

	persisted := false
	if cfg.Exists {
		if err := config.Save(*cfg); err != nil {
			slog.Warn("could not save generated dashboard password; a new one will be generated on restart", "error", err)
		} else {
			persisted = true
		}
	}
	slog.Warn("no dashboard password configured; generated one — log in with it and change it under Settings",
		"password", plain, "saved_to_config", persisted)
	return hash, nil
}
