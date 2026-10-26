package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/notifier"
)

// AgentInfo holds the minimal agent data needed for stale detection.
type AgentInfo struct {
	Name       string
	LastBackup *time.Time
}

// Options configures the alerter.
type Options struct {
	Notifiers      []notifier.Notifier
	StaleThreshold time.Duration
	CheckInterval  time.Duration // default 5m if zero
}

// Alerter dispatches backup event notifications to registered notifiers.
type Alerter struct {
	notifiers      []notifier.Notifier
	staleThreshold time.Duration
	checkInterval  time.Duration

	mu             sync.Mutex
	lastStaleAlert map[string]time.Time // agent name -> last alert time
}

// New creates an Alerter.
func New(opts Options) *Alerter {
	threshold := opts.StaleThreshold
	if threshold <= 0 {
		threshold = 25 * time.Hour
	}
	interval := opts.CheckInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &Alerter{
		notifiers:      opts.Notifiers,
		staleThreshold: threshold,
		checkInterval:  interval,
		lastStaleAlert: make(map[string]time.Time),
	}
}

// Enabled returns true if at least one notifier is configured.
func (a *Alerter) Enabled() bool {
	return len(a.notifiers) > 0
}

// OnBackupFailed sends a backup_failed notification.
// Safe to call even if the alerter is disabled (no-op).
func (a *Alerter) OnBackupFailed(agentName, hostname, reason string, details map[string]any) {
	if !a.Enabled() {
		return
	}
	evt := notifier.Event{
		EventType: notifier.EventBackupFailed,
		Server:    "hitstor",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Agent:     agentName,
		Hostname:  hostname,
		Message:   fmt.Sprintf("backup failed: %s", reason),
		Details:   details,
	}
	a.dispatch(evt)
}

// OnThreatDetected sends a threat_detected notification when ransomware is suspected.
func (a *Alerter) OnThreatDetected(agentName, hostname, reason string, confidence float64, details map[string]any) {
	if !a.Enabled() {
		return
	}
	evt := notifier.Event{
		EventType: notifier.EventThreatDetected,
		Server:    "hitstor",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Agent:     agentName,
		Hostname:  hostname,
		Message:   fmt.Sprintf("Ransomware threat detected (confidence %.1f%%): %s", confidence*100, reason),
		Details:   details,
	}
	a.dispatch(evt)
}

// StartStaleChecker runs a background goroutine that periodically checks for
// stale agents and sends alerts. Cancel the context to stop.
func (a *Alerter) StartStaleChecker(ctx context.Context, agentsFn func() []AgentInfo) {
	if !a.Enabled() {
		slog.Info("alerting disabled (no notifiers configured)")
		return
	}
	slog.Info("alerting stale checker started",
		"notifiers", len(a.notifiers),
		"threshold", a.staleThreshold,
		"interval", a.checkInterval,
	)
	go a.staleLoop(ctx, agentsFn)
}

func (a *Alerter) staleLoop(ctx context.Context, agentsFn func() []AgentInfo) {
	ticker := time.NewTicker(a.checkInterval)
	defer ticker.Stop()

	// Run once immediately on startup.
	a.checkStale(agentsFn())

	for {
		select {
		case <-ctx.Done():
			slog.Info("alerting stale checker stopped")
			return
		case <-ticker.C:
			a.checkStale(agentsFn())
		}
	}
}

func (a *Alerter) checkStale(agents []AgentInfo) {
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, agent := range agents {
		// Skip agents that never backed up to avoid spam for newly registered agents.
		if agent.LastBackup == nil {
			continue
		}

		age := now.Sub(*agent.LastBackup)
		if age <= a.staleThreshold {
			// Agent is OK — clear cooldown so we re-alert if it goes stale again.
			delete(a.lastStaleAlert, agent.Name)
			continue
		}

		// Agent is stale. Check cooldown to avoid repeated alerts.
		if lastAlert, ok := a.lastStaleAlert[agent.Name]; ok {
			if now.Sub(lastAlert) < a.staleThreshold {
				continue
			}
		}

		// Send stale alert and record cooldown.
		a.lastStaleAlert[agent.Name] = now
		evt := notifier.Event{
			EventType: notifier.EventAgentStale,
			Server:    "hitstor",
			Timestamp: now.UTC().Format(time.RFC3339),
			Agent:     agent.Name,
			Message: fmt.Sprintf("no backup in %s (threshold: %s)",
				formatDuration(age), formatDuration(a.staleThreshold)),
			Details: map[string]any{
				"last_backup":     agent.LastBackup.UTC().Format(time.RFC3339),
				"threshold_hours": int(a.staleThreshold.Hours()),
			},
		}
		// Release lock before dispatching to avoid holding it during I/O.
		a.mu.Unlock()
		a.dispatch(evt)
		a.mu.Lock()
	}
}

// dispatch sends an event to all registered notifiers (fire-and-forget).
func (a *Alerter) dispatch(evt notifier.Event) {
	for _, n := range a.notifiers {
		if err := n.Notify(context.Background(), evt); err != nil {
			slog.Warn("alerting: notifier failed", "error", err)
		}
	}
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	if h < 24 {
		return fmt.Sprintf("%dh", h)
	}
	days := h / 24
	rem := h % 24
	if rem == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd%dh", days, rem)
}
