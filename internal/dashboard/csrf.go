package dashboard

import (
	"net/http"
	"net/url"
)

// crossOriginProtect rejects state-changing requests that browsers mark as
// cross-origin, mirroring the semantics of Go 1.25's http.CrossOriginProtection.
//
// Safe methods (GET, HEAD, OPTIONS) pass through so navigation keeps working.
// For other methods the Sec-Fetch-Site header (sent by all modern browsers) is
// authoritative; when absent, the Origin header must match the request host.
// Requests with neither header come from non-browser clients, which cannot be
// driven by a victim's browser and therefore carry no CSRF risk.
func crossOriginProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}

		switch r.Header.Get("Sec-Fetch-Site") {
		case "", "same-origin", "none":
			// Fall through to the Origin check ("" = header absent).
		default:
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}

		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host {
				http.Error(w, "cross-origin request rejected", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
