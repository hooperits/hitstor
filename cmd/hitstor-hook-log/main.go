// hitstor-hook-log is a reference hook plugin that logs pre/post-backup
// events to stderr. It serves as both a test fixture and an example for
// plugin authors.
//
// Usage:
//
//	hitstor config add hook_plugins log
//	hitstor config set plugin_dir /path/to/bin
package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/hooperits/hitstor/internal/hook"
	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
)

type logHook struct {
	mu     sync.Mutex
	logger *slog.Logger
	level  slog.Level
}

var _ hook.Hook = (*logHook)(nil)

func newLogHook() *logHook {
	return &logHook{
		logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
		level:  slog.LevelInfo,
	}
}

func (h *logHook) Configure(settings map[string]string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if lvl, ok := settings["level"]; ok {
		switch lvl {
		case "debug":
			h.level = slog.LevelDebug
		case "warn":
			h.level = slog.LevelWarn
		case "error":
			h.level = slog.LevelError
		default:
			h.level = slog.LevelInfo
		}
		h.logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: h.level}))
	}
	return nil
}

func (h *logHook) PreBackup(_ context.Context, event hook.HookEvent) error {
	h.mu.Lock()
	logger := h.logger
	h.mu.Unlock()

	logger.Info("pre_backup",
		"agent", event.Agent,
		"hostname", event.Hostname,
		"paths", strings.Join(event.Paths, ", "),
	)
	return nil
}

func (h *logHook) PostBackup(_ context.Context, event hook.HookEvent) error {
	h.mu.Lock()
	logger := h.logger
	h.mu.Unlock()

	logger.Info("post_backup",
		"agent", event.Agent,
		"hostname", event.Hostname,
		"snapshot_id", event.SnapshotID,
		"paths", strings.Join(event.Paths, ", "),
	)
	return nil
}

func (h *logHook) Close() error { return nil }

func main() {
	hitstorplugin.ServeHook(newLogHook())
}
