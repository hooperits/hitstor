package dashboard

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/crypto"
	"github.com/hooperits/hitstor/internal/users"
)

const (
	sessionCookieName = "hitstor_session"
	sessionDuration   = 24 * time.Hour

	// maxLoginFailures failed login attempts from one IP within
	// loginFailureWindow block further attempts until the window expires.
	// Accounts also lock on their own (see package users).
	maxLoginFailures   = 5
	loginFailureWindow = 15 * time.Minute
)

// loginLimiter tracks failed login attempts per client IP.
type loginLimiter struct {
	mu       sync.Mutex
	failures map[string]loginFailures
}

type loginFailures struct {
	count int
	first time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{failures: make(map[string]loginFailures)}
}

// blocked reports whether ip has exhausted its failure budget.
func (l *loginLimiter) blocked(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, ok := l.failures[ip]
	if !ok {
		return false
	}
	if time.Since(f.first) > loginFailureWindow {
		delete(l.failures, ip)
		return false
	}
	return f.count >= maxLoginFailures
}

// fail records a failed attempt for ip.
func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, ok := l.failures[ip]
	if !ok || time.Since(f.first) > loginFailureWindow {
		f = loginFailures{first: time.Now()}
	}
	f.count++
	l.failures[ip] = f
	// Opportunistically prune stale entries so the map cannot grow unbounded.
	if len(l.failures) > 1000 {
		now := time.Now()
		for k, v := range l.failures {
			if now.Sub(v.first) > loginFailureWindow {
				delete(l.failures, k)
			}
		}
	}
}

// reset clears the failure count for ip after a successful login.
func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, ip)
}

// clientIP extracts the client IP (without port) from a request.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// SetUsers turns on user accounts: once the store holds one, every request
// needs a signed-in user, and what it may do follows the user's roles.
func (d *Dashboard) SetUsers(store *users.Store, audit *users.AuditLog) {
	d.users, d.audit = store, audit
}

// authMode says how the dashboard authenticates.
type authMode int

const (
	authOpen     authMode = iota // no password, no users: anyone may use it
	authSetup                    // shared password and no accounts yet: create the first administrator
	authPassword                 // shared password and no place for accounts (no repository yet)
	authUsers                    // user accounts
)

func (d *Dashboard) authMode() authMode {
	switch {
	case d.users != nil && d.users.Count() > 0:
		return authUsers
	case d.opts.Password != "" && d.users != nil:
		return authSetup
	case d.opts.Password != "":
		return authPassword
	}
	return authOpen
}

// legacySessions are sign-ins with the shared dashboard password, used
// only while there are no accounts to sign in with (no repository yet).
type legacySessions struct {
	mu     sync.Mutex
	tokens map[string]time.Time
}

func (l *legacySessions) add() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for k, exp := range l.tokens {
		if now.After(exp) {
			delete(l.tokens, k)
		}
	}
	if l.tokens == nil {
		l.tokens = map[string]time.Time{}
	}
	token := randomToken(32)
	l.tokens[token] = now.Add(sessionDuration)
	return token
}

func (l *legacySessions) valid(token string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	exp, ok := l.tokens[token]
	return ok && time.Now().Before(exp)
}

func (l *legacySessions) remove(token string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.tokens, token)
}

type ctxKey int

const authKey ctxKey = 0

// authInfo is the signed-in user of a request (nil user: open dashboard).
type authInfo struct {
	user    *users.User
	session users.Session
	token   string
}

func authOf(r *http.Request) *authInfo {
	a, _ := r.Context().Value(authKey).(*authInfo)
	return a
}

// currentUser returns the request's user, or nil when the dashboard has no
// accounts (everyone may do everything, as before accounts).
func currentUser(r *http.Request) *users.User {
	if a := authOf(r); a != nil {
		return a.user
	}
	return nil
}

// isAPI reports whether r expects JSON rather than a page.
func isAPI(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/")
}

