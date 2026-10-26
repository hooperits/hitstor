package config

import (
	"os"
	"strconv"

	"github.com/hooperits/hitstor/internal/logstore"
)

// LogStoreConfig builds logstore.Config from server config and env overrides.
func LogStoreConfig(cfg Config) logstore.Config {
	out := logstore.DefaultConfig()
	if cfg.LogMaxEntries > 0 {
		out.MaxEntries = cfg.LogMaxEntries
	}
	if cfg.LogFilePath != "" {
		out.FilePath = cfg.LogFilePath
	}
	if cfg.LogRotateMaxMB > 0 {
		out.RotateMaxMB = cfg.LogRotateMaxMB
	}
	if cfg.LogRotateMaxFiles > 0 {
		out.RotateMaxFiles = cfg.LogRotateMaxFiles
	}
	if cfg.LogFileEnabled != nil {
		out.FileEnabled = *cfg.LogFileEnabled
	}
	if v := os.Getenv("HITSTOR_LOG_MAX_ENTRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			out.MaxEntries = n
		}
	}
	if v := os.Getenv("HITSTOR_LOG_FILE_PATH"); v != "" {
		out.FilePath = v
	}
	if v := os.Getenv("HITSTOR_LOG_ROTATE_MAX_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			out.RotateMaxMB = n
		}
	}
	if v := os.Getenv("HITSTOR_LOG_ROTATE_MAX_FILES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			out.RotateMaxFiles = n
		}
	}
	if v := os.Getenv("HITSTOR_LOG_FILE_ENABLED"); v != "" {
		out.FileEnabled = v == "1" || v == "true" || v == "yes"
	}
	return out.Normalize()
}
