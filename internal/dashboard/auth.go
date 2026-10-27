package dashboard

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/crypto"
)

const (
	sessionCookieName = "hitstor_session"
	sessionDuration   = 24 * time.Hour

	// maxLoginFailures failed login attempts from one IP within
	// loginFailureWindow block further attempts until the window expires.
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

// protect wraps a handler with authentication if a password is configured.
func (d *Dashboard) protect(next http.Handler) http.Handler {
	if d.opts.Password == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !d.isAuthenticated(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (d *Dashboard) isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	d.mu.RLock()
	expiry, ok := d.sessions[cookie.Value]
	d.mu.RUnlock()
	return ok && time.Now().Before(expiry)
}

func (d *Dashboard) createSession() string {
	var token [16]byte
	rand.Read(token[:])
	sessionToken := hex.EncodeToString(token[:])

	d.mu.Lock()
	d.sessions[sessionToken] = time.Now().Add(sessionDuration)
	// Clean up expired sessions.
	now := time.Now()
	for k, v := range d.sessions {
		if now.After(v) {
			delete(d.sessions, k)
		}
	}
	d.mu.Unlock()

	return sessionToken
}

func (d *Dashboard) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if d.opts.Password == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	d.render(w, r, "login", nil)
}

func (d *Dashboard) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if d.opts.Password == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	ip := clientIP(r)
	if d.loginLimits.blocked(ip) {
		http.Error(w, "Too many failed login attempts; try again later", http.StatusTooManyRequests)
		return
	}

	password := r.FormValue("password")
	if !crypto.VerifyPassword(d.opts.Password, password) {
		d.loginLimits.fail(ip)
		d.render(w, r, "login", map[string]any{"Error": "Invalid password"})
		return
	}

	d.loginLimits.reset(ip)
	token := d.createSession()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		// Lax allows the session cookie on top-level redirects back from Microsoft OAuth.
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionDuration.Seconds()),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (d *Dashboard) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		d.mu.Lock()
		delete(d.sessions, cookie.Value)
		d.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:   sessionCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
