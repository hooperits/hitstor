package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/hooperits/hitstor/internal/notifier"
)

// Webhook delivers notifications via HTTP POST with a JSON payload.
type Webhook struct {
	url    string
	client *http.Client
}

var _ notifier.Notifier = (*Webhook)(nil)

// New creates a webhook notifier that POSTs events to the given URL.
func New(url string) *Webhook {
	return &Webhook{
		url:    url,
		client: &http.Client{Timeout: 10 * 1e9}, // 10 seconds
	}
}

func (w *Webhook) Notify(_ context.Context, event notifier.Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("webhook: marshal event: %w", err)
	}

	req, err := http.NewRequest("POST", w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "hitstor-alerter/1.0")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: POST failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		slog.Warn("webhook: non-2xx response", "url", w.url, "status", resp.StatusCode)
	}
	return nil
}

func (w *Webhook) Close() error { return nil }
