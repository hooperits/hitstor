package logstore

import "strings"

// NormalizeMessage returns a stable lowercase message for dedup and noise checks.
func NormalizeMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if e, ok := ParseSlogTextLine(msg); ok && e.Message != "" {
		msg = e.Message
	}
	return strings.ToLower(msg)
}

// IsNoise reports whether an entry should be hidden from View Logs aggregation.
func IsNoise(e Entry) bool {
	msg := NormalizeMessage(e.Message)
	if msg == "" {
		return false
	}
	if strings.Contains(msg, "tls handshake error") {
		if strings.Contains(msg, "unknown certificate") || strings.Contains(msg, " eof") || strings.HasSuffix(msg, ": eof") {
			return true
		}
	}
	if strings.Contains(msg, "remote error: tls: unknown certificate") {
		return true
	}
	return false
}
