package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAPISettingsGet(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rec := httptest.NewRecorder()
	dash.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var view SettingsView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.Network.ServePort != 8420 {
		t.Fatalf("serve_port = %d", view.Network.ServePort)
	}
	if view.Notifications.AlertStaleThreshold != 25 {
		t.Fatalf("stale threshold = %d", view.Notifications.AlertStaleThreshold)
	}
}

func TestAPISettingsIntegrationsPut(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	body := `{
		"gemini_api_key": "test-gemini-secret",
		"gemini_model": "gemini-1.5-pro"
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/settings/integrations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	dash.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rec = httptest.NewRecorder()
	dash.Handler().ServeHTTP(rec, req)

	var view SettingsView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if !view.Integrations.GeminiConfigured {
		t.Fatal("expected gemini configured")
	}
	if view.Integrations.GeminiModel != "gemini-1.5-pro" {
		t.Fatalf("gemini model = %q", view.Integrations.GeminiModel)
	}
	if view.Integrations.GeminiAPIKeyHint == "" {
		t.Fatal("expected masked gemini hint")
	}
}

func TestAPISettingsNotificationsPut(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	body := `{"alert_stale_threshold": 48, "alert_webhook_url": "https://example.com/hook"}`
	req := httptest.NewRequest(http.MethodPut, "/api/settings/notifications", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	dash.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rec = httptest.NewRecorder()
	dash.Handler().ServeHTTP(rec, req)

	var view SettingsView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.Notifications.AlertStaleThreshold != 48 {
		t.Fatalf("threshold = %d", view.Notifications.AlertStaleThreshold)
	}
	if view.Notifications.AlertWebhookURL != "https://example.com/hook" {
		t.Fatalf("webhook = %q", view.Notifications.AlertWebhookURL)
	}
}

func TestAPISettingsImportEnv(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	os.Setenv("GEMINI_API_KEY", "env-import-key")
	defer os.Unsetenv("GEMINI_API_KEY")

	req := httptest.NewRequest(http.MethodPost, "/api/settings/integrations/import-env", nil)
	rec := httptest.NewRecorder()
	dash.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rec = httptest.NewRecorder()
	dash.Handler().ServeHTTP(rec, req)

	var view SettingsView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if !view.Integrations.GeminiConfigured {
		t.Fatal("expected gemini configured after import")
	}
}
