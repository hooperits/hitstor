//go:build !windows

package agent

// defaultExcludes is empty on non-Windows platforms.
var defaultExcludes []string
