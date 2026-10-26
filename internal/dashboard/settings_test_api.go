package dashboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/config"
)

type settingsTestResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type geminiTestRequest struct {
	GeminiAPIKey string `json:"gemini_api_key,omitempty"`
}

type webhookTestRequest struct {
	AlertWebhookURL string `json:"alert_webhook_url,omitempty"`
}

func (d *Dashboard) handleAPISettingsTestGemini(w http.ResponseWriter, r *http.Request) {
	var req geminiTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	apiKey := strings.TrimSpace(req.GeminiAPIKey)
	if apiKey == "" {
		cfg, encKey, hasKey, err := d.effectiveIntegrations()
		if err != nil {
			writeJSON(w, settingsTestResponse{OK: false, Error: err.Error()})
			return
		}
		resolved, err := config.ResolveIntegrations(*cfg, encKey, hasKey)
		if err != nil || resolved.GeminiAPIKey == "" {
			writeJSON(w, settingsTestResponse{OK: false, Error: "Gemini API key is not configured"})
			return
		}
		apiKey = resolved.GeminiAPIKey
	}

	if len(apiKey) < 8 {
		writeJSON(w, settingsTestResponse{OK: false, Error: "Gemini API key looks invalid"})
		return
	}

	writeJSON(w, settingsTestResponse{OK: true})
}

func (d *Dashboard) handleAPISettingsTestWebhook(w http.ResponseWriter, r *http.Request) {
	var req webhookTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	url := strings.TrimSpace(req.AlertWebhookURL)
	if url == "" {
		cfg, err := d.requireConfig()
		if err != nil || strings.TrimSpace(cfg.AlertWebhookURL) == "" {
			writeJSON(w, settingsTestResponse{OK: false, Error: "Webhook URL is not configured"})
			return
		}
		url = cfg.AlertWebhookURL
	}

	payload, _ := json.Marshal(map[string]any{
		"event":     "settings_test",
		"message":   "HitStor webhook test",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		writeJSON(w, settingsTestResponse{OK: false, Error: err.Error()})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		writeJSON(w, settingsTestResponse{OK: false, Error: fmt.Sprintf("webhook request failed: %v", err)})
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeJSON(w, settingsTestResponse{OK: false, Error: fmt.Sprintf("webhook returned HTTP %d", resp.StatusCode)})
		return
	}
	writeJSON(w, settingsTestResponse{OK: true})
}

func (d *Dashboard) effectiveIntegrations() (*config.Config, [32]byte, bool, error) {
	cfg, err := d.requireConfig()
	if err != nil {
		return nil, [32]byte{}, false, err
	}
	encKey, hasKey := d.encryptionKey()
	return cfg, encKey, hasKey, nil
}
