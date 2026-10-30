package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/hooperits/hitstor/internal/users"
)

const testUserPassword = "correct horse battery staple"

// setupUsersDashboard is a dashboard with accounts, the agents web01 and
// db01, and the shared password "secret123" (still needed for /setup).
func setupUsersDashboard(t *testing.T) (*Dashboard, *users.Store) {
	t.Helper()
	dash, cleanup := setupTestDashboard(t, "secret123")
	t.Cleanup(cleanup)
	dir := t.TempDir()
	store, err := users.Open(dir, [32]byte{1})
	if err != nil {
		t.Fatal(err)
	}
	audit, err := users.OpenAuditLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	dash.SetUsers(store, audit)
	for _, a := range []string{"web01", "db01"} {
		if _, err := dash.registry.AddAgent(a); err != nil {
			t.Fatal(err)
		}
	}
	return dash, store
}

func totpCode(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	c, err := totp.GenerateCodeCustom(secret, at, totp.ValidateOpts{Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// signedIn creates a user, sets up their authenticator and returns the
// cookie of the fully signed-in session.
func signedIn(t *testing.T, store *users.Store, name string, grants ...users.Grant) *http.Cookie {
	t.Helper()
	if _, err := store.Add(name, "", "", testUserPassword, grants, false); err != nil {
		t.Fatal(err)
	}
	token, _, err := store.Login(name, testUserPassword, "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	secret, _, _, err := store.BeginEnrollment(token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinishEnrollment(token, totpCode(t, secret, time.Now())); err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: sessionCookieName, Value: token}
}

func do(t *testing.T, h http.Handler, c *http.Cookie, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	switch b := body.(type) {
	case nil:
		req = httptest.NewRequest(method, path, nil)
	case url.Values:
		req = httptest.NewRequest(method, path, strings.NewReader(b.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	default:
		raw, _ := json.Marshal(b)
		req = httptest.NewRequest(method, path, strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
	}
	if c != nil {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			return c
		}
	}
	t.Fatalf("no session cookie (status %d, location %q)", rec.Code, rec.Header().Get("Location"))
	return nil
}

func TestSetupCreatesFirstAdministrator(t *testing.T) {
	dash, store := setupUsersDashboard(t)
	h := dash.Handler()

	if rec := do(t, h, nil, "GET", "/", nil); rec.Header().Get("Location") != "/setup" {
		t.Fatalf("GET / = %d %q, want redirect to /setup", rec.Code, rec.Header().Get("Location"))
	}
	form := url.Values{"current": {"wrong"}, "username": {"Maria"}, "password": {testUserPassword}, "confirm": {testUserPassword}}
	if rec := do(t, h, nil, "POST", "/setup", form); rec.Code != http.StatusOK || store.Count() != 0 {
		t.Fatalf("setup with a wrong dashboard password: status %d, users %d", rec.Code, store.Count())
	}
	form.Set("current", "secret123")
	rec := do(t, h, nil, "POST", "/setup", form)
	if rec.Header().Get("Location") != "/login/enroll" {
		t.Fatalf("setup: %d %q, want /login/enroll", rec.Code, rec.Header().Get("Location"))
	}
	u, err := store.Get("maria")
	if err != nil || !u.IsAdmin() {
		t.Fatalf("administrator not created: %+v %v", u, err)
	}
	// Without an authenticator the session only reaches enrollment.
	c := sessionCookie(t, rec)
	if rec := do(t, h, c, "GET", "/agents", nil); rec.Header().Get("Location") != "/login/enroll" {
		t.Fatalf("GET /agents before enrolling = %q, want /login/enroll", rec.Header().Get("Location"))
	}
	if rec := do(t, h, c, "GET", "/api/agents", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("API before enrolling = %d, want 401", rec.Code)
	}
	// /setup is closed once an account exists.
	if rec := do(t, h, nil, "GET", "/setup", nil); rec.Header().Get("Location") != "/login" {
		t.Fatalf("GET /setup after setup = %q, want /login", rec.Header().Get("Location"))
	}
}

var secretRe = regexp.MustCompile(`class="enroll-secret">([A-Z2-7]+)<`)

func TestSignInWithAuthenticator(t *testing.T) {
	dash, store := setupUsersDashboard(t)
	h := dash.Handler()
	if _, err := store.Add("ana", "", "", testUserPassword, []users.Grant{{Role: users.RoleViewer}}, false); err != nil {
		t.Fatal(err)
	}

	rec := do(t, h, nil, "POST", "/login", url.Values{"username": {"ana"}, "password": {"wrong password!"}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Invalid user name or password.") {
		t.Fatalf("wrong password: %d", rec.Code)
	}
	rec = do(t, h, nil, "POST", "/login", url.Values{"username": {"ana"}, "password": {testUserPassword}})
	if rec.Header().Get("Location") != "/login/enroll" {
		t.Fatalf("first sign-in goes to %q, want /login/enroll", rec.Header().Get("Location"))
	}
	c := sessionCookie(t, rec)

	// Enroll: the page shows the key; a code from it finishes enrollment.
	rec = do(t, h, c, "GET", "/login/enroll", nil)
	m := secretRe.FindStringSubmatch(rec.Body.String())
	if m == nil {
		t.Fatalf("enroll page has no key: %d", rec.Code)
	}
	now := time.Now()
	rec = do(t, h, c, "POST", "/login/enroll", url.Values{"code": {totpCode(t, m[1], now)}})
	if rec.Code != http.StatusOK || strings.Count(rec.Body.String(), "<li><code>") != 10 {
		t.Fatalf("enroll did not show 10 recovery codes: %d", rec.Code)
	}
	if rec := do(t, h, c, "GET", "/agents", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET /agents after enrolling = %d", rec.Code)
	}

	// Sign out; the next sign-in asks for a code, and a reused one fails.
	do(t, h, c, "POST", "/logout", nil)
	if rec := do(t, h, c, "GET", "/agents", nil); rec.Header().Get("Location") != "/login?expired=1" {
		t.Fatalf("old cookie after logout: %q", rec.Header().Get("Location"))
	}
	rec = do(t, h, nil, "POST", "/login", url.Values{"username": {"ana"}, "password": {testUserPassword}})
	if rec.Header().Get("Location") != "/login/code" {
		t.Fatalf("second sign-in goes to %q, want /login/code", rec.Header().Get("Location"))
	}
	c = sessionCookie(t, rec)
	if rec := do(t, h, c, "POST", "/login/code", url.Values{"code": {totpCode(t, m[1], now)}}); rec.Code != http.StatusOK {
		t.Fatalf("replayed code accepted: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	rec = do(t, h, c, "POST", "/login/code", url.Values{"code": {totpCode(t, m[1], now.Add(30*time.Second))}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("code: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := do(t, h, c, "GET", "/agents", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET /agents after code = %d", rec.Code)
	}
}

func agentNames(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	var list []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode agents: %v: %s", err, rec.Body.String())
	}
	var names []string
	for _, a := range list {
		names = append(names, a.Name)
	}
	return names
}

func TestScopedBackupOperator(t *testing.T) {
	dash, store := setupUsersDashboard(t)
	h := dash.Handler()
	c := signedIn(t, store, "ops", users.Grant{Role: users.RoleBackupOperator, Agents: []string{"web01"}})

	if got := agentNames(t, do(t, h, c, "GET", "/api/agents", nil)); len(got) != 1 || got[0] != "web01" {
		t.Errorf("agents seen = %v, want [web01]", got)
	}
	for _, tc := range []struct {
		method, path string
		body         any
		want         int
	}{
		{"GET", "/agents/web01", nil, http.StatusOK},
		{"GET", "/agents/db01", nil, http.StatusForbidden},
		{"GET", "/api/agents/db01/info", nil, http.StatusForbidden},
		{"POST", "/api/backups", map[string]any{"name": "x", "repo_name": "default", "agent_name": "db01", "paths": []string{"/"}}, http.StatusForbidden},
		{"GET", "/restores", nil, http.StatusForbidden},
		{"GET", "/settings", nil, http.StatusForbidden},
		{"GET", "/users", nil, http.StatusForbidden},
		{"GET", "/api/users", nil, http.StatusForbidden},
		{"GET", "/storage", nil, http.StatusForbidden},
		{"DELETE", "/api/agents/web01", nil, http.StatusForbidden},
		{"GET", "/account", nil, http.StatusOK},
	} {
		if rec := do(t, h, c, tc.method, tc.path, tc.body); rec.Code != tc.want {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, rec.Code, tc.want)
		}
	}
	// The overview spans every agent: a scoped user lands on their jobs.
	if rec := do(t, h, c, "GET", "/", nil); rec.Header().Get("Location") != "/backups" {
		t.Errorf("GET / = %d %q, want redirect to /backups", rec.Code, rec.Header().Get("Location"))
	}
}

func TestViewerIsReadOnly(t *testing.T) {
	dash, store := setupUsersDashboard(t)
	h := dash.Handler()
	c := signedIn(t, store, "auditor", users.Grant{Role: users.RoleViewer})

	if got := agentNames(t, do(t, h, c, "GET", "/api/agents", nil)); len(got) != 2 {
		t.Errorf("viewer sees agents %v, want both", got)
	}
	for _, path := range []string{"/", "/backups", "/agents/db01", "/storage"} {
		if rec := do(t, h, c, "GET", path, nil); rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, rec.Code)
		}
	}
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/backups"},
		{"GET", "/restores"},
		{"GET", "/settings"},
		{"POST", "/api/users"},
	} {
		if rec := do(t, h, c, tc.method, tc.path, map[string]any{}); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", tc.method, tc.path, rec.Code)
		}
	}
}

func TestAdministratorManagesUsers(t *testing.T) {
	dash, store := setupUsersDashboard(t)
	h := dash.Handler()
	c := signedIn(t, store, "admin", users.Grant{Role: users.RoleAdmin})

	rec := do(t, h, c, "POST", "/api/users", map[string]any{
		"username": "pruebas", "grants": []map[string]any{{"role": "backup_operator", "agents": []string{"web01"}}},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("add user = %d: %s", rec.Code, rec.Body.String())
	}
	var added struct {
		TemporaryPassword string `json:"temporary_password"`
	}
	json.Unmarshal(rec.Body.Bytes(), &added)
	u, err := store.Get("pruebas")
	if err != nil || !u.MustChangePassword || added.TemporaryPassword == "" {
		t.Fatalf("added user = %+v %v (temporary password %q)", u, err, added.TemporaryPassword)
	}
	if !u.Can(users.PermBackup, "web01") || u.Can(users.PermBackup, "db01") {
		t.Errorf("grants = %+v", u.Grants)
	}
	// The last administrator cannot remove themselves.
	if rec := do(t, h, c, "DELETE", "/api/users/admin", nil); rec.Code == http.StatusOK {
		t.Error("administrator removed their own account")
	}
	for _, path := range []string{"/users", "/audit", "/account"} {
		if rec := do(t, h, c, "GET", path, nil); rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d", path, rec.Code)
		}
	}
	if rec := do(t, h, c, "GET", "/api/audit?category=users", nil); !strings.Contains(rec.Body.String(), `"action":"user.add"`) ||
		!strings.Contains(rec.Body.String(), `"target":"pruebas"`) {
		t.Errorf("audit log does not show the new user: %s", rec.Body.String())
	}
	// The shared dashboard password no longer signs in: Settings stops offering it.
	if body := do(t, h, c, "GET", "/settings", nil).Body.String(); strings.Contains(body, "settings-dashboard-password") || !strings.Contains(body, `href="/users"`) {
		t.Error("Settings still offers the shared dashboard password")
	}
}

func TestTemporaryPasswordMustBeChanged(t *testing.T) {
	dash, store := setupUsersDashboard(t)
	h := dash.Handler()
	c := signedIn(t, store, "luis", users.Grant{Role: users.RoleViewer})
	if err := store.SetPassword("luis", "temporary-password-1", true, ""); err != nil {
		t.Fatal(err)
	}
	// Reset ends the sessions; signing in again lands on the password change.
	if rec := do(t, h, c, "GET", "/agents", nil); rec.Header().Get("Location") != "/login?expired=1" {
		t.Fatalf("session survived a password reset: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	token, _, err := store.Login("luis", "temporary-password-1", "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := store.Get("luis")
	if _, _, err := store.CompleteSecondFactor(token, totpCode(t, u.TOTPSecret, time.Now().Add(30*time.Second))); err != nil {
		t.Fatal(err)
	}
	c = &http.Cookie{Name: sessionCookieName, Value: token}
	if rec := do(t, h, c, "GET", "/agents", nil); rec.Header().Get("Location") != "/account/password" {
		t.Fatalf("GET /agents = %d %q, want /account/password", rec.Code, rec.Header().Get("Location"))
	}
	rec := do(t, h, c, "POST", "/api/account/password", url.Values{
		"current": {"temporary-password-1"}, "password": {testUserPassword + " 2"}, "confirm": {testUserPassword + " 2"},
	})
	if rec.Code >= 400 {
		t.Fatalf("change password = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, c, "GET", "/agents", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET /agents after the change = %d", rec.Code)
	}
}

func TestOnlyUserActivityCounts(t *testing.T) {
	for _, tc := range []struct {
		name    string
		method  string
		headers map[string]string
		want    bool
	}{
		{"page load", "GET", map[string]string{"Sec-Fetch-Mode": "navigate"}, true},
		{"auto-refresh", "GET", map[string]string{"Sec-Fetch-Mode": "cors", "HX-Request": "true"}, false},
		{"live progress", "GET", map[string]string{"Sec-Fetch-Mode": "cors", "Accept": "text/event-stream"}, false},
		{"change", "POST", map[string]string{"Sec-Fetch-Mode": "cors"}, true},
		{"old browser page load", "GET", map[string]string{"Accept": "text/html,*/*"}, true},
		{"old browser auto-refresh", "GET", map[string]string{"Accept": "text/html", "HX-Request": "true"}, false},
	} {
		r := httptest.NewRequest(tc.method, "/backups", nil)
		for k, v := range tc.headers {
			r.Header.Set(k, v)
		}
		if got := userActivity(r); got != tc.want {
			t.Errorf("%s: userActivity = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSessionStateAndExpiredNotice(t *testing.T) {
	dash, store := setupUsersDashboard(t)
	h := dash.Handler()
	c := signedIn(t, store, "ana", users.Grant{Role: users.RoleViewer})
	rec := do(t, h, c, "GET", "/api/session", nil)
	var s sessionState
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil || s.Absolute || s.ExpiresIn < 29*60 || s.ExpiresIn > 30*60 {
		t.Fatalf("session state = %+v %v (%s)", s, err, rec.Body.String())
	}
	do(t, h, c, "POST", "/logout", nil)
	rec = do(t, h, c, "GET", "/backups", nil)
	if loc := rec.Header().Get("Location"); loc != "/login?expired=1" {
		t.Fatalf("ended session redirects to %q", loc)
	}
	if rec := do(t, h, nil, "GET", "/login?expired=1", nil); !strings.Contains(rec.Body.String(), "login-notice") {
		t.Error("login page does not explain the session ended")
	}
}

func TestPagesHideActionsTheUserCannotTake(t *testing.T) {
	dash, store := setupUsersDashboard(t)
	h := dash.Handler()
	viewer := signedIn(t, store, "auditor", users.Grant{Role: users.RoleViewer})
	ops := signedIn(t, store, "ops", users.Grant{Role: users.RoleBackupOperator, Agents: []string{"web01"}})
	admin := signedIn(t, store, "admin", users.Grant{Role: users.RoleAdmin})
	for _, tc := range []struct {
		name   string
		c      *http.Cookie
		path   string
		marker string
		want   bool
	}{
		{"viewer: new job", viewer, "/backups", `onclick="HitStorBackupJob.openWizard()"`, false},
		{"operator: new job", ops, "/backups", `onclick="HitStorBackupJob.openWizard()"`, true},
		{"operator: add agent", ops, "/agents", `onclick="toggleAgentsAddPanel()"`, false},
		{"admin: add agent", admin, "/agents", `onclick="toggleAgentsAddPanel()"`, true},
		{"operator: suspend own agent", ops, "/agents/web01", `onclick="toggleSuspension(`, true},
		{"operator: remove agent", ops, "/agents/web01", `onclick="removeAgent()"`, false},
		{"viewer: suspend", viewer, "/agents/web01", `onclick="toggleSuspension(`, false},
		{"admin: remove agent", admin, "/agents/web01", `onclick="removeAgent()"`, true},
	} {
		body := do(t, h, tc.c, "GET", tc.path, nil).Body.String()
		if got := strings.Contains(body, tc.marker); got != tc.want {
			t.Errorf("%s: shown = %v, want %v", tc.name, got, tc.want)
		}
	}
	// Everyone gets the user menu; only administrators the Administration section.
	if body := do(t, h, ops, "GET", "/backups", nil).Body.String(); !strings.Contains(body, `id="user-menu-btn"`) || strings.Contains(body, `href="/users"`) {
		t.Error("operator: user menu missing or Users shown")
	}
}
