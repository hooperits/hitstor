package alerting

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/notifier"
)

// mockNotifier records events for test assertions.
type mockNotifier struct {
	mu     sync.Mutex
	events []notifier.Event
}

func (m *mockNotifier) Notify(_ context.Context, event notifier.Event) error {
	m.mu.Lock()
	m.events = append(m.events, event)
	m.mu.Unlock()
	return nil
}

func (m *mockNotifier) Close() error { return nil }

func (m *mockNotifier) getEvents() []notifier.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]notifier.Event, len(m.events))
	copy(cp, m.events)
	return cp
}

func TestSendNotification(t *testing.T) {
	mock := &mockNotifier{}
	a := New(Options{Notifiers: []notifier.Notifier{mock}, StaleThreshold: 25 * time.Hour})
	a.OnBackupFailed("agent-1", "host-1", "agent disconnected", map[string]any{
		"session_id":      "sess-abc",
		"chunks_received": 42,
	})

	events := mock.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	evt := events[0]
	if evt.EventType != notifier.EventBackupFailed {
		t.Errorf("event type = %q, want %q", evt.EventType, notifier.EventBackupFailed)
	}
	if evt.Agent != "agent-1" {
		t.Errorf("agent = %q, want agent-1", evt.Agent)
	}
	if evt.Hostname != "host-1" {
		t.Errorf("hostname = %q, want host-1", evt.Hostname)
	}
	if evt.Message != "backup failed: agent disconnected" {
		t.Errorf("message = %q", evt.Message)
	}
}

func TestNotificationDisabled(t *testing.T) {
	a := New(Options{Notifiers: nil})
	if a.Enabled() {
		t.Error("alerter should not be enabled with no notifiers")
	}
	// Should not panic.
	a.OnBackupFailed("agent-1", "host-1", "test", nil)
}

func TestStaleDetection(t *testing.T) {
	mock := &mockNotifier{}
	a := New(Options{Notifiers: []notifier.Notifier{mock}, StaleThreshold: 25 * time.Hour})

	staleTime := time.Now().Add(-48 * time.Hour)
	agents := []AgentInfo{
		{Name: "agent-stale", LastBackup: &staleTime},
	}
	a.checkStale(agents)

	events := mock.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	evt := events[0]
	if evt.EventType != notifier.EventAgentStale {
		t.Errorf("event type = %q, want %q", evt.EventType, notifier.EventAgentStale)
	}
	if evt.Agent != "agent-stale" {
		t.Errorf("agent = %q, want agent-stale", evt.Agent)
	}
}

func TestStaleCooldown(t *testing.T) {
	mock := &mockNotifier{}
	a := New(Options{Notifiers: []notifier.Notifier{mock}, StaleThreshold: 25 * time.Hour})

	staleTime := time.Now().Add(-48 * time.Hour)
	agents := []AgentInfo{
		{Name: "agent-stale", LastBackup: &staleTime},
	}

	a.checkStale(agents)
	a.checkStale(agents) // Second call — should be suppressed by cooldown.

	events := mock.getEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event (cooldown), got %d", len(events))
	}
}

func TestStaleCooldownExpiry(t *testing.T) {
	mock := &mockNotifier{}
	// Use a very short threshold so cooldown expires quickly.
	a := New(Options{Notifiers: []notifier.Notifier{mock}, StaleThreshold: 50 * time.Millisecond})

	staleTime := time.Now().Add(-1 * time.Hour)
	agents := []AgentInfo{
		{Name: "agent-stale", LastBackup: &staleTime},
	}

	a.checkStale(agents)
	time.Sleep(60 * time.Millisecond) // Wait for cooldown to expire.
	a.checkStale(agents)

	events := mock.getEvents()
	if len(events) != 2 {
		t.Errorf("expected 2 events (after cooldown expiry), got %d", len(events))
	}
}

func TestStaleRecoveryResetsAlert(t *testing.T) {
	mock := &mockNotifier{}
	a := New(Options{Notifiers: []notifier.Notifier{mock}, StaleThreshold: 25 * time.Hour})

	staleTime := time.Now().Add(-48 * time.Hour)
	agents := []AgentInfo{
		{Name: "agent-1", LastBackup: &staleTime},
	}

	// First check: agent is stale → alert fires.
	a.checkStale(agents)
	events := mock.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// Agent recovers (recent backup).
	recentTime := time.Now()
	agents[0].LastBackup = &recentTime
	a.checkStale(agents) // Clears cooldown.

	// Agent goes stale again.
	agents[0].LastBackup = &staleTime
	a.checkStale(agents)

	events = mock.getEvents()
	if len(events) != 2 {
		t.Errorf("expected 2 events (after recovery), got %d", len(events))
	}
}

