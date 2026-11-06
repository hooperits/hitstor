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

// defaultPluginPackageDir is where uploaded plugin packages are kept.
func defaultPluginPackageDir() string {
	if programData := os.Getenv("ProgramData"); programData != "" {
		return filepath.Join(programData, "HitStor", "plugin-packages")
	}
	return filepath.Join(os.TempDir(), "hitstor-plugin-packages")
}

// defaultTLSDir is where an uploaded dashboard certificate is stored.
func defaultTLSDir() string {
	if programData := os.Getenv("ProgramData"); programData != "" {
		return filepath.Join(programData, "HitStor", "tls")
	}
	return filepath.Join(os.TempDir(), "hitstor-tls")
}
