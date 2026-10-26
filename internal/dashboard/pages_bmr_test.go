package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDisasterPageRendersTheBareMetalTab(t *testing.T) {
	d := bmrTestDashboard(t)

	req := httptest.NewRequest(http.MethodGet, "/disaster", nil)
	rec := httptest.NewRecorder()
	d.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\nbody: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"dr-tab-baremetal", "bmr-hosts-body", "bmr-tokens-body",
		"/static/baremetal.js", "Runbook",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}
}

func TestBareMetalScriptIsServed(t *testing.T) {
	d := bmrTestDashboard(t)

	req := httptest.NewRequest(http.MethodGet, "/static/baremetal.js", nil)
	rec := httptest.NewRecorder()
	d.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "bmr-hosts-body") {
		t.Error("served script does not look like the bare-metal tab code")
	}
}
