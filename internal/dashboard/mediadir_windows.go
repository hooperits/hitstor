//go:build windows

package dashboard

import (
	"os"
	"path/filepath"
)

// defaultMediaCacheDir is where recovery images live when nothing overrides it.
func defaultMediaCacheDir() string {
	if programData := os.Getenv("ProgramData"); programData != "" {
		return filepath.Join(programData, "HitStor", "bmr-media")
	}
	if cacheDir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(cacheDir, "hitstor", "bmr-media")
	}
	return filepath.Join(os.TempDir(), "hitstor-bmr-media")
}
