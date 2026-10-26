package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/notifier"
)

func TestWebhookDelivery(t *testing.T) {
	var mu sync.Mutex
	var received []notifier.Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if ua := r.Header.Get("User-Agent"); ua != "hitstor-alerter/1.0" {
			t.Errorf("User-Agent = %q, want hitstor-alerter/1.0", ua)
		}
		var evt notifier.Event
		if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
			t.Errorf("decode event: %v", err)
			return
		}
		mu.Lock()
		received = append(received, evt)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := New(srv.URL)
	evt := notifier.Event{
		EventType: notifier.EventBackupFailed,
		Server:    "hitstor",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Agent:     "agent-1",
		Hostname:  "host-1",
		Message:   "backup failed: connection lost",
		Details:   map[string]any{"session_id": "s123"},
	}

	if err := wh.Notify(context.Background(), evt); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	got := received[0]
	if got.EventType != notifier.EventBackupFailed {
		t.Errorf("event type = %q, want %q", got.EventType, notifier.EventBackupFailed)
	}
	if got.Agent != "agent-1" {
		t.Errorf("agent = %q, want agent-1", got.Agent)
	}
}

func TestWebhookNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	wh := New(srv.URL)
	evt := notifier.Event{
		EventType: notifier.EventBackupFailed,
		Server:    "hitstor",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Agent:     "agent-1",
		Message:   "test",
	}

	// Non-2xx should not return an error (fire-and-forget with warning log).
	if err := wh.Notify(context.Background(), evt); err != nil {
		t.Errorf("Notify should not error on non-2xx, got: %v", err)
	}
}

func TestWebhookConnectionError(t *testing.T) {
	wh := New("http://127.0.0.1:1") // Nothing listening.
	evt := notifier.Event{
		EventType: notifier.EventBackupFailed,
		Server:    "hitstor",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Agent:     "agent-1",
		Message:   "test",
	}

	err := wh.Notify(context.Background(), evt)
	if err == nil {
		t.Error("expected error for connection failure")
	}
}

func TestWebhookClose(t *testing.T) {
	wh := New("http://example.com")
	if err := wh.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}