// protect wraps a handler with authentication: a fully signed-in user (who
// has changed a password an administrator set) or, before accounts exist,
// the shared password.
func (d *Dashboard) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch d.authMode() {
		case authOpen:
			next.ServeHTTP(w, r)
			return
		case authSetup:
			// Accounts replace the shared password: create the first
			// administrator with it.
			d.denyUnauthenticated(w, r, "/setup")
			return
		case authPassword:
			if c, err := r.Cookie(sessionCookieName); err == nil && d.legacy.valid(c.Value) {
				next.ServeHTTP(w, r)
				return
			}
			d.denyUnauthenticated(w, r, "/login")
			return
		}
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			d.denyUnauthenticated(w, r, "/login")
			return
		}
		sess, u, err := d.users.Authenticate(cookie.Value)
		if err != nil {
			d.denyUnauthenticated(w, r, "/login")
			return
		}
		switch sess.Stage {
		case users.StageSecondFactor:
			d.denyUnauthenticated(w, r, "/login/code")
			return
		case users.StageEnroll:
			d.denyUnauthenticated(w, r, "/login/enroll")
			return
		}
		if u.MustChangePassword && r.URL.Path != "/account/password" && r.URL.Path != "/api/account/password" {
			d.denyUnauthenticated(w, r, "/account/password")
			return
		}
		ctx := context.WithValue(r.Context(), authKey, &authInfo{user: &u, session: sess, token: cookie.Value})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (d *Dashboard) denyUnauthenticated(w http.ResponseWriter, r *http.Request, to string) {
	if isAPI(r) {
		http.Error(w, "not signed in", http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// audit records an event of the request's user.
func (d *Dashboard) auditEvent(r *http.Request, user, action, target string, ok bool, detail string) {
	if d.audit == nil {
		return
	}
	if user == "" {
		if u := currentUser(r); u != nil {
			user = u.Username
		}
	}
	d.audit.Append(users.AuditEvent{User: user, IP: clientIP(r), Action: action, Target: target, OK: ok, Detail: detail})
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		// Lax allows the session cookie on top-level redirects back from
		// Microsoft OAuth; state changes are guarded by crossOriginProtect.
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
}

// loginPage is the data of the sign-in pages (login.html): the step shown
// and what it needs.
type loginPage struct {
	Mode     string // login, code, enroll, recovery-codes, setup, change-password, open
	Error    string
	Username string
	QR       string // data: URL of the TOTP QR code
	Secret   string // TOTP secret, to type in by hand
	Codes    []string
	Warning  string
}

func (d *Dashboard) renderLogin(w http.ResponseWriter, r *http.Request, p loginPage) {
	d.render(w, r, "login", p)
}

func (d *Dashboard) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	switch d.authMode() {
	case authOpen:
		http.Redirect(w, r, "/", http.StatusSeeOther)
	case authSetup:
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
	case authPassword:
		d.renderLogin(w, r, loginPage{Mode: "password"})
	default:
		d.renderLogin(w, r, loginPage{Mode: "login"})
	}
}

// handlePasswordLogin signs in with the shared dashboard password.
func (d *Dashboard) handlePasswordLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if d.loginLimits.blocked(ip) {
		http.Error(w, "Too many failed login attempts; try again later", http.StatusTooManyRequests)
		return
	}
	if !crypto.VerifyPassword(d.opts.Password, r.FormValue("password")) {
		d.loginLimits.fail(ip)
		d.renderLogin(w, r, loginPage{Mode: "password", Error: d.catalog(r).T("Invalid password")})
		return
	}
	d.loginLimits.reset(ip)
	setSessionCookie(w, r, d.legacy.add())
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (d *Dashboard) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	switch d.authMode() {
	case authPassword:
		d.handlePasswordLogin(w, r)
		return
	case authSetup:
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	case authOpen:
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	ip := clientIP(r)
	if d.loginLimits.blocked(ip) {
		http.Error(w, "Too many failed login attempts; try again later", http.StatusTooManyRequests)
		return
	}
	name := r.FormValue("username")
	token, u, err := d.users.Login(name, r.FormValue("password"), ip, r.UserAgent())
	if err != nil {
		d.loginLimits.fail(ip)
		d.auditEvent(r, strings.ToLower(name), "login", "", false, err.Error())
		msg := d.catalog(r).T("Invalid user name or password.")
		if errors.Is(err, users.ErrLocked) {
			msg = d.userMessage(r, err)
		}
		d.renderLogin(w, r, loginPage{Mode: "login", Error: msg, Username: name})
		return
	}
	d.loginLimits.reset(ip)
	setSessionCookie(w, r, token)
	if u.TOTPEnabled {
		http.Redirect(w, r, "/login/code", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/login/enroll", http.StatusSeeOther)
}

// pendingSession returns the session of a sign-in in progress.
func (d *Dashboard) pendingSession(r *http.Request) (users.Session, users.User, string, bool) {
	if d.authMode() != authUsers {
		return users.Session{}, users.User{}, "", false
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return users.Session{}, users.User{}, "", false
	}
	sess, u, err := d.users.Authenticate(c.Value)
	if err != nil {
		return users.Session{}, users.User{}, "", false
	}
	return sess, u, c.Value, true
}

func (d *Dashboard) handleCodePage(w http.ResponseWriter, r *http.Request) {
	sess, _, _, ok := d.pendingSession(r)
	if !ok || sess.Stage != users.StageSecondFactor {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	d.renderLogin(w, r, loginPage{Mode: "code"})
}

func (d *Dashboard) handleCodeSubmit(w http.ResponseWriter, r *http.Request) {
	sess, u, token, ok := d.pendingSession(r)
	if !ok || sess.Stage != users.StageSecondFactor {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	ip := clientIP(r)
	if d.loginLimits.blocked(ip) {
		http.Error(w, "Too many failed login attempts; try again later", http.StatusTooManyRequests)
		return
	}
	usedRecovery, left, err := d.users.CompleteSecondFactor(token, r.FormValue("code"))
	if err != nil {
		d.loginLimits.fail(ip)
		d.auditEvent(r, u.Username, "login.second-factor", "", false, err.Error())
		msg := d.catalog(r).T("Wrong code.")
		if errors.Is(err, users.ErrLocked) {
			msg = d.userMessage(r, err)
		}
		d.renderLogin(w, r, loginPage{Mode: "code", Error: msg})
		return
	}
	d.loginLimits.reset(ip)
	detail := ""
	if usedRecovery {
		detail = "recovery code used"
	}
	d.auditEvent(r, u.Username, "login", "", true, detail)
	if usedRecovery {
		// Say how many are left, and offer new ones.
		d.renderLogin(w, r, loginPage{Mode: "recovery-used", Codes: make([]string, left)})
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (d *Dashboard) handleEnrollPage(w http.ResponseWriter, r *http.Request) {
	sess, u, token, ok := d.pendingSession(r)
	if !ok || (sess.Stage != users.StageEnroll && sess.Stage != users.StageFull) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	d.renderEnroll(w, r, token, u.Username, "")
}

func (d *Dashboard) renderEnroll(w http.ResponseWriter, r *http.Request, token, username, errMsg string) {
	secret, _, png, err := d.users.BeginEnrollment(token)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	d.renderLogin(w, r, loginPage{Mode: "enroll", Username: username, Error: errMsg,
		QR: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), Secret: secret})
}

func (d *Dashboard) handleEnrollSubmit(w http.ResponseWriter, r *http.Request) {
	sess, u, token, ok := d.pendingSession(r)
	if !ok || (sess.Stage != users.StageEnroll && sess.Stage != users.StageFull) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	codes, err := d.users.FinishEnrollment(token, r.FormValue("code"))
	if err != nil {
		d.auditEvent(r, u.Username, "second-factor.enroll", "", false, err.Error())
		d.renderEnroll(w, r, token, u.Username, d.catalog(r).T("Wrong code: check that the time on your phone is right, and try the next code."))
		return
	}
	d.auditEvent(r, u.Username, "second-factor.enroll", "", true, "")
	d.renderLogin(w, r, loginPage{Mode: "recovery-codes", Codes: codes})
}

func (d *Dashboard) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil && d.users != nil {
		if _, u, err := d.users.Authenticate(c.Value); err == nil {
			d.auditEvent(r, u.Username, "logout", "", true, "")
		}
		d.users.Logout(c.Value)
	}
	if c, err := r.Cookie(sessionCookieName); err == nil {
		d.legacy.remove(c.Value)
	}
	clearSessionCookie(w)
	w.Header().Set("Clear-Site-Data", `"cookies", "storage"`)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// --- First administrator ---

func (d *Dashboard) handleSetupPage(w http.ResponseWriter, r *http.Request) {
	if d.authMode() == authUsers || d.users == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	d.renderLogin(w, r, loginPage{Mode: "setup"})
}

// handleSetupSubmit creates the first administrator. With a shared password
// configured, it must be given: whoever had the dashboard creates its first
// account.
func (d *Dashboard) handleSetupSubmit(w http.ResponseWriter, r *http.Request) {
	if d.authMode() == authUsers || d.users == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	ip := clientIP(r)
	if d.loginLimits.blocked(ip) {
		http.Error(w, "Too many failed attempts; try again later", http.StatusTooManyRequests)
		return
	}
	page := loginPage{Mode: "setup", Username: r.FormValue("username")}
	if d.opts.Password != "" && !crypto.VerifyPassword(d.opts.Password, r.FormValue("current")) {
		d.loginLimits.fail(ip)
		d.auditEvent(r, "", "setup", "", false, "wrong dashboard password")
		page.Error = d.catalog(r).T("The current dashboard password is not correct.")
		d.renderLogin(w, r, page)
		return
	}
	if r.FormValue("password") != r.FormValue("confirm") {
		page.Error = d.catalog(r).T("The passwords do not match.")
		d.renderLogin(w, r, page)
		return
	}
	u, err := d.users.Add(r.FormValue("username"), "", r.FormValue("password"), []users.Grant{{Role: users.RoleAdmin}}, false)
	if err != nil {
		page.Error = d.userMessage(r, err)
		d.renderLogin(w, r, page)
		return
	}
	d.auditEvent(r, u.Username, "setup", u.Username, true, "first administrator created")
	token, _, err := d.users.Login(u.Username, r.FormValue("password"), ip, r.UserAgent())
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	setSessionCookie(w, r, token)
	http.Redirect(w, r, "/login/enroll", http.StatusSeeOther)
}

// randomToken returns a random hex string.
func randomToken(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
