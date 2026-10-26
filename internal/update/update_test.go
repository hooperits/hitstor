package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"1.0.0", "1.0.0", false},      // equal
		{"1.0.0", "1.0.1", true},       // newer patch
		{"1.0.0", "1.1.0", true},       // newer minor
		{"1.0.0", "2.0.0", true},       // newer major
		{"2.0.0", "1.0.0", false},      // older major
		{"1.1.0", "1.0.9", false},      // older minor
		{"1.0.1", "1.0.0", false},      // older patch
		{"0.12.0", "0.13.0", true},     // realistic upgrade
		{"0.13.0", "0.13.0", false},    // same version
		{"0.13.0", "1.0.0", true},      // major bump
		{"1.0.0-beta", "1.0.0", false}, // pre-release vs release (equal after strip)
		{"", "1.0.0", true},            // empty current
		{"1.0.0", "", false},           // empty latest
		{"", "", false},                // both empty
		{"dev", "1.0.0", true},         // dev version
		{"1.0.0", "dev", false},        // latest is dev
	}

	for _, tt := range tests {
		got := CompareVersions(tt.current, tt.latest)
		if got != tt.want {
			t.Errorf("CompareVersions(%q, %q) = %v, want %v",
				tt.current, tt.latest, got, tt.want)
		}
	}
}

func TestCheck(t *testing.T) {
	t.Run("update available", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]string{
				"tag_name": "v1.0.0",
				"html_url": "https://github.com/hooperits/hitstor/releases/tag/v1.0.0",
			})
		}))
		defer srv.Close()

		result, err := Check("0.12.0", srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.UpdateAvailable {
			t.Error("expected UpdateAvailable=true")
		}
		if result.LatestVersion != "1.0.0" {
			t.Errorf("LatestVersion = %q, want %q", result.LatestVersion, "1.0.0")
		}
		if result.ReleaseURL == "" {
			t.Error("expected non-empty ReleaseURL")
		}
	})

	t.Run("up to date", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]string{
				"tag_name": "v0.12.0",
				"html_url": "https://github.com/hooperits/hitstor/releases/tag/v0.12.0",
			})
		}))
		defer srv.Close()

		result, err := Check("0.12.0", srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.UpdateAvailable {
			t.Error("expected UpdateAvailable=false")
		}
	})

	t.Run("404 response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		result, err := Check("0.12.0", srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.UpdateAvailable {
			t.Error("expected UpdateAvailable=false on 404")
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not json"))
		}))
		defer srv.Close()

		result, err := Check("0.12.0", srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.UpdateAvailable {
			t.Error("expected UpdateAvailable=false on malformed JSON")
		}
	})

	t.Run("network error", func(t *testing.T) {
		result, err := Check("0.12.0", "http://127.0.0.1:1")
		if err != nil {
			t.Fatalf("expected nil error for network failure, got: %v", err)
		}
		if result.UpdateAvailable {
			t.Error("expected UpdateAvailable=false on network error")
		}
	})
}

func TestFormatResult(t *testing.T) {
	t.Run("update available", func(t *testing.T) {
		r := CheckResult{
			CurrentVersion:  "0.12.0",
			LatestVersion:   "1.0.0",
			UpdateAvailable: true,
			ReleaseURL:      "https://example.com/release",
		}
		got := FormatResult(r)
		if got == "" {
			t.Error("expected non-empty result")
		}
	})

	t.Run("up to date", func(t *testing.T) {
		r := CheckResult{
			CurrentVersion: "1.0.0",
			LatestVersion:  "1.0.0",
		}
		got := FormatResult(r)
		if got == "" {
			t.Error("expected non-empty result")
		}
	})

	t.Run("check failed", func(t *testing.T) {
		r := CheckResult{}
		got := FormatResult(r)
		if got != "Could not check for updates." {
			t.Errorf("got %q, want 'Could not check for updates.'", got)
		}
	})
}
