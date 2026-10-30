package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/mailer"
	"github.com/hooperits/hitstor/internal/users"
)

// withEmail sets up email on dash and returns the messages it sends.
func withEmail(t *testing.T, dash *Dashboard) <-chan mailer.Message {
	t.Helper()
	sent := make(chan mailer.Message, 10)
	dash.SetConfig(&config.Config{Path: filepath.Join(t.TempDir(), "config.yaml"), Exists: true,
		Email: config.EmailConfig{Host: "relay.example.com", Port: 25, Security: mailer.SecurityNone, Auth: mailer.AuthNone,
			From: "hitstor@example.com", DashboardURL: "https://backup.example.com:8421"}})
	dash.sendMailHook = func(_ context.Context, m mailer.Message) error {
		sent <- m
		return nil
	}
	return sent
}

func nextMail(t *testing.T, sent <-chan mailer.Message) mailer.Message {
	t.Helper()
	select {
	case m := <-sent:
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("no email sent")
	}
	return mailer.Message{}
}

func noMail(t *testing.T, sent <-chan mailer.Message) {
	t.Helper()
	select {
	case m := <-sent:
		t.Fatalf("unexpected email to %v: %s", m.To, m.Subject)
	case <-time.After(200 * time.Millisecond):
	}
}

var linkRe = regexp.MustCompile(`https://backup\.example\.com:8421/login/reset\?token=([0-9a-f]+)`)

// mailToken returns the token of the link in m, checking both bodies have it.
func mailToken(t *testing.T, m mailer.Message) string {
	t.Helper()
	tm := linkRe.FindStringSubmatch(m.Text)
	if tm == nil || !strings.Contains(m.HTML, tm[0]) {
		t.Fatalf("no link to the configured dashboard address in the email:\n%s", m.Text)
	}
	return tm[1]
}

