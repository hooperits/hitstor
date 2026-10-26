package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hooperits/hitstor/internal/crypto"
)

func TestAPISettingsSecurityHashesPassword(t *testing.T) {
	dash, _, cleanup := setupTestDashboardWithRepoManager(t)
	defer cleanup()

	body := `{"dashboard_password":"new-secret"}`
	req := httptest.NewRequest(http.MethodPut, "/api/settings/security", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	dash.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	// The config must store an scrypt hash, never the plaintext password.
	stored := dash.cfg.ServeDashboardPassword
	if stored == "new-secret" {
		t.Fatal("config stores plaintext dashboard password, want scrypt hash")
	}
	if !crypto.IsHashedPassword(stored) {
		t.Fatalf("stored password = %q, want scrypt hash", stored)
	}
	if !crypto.VerifyPassword(stored, "new-secret") {
		t.Fatal("stored hash does not verify against the new password")
	}

	// The in-memory option must match, so a login after the change works.
	if dash.opts.Password != stored {
		t.Fatal("opts.Password not updated to the stored hash")
	}
}
