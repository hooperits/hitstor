package dashboard

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/cron"
	"github.com/hooperits/hitstor/internal/i18n"
)

// languageCookieName stores the language chosen with the dashboard selector.
const languageCookieName = "hitstor_lang"

// langFuncs returns the template helpers bound to catalog c:
//
//	{{t "Backups"}}                       translated text
//	{{tf "{n} agents" "n" .Count}}        translated text with placeholders
//	{{lang}}                              current language code
//	{{languages}}                         available languages (for the selector)
//	{{i18nJSON}}                          catalog for client-side t() (inside <script>)
func langFuncs(c *i18n.Catalog) template.FuncMap {
	msgs, _ := json.Marshal(c.Messages())
	return template.FuncMap{
		"t":         c.T,
		"tf":        c.Tf,
		"lang":      func() string { return c.Code },
		"languages": i18n.Languages,
		"i18nJSON":  func() template.JS { return template.JS(msgs) },
		"timeAgo":   func(v any) string { return timeAgoIn(c, v) },
	}
}

// language picks the language for a request: the selector cookie, then the
// configured default (serve_dashboard_language), then the browser's
// Accept-Language, then English.
func (d *Dashboard) language(r *http.Request) string {
	if r != nil {
		if ck, err := r.Cookie(languageCookieName); err == nil && i18n.Supported(ck.Value) {
			return ck.Value
		}
	}
	// An explicit server default wins over the browser preference.
	if d.cfg != nil && i18n.Supported(d.cfg.ServeDashboardLanguage) {
		return d.cfg.ServeDashboardLanguage
	}
	if r == nil {
		return i18n.Default
	}
	return i18n.Negotiate(r.Header.Get("Accept-Language"), i18n.Default)
}

// catalog returns the message catalog for a request.
func (d *Dashboard) catalog(r *http.Request) *i18n.Catalog {
	return i18n.Get(d.language(r))
}

// handleSetLanguage stores the chosen language (GET /language?set=es&next=/path)
// and redirects back. It is reachable without a session so the login page can
// switch language too.
func (d *Dashboard) handleSetLanguage(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("set")
	if !i18n.Supported(code) {
		http.Error(w, "unsupported language", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     languageCookieName,
		Value:    code,
		Path:     "/",
		MaxAge:   int((365 * 24 * time.Hour).Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, safeNext(r.URL.Query().Get("next")), http.StatusSeeOther)
}

// safeNext keeps redirects on this site: only absolute paths are allowed.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}

// formatPatterns lists messages whose translation is a format pattern that
// may use a different set of placeholders than the English text.
var formatPatterns = map[string][]string{
	"{h12}:{mm} {ampm}": {"h12", "hh", "mm", "ampm"},
}

// clockIn formats a time of day with the language's pattern. The pattern is
// the translation of "{h12}:{mm} {ampm}" and may use {h12} (1-12), {hh}
// (00-23), {mm} and {ampm}.
func clockIn(c *i18n.Catalog, hour, minute int) string {
	h12, ampm := hour%12, "AM"
	if h12 == 0 {
		h12 = 12
	}
	if hour >= 12 {
		ampm = "PM"
	}
	return c.Tf("{h12}:{mm} {ampm}",
		"h12", h12, "hh", fmt.Sprintf("%02d", hour), "mm", fmt.Sprintf("%02d", minute), "ampm", ampm)
}

// dateTimeIn formats t as a short date and time ("Jan 2 3:04 PM" in English).
func dateTimeIn(c *i18n.Catalog, t time.Time) string {
	return c.Tf("{month} {day} {clock}",
		"month", c.T(t.Format("Jan")), "day", t.Day(), "clock", clockIn(c, t.Hour(), t.Minute()))
}

// describeCron describes a cron expression in the catalog's language.
func describeCron(c *i18n.Catalog, expr cron.Expression) string {
	return expr.DescribeWith(c.Tf, func(h, m int) string { return clockIn(c, h, m) })
}