func TestNewUserGetsInvitation(t *testing.T) {
	dash, store := setupUsersDashboard(t)
	sent := withEmail(t, dash)
	h := dash.Handler()
	admin := signedIn(t, store, "admin", users.Grant{Role: users.RoleAdmin})

	rec := do(t, h, admin, "POST", "/api/users", map[string]any{"username": "ana", "display_name": "Ana Pérez",
		"email": "Ana@Example.com", "grants": []users.Grant{{Role: users.RoleViewer}}})
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if rec.Code != http.StatusCreated || resp["invited"] != true || resp["temporary_password"] != nil {
		t.Fatalf("add user: %d %s", rec.Code, rec.Body)
	}
	m := nextMail(t, sent)
	if m.To[0] != "ana@example.com" || !strings.Contains(m.Text, "Ana Pérez") || !strings.Contains(m.Text, "ana") {
		t.Fatalf("invitation: %v\n%s", m.To, m.Text)
	}
	token := mailToken(t, m)

	page := do(t, h, nil, "GET", "/login/reset?token="+token, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `name="token"`) || strings.Contains(page.Body.String(), `name="code"`) {
		t.Fatalf("invitation page: %d", page.Code)
	}
	if page.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Error("the link's page leaks its token in Referer headers")
	}
	form := url.Values{"token": {token}, "password": {"my own phrase 2026"}, "confirm": {"another phrase 2026"}}
	if rec := do(t, h, nil, "POST", "/login/reset", form); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "login-error") {
		t.Fatalf("mismatched passwords: %d", rec.Code)
	}
	form.Set("confirm", "my own phrase 2026")
	rec = do(t, h, nil, "POST", "/login/reset", form)
	if rec.Header().Get("Location") != "/login/enroll" {
		t.Fatalf("activate: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	sessionCookie(t, rec)
	u, _ := store.Get("ana")
	if u.MustChangePassword || !users.VerifyPassword(u.PasswordHash, "my own phrase 2026") {
		t.Fatal("password not chosen")
	}
	if rec := do(t, h, nil, "GET", "/login/reset?token="+token, nil); !strings.Contains(rec.Body.String(), `href="/login/forgot"`) {
		t.Fatal("a used link still shows its form")
	}
}

func TestInvitationFallsBackToTemporaryPassword(t *testing.T) {
	dash, store := setupUsersDashboard(t)
	withEmail(t, dash)
	dash.sendMailHook = func(context.Context, mailer.Message) error {
		return &mailError{"550 5.7.54 Unable to relay"}
	}
	admin := signedIn(t, store, "admin", users.Grant{Role: users.RoleAdmin})
	rec := do(t, dash.Handler(), admin, "POST", "/api/users", map[string]any{"username": "ana", "email": "ana@example.com",
		"grants": []users.Grant{{Role: users.RoleViewer}}})
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if rec.Code != http.StatusCreated || resp["temporary_password"] == nil || !strings.Contains(resp["email_error"].(string), "Unable to relay") {
		t.Fatalf("add user: %d %s", rec.Code, rec.Body)
	}
}

type mailError struct{ s string }

func (e *mailError) Error() string { return e.s }

func TestForgotPassword(t *testing.T) {
	dash, store := setupUsersDashboard(t)
	sent := withEmail(t, dash)
	h := dash.Handler()
	signedIn(t, store, "ana", users.Grant{Role: users.RoleViewer})
	store.SetEmail("ana", "ana@example.com")
	signedIn(t, store, "luis", users.Grant{Role: users.RoleViewer}) // no address

	if rec := do(t, h, nil, "GET", "/login", nil); !strings.Contains(rec.Body.String(), `href="/login/forgot"`) {
		t.Fatal("the sign-in page does not offer a reset")
	}
	// Unknown accounts and accounts without an address get the same answer, and no email.
	var answer string
	for _, login := range []string{"nobody", "luis", "ana@example.com"} {
		rec := do(t, h, nil, "POST", "/login/forgot", url.Values{"login": {login}})
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "login-notice") {
			t.Fatalf("%s: %d", login, rec.Code)
		}
		if answer == "" {
			answer = rec.Body.String()
		} else if rec.Body.String() != answer {
			t.Fatalf("%s: the answer differs, revealing whether the account exists", login)
		}
		if login != "ana@example.com" {
			noMail(t, sent)
		}
	}
	m := nextMail(t, sent)
	token := mailToken(t, m)
	if !strings.Contains(m.Text, "authenticator") {
		t.Error("the reset email does not warn that a code is needed")
	}

	page := do(t, h, nil, "GET", "/login/reset?token="+token, nil)
	if !strings.Contains(page.Body.String(), `name="code"`) {
		t.Fatal("an account with two-step verification is not asked for a code")
	}
	form := url.Values{"token": {token}, "password": {"a brand new phrase 7"}, "confirm": {"a brand new phrase 7"}, "code": {"000000"}}
	if rec := do(t, h, nil, "POST", "/login/reset", form); !strings.Contains(rec.Body.String(), "login-error") {
		t.Fatalf("wrong code accepted: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	u, _ := store.Get("ana")
	form.Set("code", totpCode(t, u.TOTPSecret, time.Now().Add(30*time.Second)))
	rec := do(t, h, nil, "POST", "/login/reset", form)
	if rec.Header().Get("Location") != "/login?changed=1" {
		t.Fatalf("reset: %d %q %s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	if _, _, err := store.Login("ana", "a brand new phrase 7", "", ""); err != nil {
		t.Fatalf("new password: %v", err)
	}
	// Right after, another request does not flood the mailbox.
	do(t, h, nil, "POST", "/login/forgot", url.Values{"login": {"ana"}})
	nextMail(t, sent)
	do(t, h, nil, "POST", "/login/forgot", url.Values{"login": {"ana"}})
	noMail(t, sent)
}

func TestForgotPasswordNeedsEmail(t *testing.T) {
	dash, store := setupUsersDashboard(t)
	sent := make(chan mailer.Message, 1)
	dash.sendMailHook = func(_ context.Context, m mailer.Message) error { sent <- m; return nil }
	h := dash.Handler()
	signedIn(t, store, "ana", users.Grant{Role: users.RoleViewer})
	store.SetEmail("ana", "ana@example.com")
	if rec := do(t, h, nil, "GET", "/login", nil); strings.Contains(rec.Body.String(), `href="/login/forgot"`) {
		t.Fatal("reset offered without email set up")
	}
	do(t, h, nil, "POST", "/login/forgot", url.Values{"login": {"ana"}})
	noMail(t, sent)
}

func TestAdministratorEmailsResetLink(t *testing.T) {
	dash, store := setupUsersDashboard(t)
	sent := withEmail(t, dash)
	h := dash.Handler()
	admin := signedIn(t, store, "admin", users.Grant{Role: users.RoleAdmin})
	signedIn(t, store, "ana", users.Grant{Role: users.RoleViewer})

	if rec := do(t, h, admin, "POST", "/api/users/ana/password", map[string]any{"send_email": true}); rec.Code != http.StatusConflict {
		t.Fatalf("no address: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, admin, "PUT", "/api/users/ana", map[string]any{"email": "ana@example.com"}); rec.Code != http.StatusOK {
		t.Fatalf("set email: %d %s", rec.Code, rec.Body)
	}
	before, _ := store.Get("ana")
	rec := do(t, h, admin, "POST", "/api/users/ana/password", map[string]any{"send_email": true})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"purpose":"reset"`) {
		t.Fatalf("send link: %d %s", rec.Code, rec.Body)
	}
	mailToken(t, nextMail(t, sent))
	if after, _ := store.Get("ana"); after.PasswordHash != before.PasswordHash {
		t.Fatal("the current password stopped working before the link was used")
	}
	// The older behaviour, a temporary password, still works.
	rec = do(t, h, admin, "POST", "/api/users/ana/password", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "temporary_password") {
		t.Fatalf("temporary password: %d %s", rec.Code, rec.Body)
	}
}

func TestEmailSettings(t *testing.T) {
	dash, store := setupUsersDashboard(t)
	withEmail(t, dash)
	h := dash.Handler()
	admin := signedIn(t, store, "admin", users.Grant{Role: users.RoleAdmin})
	viewer := signedIn(t, store, "eva", users.Grant{Role: users.RoleViewer})

	m365 := map[string]any{"host": "smtp.office365.com", "port": 587, "security": "starttls", "auth": "microsoft365",
		"tenant_id": "tenant-1", "client_id": "client-1", "client_secret": "s3cr3t-value",
		"from": "hitstor@example.com", "dashboard_url": "https://backup.example.com:8421/"}
	if rec := do(t, h, viewer, "PUT", "/api/settings/email", m365); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer changed email settings: %d", rec.Code)
	}
	if rec := do(t, h, admin, "PUT", "/api/settings/email", m365); rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	raw, _ := os.ReadFile(dash.cfg.Path)
	if strings.Contains(string(raw), "s3cr3t-value") || !strings.Contains(string(raw), "client_secret_encrypted:") {
		t.Fatalf("client secret not encrypted at rest:\n%s", raw)
	}
	view := func() EmailSettingsView {
		rec := do(t, h, admin, "GET", "/api/settings", nil)
		if strings.Contains(rec.Body.String(), "s3cr3t-value") {
			t.Fatal("settings show the secret")
		}
		var v SettingsView
		json.Unmarshal(rec.Body.Bytes(), &v)
		return v.Email
	}
	if v := view(); !v.Configured || !v.ClientSecretSet || v.DashboardURL != "https://backup.example.com:8421" {
		t.Fatalf("view: %+v", v)
	}
	// An empty secret keeps the saved one.
	delete(m365, "client_secret")
	if rec := do(t, h, admin, "PUT", "/api/settings/email", m365); rec.Code != http.StatusOK || !view().ClientSecretSet {
		t.Fatalf("keep secret: %d %s", rec.Code, rec.Body)
	}
	for name, mod := range map[string]func(map[string]any){
		"no TLS for credentials": func(m map[string]any) { m["security"] = "none" },
		"bad dashboard address":  func(m map[string]any) { m["dashboard_url"] = "backup.example.com" },
		"bad sender":             func(m map[string]any) { m["from"] = "not an address" },
	} {
		bad := map[string]any{}
		for k, v := range m365 {
			bad[k] = v
		}
		mod(bad)
		if rec := do(t, h, admin, "PUT", "/api/settings/email", bad); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	// A relay needs no secrets: the saved ones go.
	relay := map[string]any{"host": "relay.example.com", "port": 25, "security": "none", "auth": "none",
		"from": "hitstor@example.com", "dashboard_url": "https://backup.example.com:8421"}
	if rec := do(t, h, admin, "PUT", "/api/settings/email", relay); rec.Code != http.StatusOK || view().ClientSecretSet {
		t.Fatalf("relay: %d %s", rec.Code, rec.Body)
	}
	// The test sends with the form's settings.
	var got mailer.Message
	dash.sendMailHook = func(_ context.Context, m mailer.Message) error { got = m; return nil }
	relay["to"] = "admin@example.com"
	if rec := do(t, h, admin, "POST", "/api/settings/email/test", relay); !strings.Contains(rec.Body.String(), `"ok":true`) || got.To[0] != "admin@example.com" {
		t.Fatalf("test: %s %v", rec.Body, got.To)
	}
	// An empty host turns email off.
	if rec := do(t, h, admin, "PUT", "/api/settings/email", map[string]any{"host": ""}); rec.Code != http.StatusOK || view().Configured {
		t.Fatalf("turn off: %d", rec.Code)
	}
}
