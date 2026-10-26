package dashboard

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hooperits/hitstor/internal/crypto"
)

func TestAuthLoginHashedPassword(t *testing.T) {
	hash, err := crypto.HashPassword("secret123")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	dash, cleanup := setupTestDashboard(t, hash)
	defer cleanup()

	handler := dash.Handler()

	// Correct password must log in even though opts.Password holds a hash.
	form := url.Values{"password": {"secret123"}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("POST /login with hashed stored password status = %d, want %d", rec.Code, http.StatusSeeOther)
	}

	// The literal hash string must NOT work as a password.
	form = url.Values{"password": {hash}}
	req = httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Invalid password") {
		t.Errorf("POST /login with hash as password: status = %d, want 200 with error", rec.Code)
	}
}
