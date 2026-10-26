package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func csrfRequest(t *testing.T, handler http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(`{"dashboard_password":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestCSRFBlocksCrossSiteMutation(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()
	handler := dash.Handler()

	tests := []struct {
		name    string
		headers map[string]string
	}{
		{"sec-fetch-site cross-site", map[string]string{"Sec-Fetch-Site": "cross-site"}},
		{"sec-fetch-site same-site", map[string]string{"Sec-Fetch-Site": "same-site"}},
		{"origin mismatch", map[string]string{"Origin": "https://evil.example"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := csrfRequest(t, handler, http.MethodPut, "/api/settings/security", tt.headers)
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
			}
		})
	}
}

func TestCSRFAllowsSameOriginAndNonBrowser(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()
	handler := dash.Handler()

	tests := []struct {
		name    string
		headers map[string]string
	}{
		{"sec-fetch-site same-origin", map[string]string{"Sec-Fetch-Site": "same-origin"}},
		{"sec-fetch-site none (address bar)", map[string]string{"Sec-Fetch-Site": "none"}},
		{"matching origin", map[string]string{"Origin": "http://example.com"}}, // httptest host is example.com
		{"no browser headers (curl)", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := csrfRequest(t, handler, http.MethodPut, "/api/settings/security", tt.headers)
			if rec.Code == http.StatusForbidden {
				t.Errorf("status = 403, want request allowed through CSRF check (body: %s)", rec.Body.String())
			}
		})
	}
}

func TestCSRFIgnoresSafeMethods(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()
	handler := dash.Handler()

	// GETs are never blocked, even when cross-site (navigation must work).
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Errorf("GET status = 403, want allowed")
	}
}