func TestStaleNeverBackedUpSkipped(t *testing.T) {
	mock := &mockNotifier{}
	a := New(Options{Notifiers: []notifier.Notifier{mock}, StaleThreshold: 25 * time.Hour})

	agents := []AgentInfo{
		{Name: "agent-new", LastBackup: nil},
	}
	a.checkStale(agents)

	events := mock.getEvents()
	if len(events) != 0 {
		t.Errorf("expected 0 events for never-backed-up agent, got %d", len(events))
	}
}

func TestNotificationPayloadFormat(t *testing.T) {
	mock := &mockNotifier{}
	a := New(Options{Notifiers: []notifier.Notifier{mock}, StaleThreshold: 25 * time.Hour})
	a.OnBackupFailed("test-agent", "test-host", "connection lost", map[string]any{
		"session_id": "s123",
	})

	events := mock.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	evt := events[0]

	if evt.Server != "hitstor" {
		t.Errorf("server = %q, want hitstor", evt.Server)
	}
	// Verify timestamp is valid RFC3339.
	if _, err := time.Parse(time.RFC3339, evt.Timestamp); err != nil {
		t.Errorf("timestamp %q is not valid RFC3339: %v", evt.Timestamp, err)
	}
	if evt.Details == nil {
		t.Error("details should not be nil")
	}
	if evt.Details["session_id"] != "s123" {
		t.Errorf("details.session_id = %v, want s123", evt.Details["session_id"])
	}
}

func TestMultipleNotifiers(t *testing.T) {
	mock1 := &mockNotifier{}
	mock2 := &mockNotifier{}
	a := New(Options{Notifiers: []notifier.Notifier{mock1, mock2}, StaleThreshold: 25 * time.Hour})

	a.OnBackupFailed("agent-1", "host-1", "test error", nil)

	events1 := mock1.getEvents()
	events2 := mock2.getEvents()
	if len(events1) != 1 {
		t.Errorf("notifier 1: expected 1 event, got %d", len(events1))
	}
	if len(events2) != 1 {
		t.Errorf("notifier 2: expected 1 event, got %d", len(events2))
	}
}

// failingNotifier returns an error on every Notify call.
type failingNotifier struct{}

func (f *failingNotifier) Notify(_ context.Context, _ notifier.Event) error {
	return errors.New("notify failed")
}
func (f *failingNotifier) Close() error { return nil }

func TestDispatchWithFailingNotifier(t *testing.T) {
	mock := &mockNotifier{}
	failing := &failingNotifier{}
	// Failing notifier first, working notifier second — both should be called.
	a := New(Options{Notifiers: []notifier.Notifier{failing, mock}, StaleThreshold: 25 * time.Hour})

	a.OnBackupFailed("agent-1", "host-1", "test", nil)

	events := mock.getEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event on working notifier despite failure, got %d", len(events))
	}
}

func TestStartStaleCheckerFires(t *testing.T) {
	mock := &mockNotifier{}
	a := New(Options{
		Notifiers:      []notifier.Notifier{mock},
		StaleThreshold: 10 * time.Millisecond,
		CheckInterval:  20 * time.Millisecond,
	})

	staleTime := time.Now().Add(-1 * time.Hour)
	agentsFn := func() []AgentInfo {
		return []AgentInfo{{Name: "stale-agent", LastBackup: &staleTime}}
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.StartStaleChecker(ctx, agentsFn)

	// Wait enough for the immediate run + at least one tick.
	time.Sleep(100 * time.Millisecond)
	cancel()

	events := mock.getEvents()
	if len(events) < 1 {
		t.Errorf("expected at least 1 stale alert, got %d", len(events))
	}
}

func TestStartStaleCheckerDisabled(t *testing.T) {
	// No notifiers — should not panic, just return immediately.
	a := New(Options{Notifiers: nil})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.StartStaleChecker(ctx, func() []AgentInfo { return nil })
	// No assertion needed — just verify it doesn't panic.
}
