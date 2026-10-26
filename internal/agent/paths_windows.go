//go:build windows

package agent

import (
	"os"
	"path/filepath"
)

// Platform-specific default paths for the agent on Windows.
// Config lives under %ProgramData%\HitStor (typically C:\ProgramData\HitStor).
var (
	DefaultTokenPath = filepath.Join(programDataDir(), "agent-token")
	DefaultEnvPath   = filepath.Join(programDataDir(), "agent.env")
	DefaultConfigDir = programDataDir()
	// DefaultServerPinPath stores the pinned server certificate fingerprint.
	DefaultServerPinPath = filepath.Join(programDataDir(), "server-fingerprint")
)

func programDataDir() string {
	if pd := os.Getenv("ProgramData"); pd != "" {
		return filepath.Join(pd, "HitStor")
	}
	return `C:\ProgramData\HitStor`
}
