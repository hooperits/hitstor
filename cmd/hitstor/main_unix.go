//go:build !windows

package main

// runAsService returns false on non-Windows platforms.
func runAsService() bool {
	return false
}
