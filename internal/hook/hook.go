// Package hook defines the Hook interface for pre/post-backup lifecycle plugins.
package hook

import "context"

// HookEvent carries context about a backup operation to hook plugins.
type HookEvent struct {
	Agent      string            `json:"agent"`
	Hostname   string            `json:"hostname,omitempty"`
	Paths      []string          `json:"paths"`
	Excludes   []string          `json:"excludes,omitempty"`
	Tags       []string          `json:"tags,omitempty"`
	SnapshotID string            `json:"snapshot_id,omitempty"` // Set in PostBackup only.
	Details    map[string]string `json:"details,omitempty"`
}

// Hook is implemented by plugins that run logic before or after backups.
//
// PreBackup is called before a backup starts. Returning a non-nil error
// aborts the backup.
//
// PostBackup is called after a backup completes successfully. Errors are
// logged but do not affect the backup result.
type Hook interface {
	PreBackup(ctx context.Context, event HookEvent) error
	PostBackup(ctx context.Context, event HookEvent) error
	Close() error
}
