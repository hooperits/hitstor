//go:build !windows

package agent

// SecureConfigDir is a no-op outside Windows: files there are created with
// 0600 permissions, which already keep them private.
func SecureConfigDir() error { return nil }
