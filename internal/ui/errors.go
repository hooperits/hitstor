package ui

import (
	"strings"
)

// ErrorHint holds a user-friendly error message and corrective hint.
type ErrorHint struct {
	Message string
	Hint    string
}

// FormatError pattern-matches known error strings and returns a user-friendly
// message with a contextual hint. If no pattern matches, it returns the
// original error message with no hint.
func FormatError(err error) ErrorHint {
	msg := err.Error()

	// Missing repository (file not found at repo path).
	if strings.Contains(msg, "no such file or directory") {
		path := extractRepoPath(msg)
		if path != "" {
			return ErrorHint{
				Message: "Repository not found at " + path,
				Hint:    "Run 'hitstor init --repo " + path + "' to create a new repository",
			}
		}
	}

	// Password required (no password provided).
	if strings.Contains(msg, "password required") {
		return ErrorHint{
			Message: "Password required",
			Hint:    "Use --password, --password-file, or set HITSTOR_PASSWORD",
		}
	}

	// Wrong password (decryption failure).
	if strings.Contains(msg, "wrong password") {
		return ErrorHint{
			Message: "Incorrect password",
			Hint:    "Check your password or password file",
		}
	}

	// Snapshot not found.
	if strings.Contains(msg, "not found") && strings.Contains(msg, "snapshot") {
		return ErrorHint{
			Message: "Snapshot not found",
			Hint:    "Run 'hitstor snapshots' to list available snapshots",
		}
	}

	// Missing --repo flag.
	if strings.Contains(msg, "--repo flag is required") {
		return ErrorHint{
			Message: "Repository location not specified",
			Hint:    "Use --repo <path>, set HITSTOR_REPO, or run 'hitstor config set repo <path>'",
		}
	}

	// Invalid YAML in config file.
	if strings.Contains(msg, "invalid YAML") {
		return ErrorHint{
			Message: msg,
			Hint:    "Run 'hitstor config edit' to fix the config file",
		}
	}

	// Unknown config key.
	if strings.Contains(msg, "unknown config key") {
		return ErrorHint{
			Message: msg,
			Hint:    "Valid keys: repo, password_file, s3_access_key, s3_secret_key, s3_region, exclude, parallel, serve_port, serve_tls_cert, serve_tls_key, agent_server, agent_api_key, agent_ca_cert, agent_hostname, agent_paths, agent_excludes",
		}
	}

	// Server connection refused.
	if strings.Contains(msg, "connection refused") {
		return ErrorHint{
			Message: "Connection refused",
			Hint:    "Check that the server is running and the address/port are correct",
		}
	}

	// TLS handshake errors.
	if strings.Contains(msg, "TLS handshake") || strings.Contains(msg, "tls:") || strings.Contains(msg, "certificate") {
		return ErrorHint{
			Message: "TLS connection error",
			Hint:    "Check that the server certificate is trusted, or use the server's self-signed cert",
		}
	}

	// Agent authentication failure.
	if strings.Contains(msg, "Unauthenticated") || strings.Contains(msg, "invalid API key") {
		return ErrorHint{
			Message: "Agent authentication failed",
			Hint:    "Check your API key or register the agent with 'hitstor serve agent add <name>'",
		}
	}

	// Agent: server address required.
	if strings.Contains(msg, "server address required") {
		return ErrorHint{
			Message: "Server address not specified",
			Hint:    "Use --server <host:port>, set HITSTOR_AGENT_SERVER, or run 'hitstor-agent config set agent_server <host:port>'",
		}
	}

	// Agent: API key required.
	if strings.Contains(msg, "API key required") {
		return ErrorHint{
			Message: "API key not specified",
			Hint:    "Use --api-key <key>, set HITSTOR_AGENT_API_KEY, or run 'hitstor-agent config set agent_api_key <key>'",
		}
	}

	// Agent: upload failure.
	if strings.Contains(msg, "upload chunk") || strings.Contains(msg, "upload stream") {
		return ErrorHint{
			Message: "Chunk upload failed",
			Hint:    "Check network connectivity to the server. The backup can be retried safely.",
		}
	}

	// Config init requires TTY.
	if strings.Contains(msg, "config init requires an interactive terminal") {
		return ErrorHint{
			Message: "Interactive terminal required for config init",
			Hint:    "Use 'hitstor config set <key> <value>' for non-interactive configuration",
		}
	}

	// Default: return original error, no hint.
	return ErrorHint{
		Message: msg,
		Hint:    "",
	}
}

// extractRepoPath attempts to extract a file path from an error message
// containing "no such file or directory". It looks for the path before "/config"
// since repository open errors reference the config file.
func extractRepoPath(msg string) string {
	// Errors typically look like: "open /path/to/repo/config: no such file or directory"
	idx := strings.Index(msg, "/config:")
	if idx < 0 {
		idx = strings.Index(msg, "/config :")
	}
	if idx < 0 {
		return ""
	}

	// Walk backwards from idx to find the start of the path.
	start := idx
	for start > 0 && msg[start-1] != ' ' {
		start--
	}

	path := msg[start:idx]
	if path == "" {
		return ""
	}
	return path
}
