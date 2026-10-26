package notifier

import "context"

// Event types.
const (
	EventBackupFailed   = "backup_failed"
	EventAgentStale     = "agent_stale"
	EventThreatDetected = "threat_detected"
)

// Event represents a notification payload for backup system events.
type Event struct {
	EventType string         `json:"event"`
	Server    string         `json:"server"`
	Timestamp string         `json:"timestamp"`
	Agent     string         `json:"agent"`
	Hostname  string         `json:"hostname,omitempty"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
}

// Notifier delivers backup event notifications.
// Implementations handle a single delivery channel (webhook, email, Slack, etc.).
// The caller handles orchestration (cooldowns, stale detection, event creation).
type Notifier interface {
	// Notify sends an event notification. Implementations should be safe for
	// concurrent use and return quickly (use reasonable timeouts).
	Notify(ctx context.Context, event Event) error

	// Close releases any resources held by the notifier.
	Close() error
}
