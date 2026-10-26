//go:build !windows

package agent

// Platform-specific default paths for the agent on Unix systems.
var (
	DefaultTokenPath = "/etc/hitstor/agent-token"
	DefaultEnvPath   = "/etc/hitstor/agent.env"
	DefaultConfigDir = "/etc/hitstor"
	// DefaultServerPinPath stores the pinned server certificate fingerprint.
	DefaultServerPinPath = "/etc/hitstor/server-fingerprint"
)
