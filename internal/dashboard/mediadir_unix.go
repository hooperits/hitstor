//go:build !windows

package dashboard

import (
	"os"
	"path/filepath"
)

// defaultMediaCacheDir is where recovery images live when nothing overrides it.
//
// /var/lib is the right home for large, regenerable server state, but a server
// running unprivileged cannot write there, so fall back to the user's cache
// rather than failing the first build with a permission error.
func defaultMediaCacheDir() string {
	const systemDir = "/var/lib/hitstor/bmr-media"
	if os.Geteuid() == 0 {
		return systemDir
	}
	if cacheDir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(cacheDir, "hitstor", "bmr-media")
	}
	return systemDir
}
