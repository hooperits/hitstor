package dashboard

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func postLogin(handler http.Handler, password, remoteAddr string) *httptest.ResponseRecorder {
	form := url.Values{"password": {password}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestLoginRateLimitBlocksAfterFailures(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "secret123")
	defer cleanup()
	handler := dash.Handler()

	// Exhaust the failure budget from one IP.
	for i := 0; i < maxLoginFailures; i++ {
		rec := postLogin(handler, "wrong", "10.0.0.1:1000")
		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: status = %d, want 200 (login page with error)", i+1, rec.Code)
		}
	}

	// Next attempt from the same IP is blocked even with the right password.
	rec := postLogin(handler, "secret123", "10.0.0.1:2000")
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("blocked attempt status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}

	// A different IP is unaffected.
	rec = postLogin(handler, "secret123", "10.0.0.2:1000")
	if rec.Code != http.StatusSeeOther {
		t.Errorf("other IP status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
}

func TestLoginRateLimitResetsOnSuccess(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "secret123")
	defer cleanup()
	handler := dash.Handler()

	// A few failures, then a success, then failures again: the success must
	// have cleared the counter so the budget is fresh.
	for i := 0; i < maxLoginFailures-1; i++ {
		postLogin(handler, "wrong", "10.0.0.3:1000")
	}
	rec := postLogin(handler, "secret123", "10.0.0.3:1000")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("login after %d failures status = %d, want 303", maxLoginFailures-1, rec.Code)
	}

	for i := 0; i < maxLoginFailures-1; i++ {
		rec = postLogin(handler, "wrong", "10.0.0.3:1000")
		if rec.Code != http.StatusOK {
			t.Fatalf("post-reset failure %d: status = %d, want 200", i+1, rec.Code)
		}
	}
}

func TestLoginRateLimitPerIP(t *testing.T) {
	dash, cleanup := setupTestDashboard(t, "secret123")
	defer cleanup()
	handler := dash.Handler()

	// Failures spread across many IPs never block any single one.
	for i := 0; i < maxLoginFailures*2; i++ {
		addr := fmt.Sprintf("10.1.0.%d:1000", i+1)
		rec := postLogin(handler, "wrong", addr)
		if rec.Code != http.StatusOK {
			t.Fatalf("IP %s: status = %d, want 200", addr, rec.Code)
		}
	}
}
