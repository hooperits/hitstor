// hitstor-notifier-log is a reference notifier plugin that logs events to stderr.
// It serves as both a test fixture and an example for plugin authors.
//
// Usage:
//
//	hitstor config set alert_notifier_plugins log
//	hitstor config set plugin_dir /path/to/bin
package main

import (
	"context"
	"log/slog"
	"os"
	"sync"

	"github.com/hooperits/hitstor/internal/notifier"
	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
)

type logNotifier struct {
	mu     sync.Mutex
	logger *slog.Logger
	level  slog.Level
}

var _ notifier.Notifier = (*logNotifier)(nil)

func newLogNotifier() *logNotifier {
	return &logNotifier{
		logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
		level:  slog.LevelInfo,
	}
}

func (n *logNotifier) Configure(settings map[string]string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if lvl, ok := settings["level"]; ok {
		switch lvl {
		case "debug":
			n.level = slog.LevelDebug
		case "warn":
			n.level = slog.LevelWarn
		case "error":
			n.level = slog.LevelError
		default:
			n.level = slog.LevelInfo
		}
		n.logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: n.level}))
	}
	return nil
}

func (n *logNotifier) Notify(_ context.Context, event notifier.Event) error {
	n.mu.Lock()
	logger := n.logger
	n.mu.Unlock()

	logger.Info("notification",
		"event", event.EventType,
		"agent", event.Agent,
		"hostname", event.Hostname,
		"message", event.Message,
		"timestamp", event.Timestamp,
	)
	return nil
}

func (n *logNotifier) Close() error { return nil }

func main() {
	hitstorplugin.ServeNotifier(newLogNotifier())
}
