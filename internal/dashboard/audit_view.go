package dashboard

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/i18n"
	"github.com/hooperits/hitstor/internal/users"
)

// Changes made through the API are audited by route: the action is
// "api:" + the route pattern and the target what it acted on. Account and
// user changes are audited by their handlers, with their own actions.
var auditedRoutes = map[string]struct {
	phrase   string // English message key; {target} is the target
	category string
}{
	"POST /api/backups":                                     {"Created the backup job {target}", "backups"},
	"PUT /api/backups/{id}":                                 {"Changed the backup job {target}", "backups"},
	"DELETE /api/backups/{id}":                              {"Deleted the backup job {target}", "backups"},
	"POST /api/backups/{id}/run":                            {"Started the backup job {target}", "backups"},
	"POST /api/backups/{id}/cancel":                         {"Cancelled the backup job {target}", "backups"},
	"POST /api/agents/{name}/suspend":                       {"Suspended the agent {target}", "backups"},
	"POST /api/agents/{name}/unsuspend":                     {"Resumed the agent {target}", "backups"},
	"POST /api/agents/{name}/schedule":                      {"Changed the schedule of the agent {target}", "backups"},
	"PUT /api/agents/{name}/plugins/{plugin}/object-config": {"Changed plugin settings on {target}", "backups"},
	"POST /api/snapshots/{id}/restore":                      {"Started a restore of snapshot {target}", "restores"},
	"POST /api/workload/restores":                           {"Started a database restore on {target}", "restores"},
	"POST /api/disaster/recover":                            {"Started a disaster recovery of {target}", "restores"},
	"POST /api/bmr/prepare/{agent}":                         {"Prepared bare-metal recovery of {target}", "restores"},
	"POST /api/bmr/media/{agent}/build":                     {"Built recovery media for {target}", "restores"},
	"DELETE /api/bmr/media/{agent}":                         {"Deleted the recovery media of {target}", "restores"},
	"DELETE /api/bmr/tokens/{id}":                           {"Revoked a bare-metal recovery token", "restores"},
	"POST /api/repos":                                       {"Added the repository {target}", "storage"},
	"DELETE /api/repos/{name}":                              {"Removed the repository {target}", "storage"},
	"POST /api/repos/{name}/prune":                          {"Pruned the repository {target}", "storage"},
	"POST /api/repos/{name}/forget":                         {"Applied the retention policy on {target}", "storage"},
	"POST /api/repos/{name}/check":                          {"Checked the repository {target}", "storage"},
	"POST /api/agents":                                      {"Added the agent {target}", "infrastructure"},
	"DELETE /api/agents/{name}":                             {"Removed the agent {target}", "infrastructure"},
	"POST /api/agents/{name}/configure":                     {"Configured the agent {target}", "infrastructure"},
	"POST /api/agents/{name}/repo":                          {"Changed the repository of the agent {target}", "infrastructure"},
	"POST /api/agents/{name}/plugins":                       {"Installed a plugin on {target}", "infrastructure"},
	"DELETE /api/agents/{name}/plugins/{id}":                {"Removed a plugin from {target}", "infrastructure"},
	"POST /api/plugin-packages":                             {"Uploaded a plugin package", "settings"},
	"DELETE /api/plugin-packages/{file}":                    {"Deleted the plugin package {target}", "settings"},
	"PUT /api/settings/general":                             {"Changed the general settings", "settings"},
	"PUT /api/settings/security":                            {"Changed the security settings", "settings"},
	"PUT /api/settings/network":                             {"Changed the network settings", "settings"},
	"PUT /api/settings/integrations":                        {"Changed the integrations", "settings"},
	"PUT /api/settings/notifications":                       {"Changed the notifications", "settings"},
	"PUT /api/settings/email":                               {"Changed the email settings", "settings"},
	"POST /api/settings/email/test":                         {"Sent a test email", "settings"},
	"POST /api/settings/integrations/import-env":            {"Imported integrations from the environment", "settings"},
	"POST /api/settings/tls/upload":                         {"Uploaded a TLS certificate", "settings"},
	"POST /api/alerts/{id}/resolve":                         {"Resolved an alert", "backups"},
}

// Phrases of the actions handlers audit themselves.
var auditActions = map[string]struct {
	ok, failed string
	category   string
}{
	"login":                    {"Signed in", "Sign-in failed", "signin"},
	"login.second-factor":      {"Completed two-step verification", "Two-step verification failed", "signin"},
	"logout":                   {"Signed out", "Signed out", "signin"},
	"setup":                    {"Created the first administrator {target}", "Could not create the first administrator", "signin"},
	"second-factor.enroll":     {"Set up their two-step verification", "Could not set up two-step verification", "signin"},
	"reauth":                   {"Confirmed their identity", "Could not confirm their identity", "signin"},
	"recovery-codes.new":       {"Generated new recovery codes", "Could not generate recovery codes", "signin"},
	"password.change":          {"Changed their password", "Could not change their password", "signin"},
	"password.reset-request":   {"Asked for a password reset link", "Asked for a password reset link that was not sent", "signin"},
	"password.reset":           {"Chose a new password with an emailed link", "Could not choose a new password with an emailed link", "signin"},
	"account.activate":         {"Activated their account with the emailed link", "Could not activate their account with the emailed link", "signin"},
	"session.end":              {"Ended one of their sessions", "Could not end a session", "signin"},
	"session.end-others":       {"Ended their other sessions", "Could not end their other sessions", "signin"},
	"profile.change":           {"Changed their name", "Could not change their name", "signin"},
	"user.add":                 {"Created the user {target}", "Could not create the user {target}", "users"},
	"user.roles":               {"Changed the roles of {target}", "Could not change the roles of {target}", "users"},
	"user.profile":             {"Changed the name of {target}", "Could not change the name of {target}", "users"},
	"user.enable":              {"Enabled the user {target}", "Could not enable the user {target}", "users"},
	"user.disable":             {"Disabled the user {target}", "Could not disable the user {target}", "users"},
	"user.remove":              {"Removed the user {target}", "Could not remove the user {target}", "users"},
	"user.password-reset":      {"Reset the password of {target}", "Could not reset the password of {target}", "users"},
	"user.invite":              {"Emailed an invitation to {target}", "Could not email an invitation to {target}", "users"},
	"user.reset-link":          {"Emailed a password reset link to {target}", "Could not email a password reset link to {target}", "users"},
	"user.email":               {"Changed the email address of {target}", "Could not change the email address of {target}", "users"},
	"user.second-factor-reset": {"Reset the two-step verification of {target}", "Could not reset the two-step verification of {target}", "users"},
	"user.session-end":         {"Ended a session of {target}", "Could not end a session of {target}", "users"},
	"denied":                   {"Was denied {target}", "Was denied {target}", "denied"},
	"audit.export":             {"Exported the audit log", "Could not export the audit log", ""},
}

// auditCategories are the filters of the audit page, in order.
var auditCategories = []struct{ Key, Label string }{
	{"signin", "Sign-ins and accounts"},
	{"users", "Users and roles"},
	{"backups", "Backups"},
	{"restores", "Restores and recovery"},
	{"storage", "Storage"},
	{"infrastructure", "Agents"},
	{"settings", "Settings"},
	{"denied", "Denied requests"},
}

func auditCategory(e users.AuditEvent) string {
	if p, ok := strings.CutPrefix(e.Action, "api:"); ok {
		return auditedRoutes[p].category
	}
	return auditActions[e.Action].category
}

// auditPhrase says what an event did, in c's language, with {target}
// left for the page to fill in (it shows the target in bold).
func auditPhrase(c *i18n.Catalog, e users.AuditEvent) string {
	if p, ok := strings.CutPrefix(e.Action, "api:"); ok {
		if r, ok := auditedRoutes[p]; ok {
			if e.OK {
				return c.T(r.phrase)
			}
			return c.Tf("Tried: {what}", "what", c.T(r.phrase))
		}
		return p
	}
	if a, ok := auditActions[e.Action]; ok {
		if e.OK {
			return c.T(a.ok)
		}
		return c.T(a.failed)
	}
	return e.Action
}

type auditEventJSON struct {
	Time   time.Time `json:"time"`
	User   string    `json:"user,omitempty"`
	IP     string    `json:"ip,omitempty"`
	Phrase string    `json:"phrase"`
	Target string    `json:"target,omitempty"`
	Detail string    `json:"detail,omitempty"`
	Result string    `json:"result"` // ok, failed, denied
	Action string    `json:"action"`
}

func auditResult(e users.AuditEvent) string {
	switch {
	case e.Action == "denied":
		return "denied"
	case e.OK:
		return "ok"
	}
	return "failed"
}

// auditFilter reads the audit page's filters from the query string.
func auditFilter(q map[string][]string) func(users.AuditEvent) bool {
	get := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return strings.TrimSpace(v[0])
		}
		return ""
	}
	user, category, result := get("user"), get("category"), get("result")
	text := strings.ToLower(get("q"))
	var since time.Time
	if days, err := strconv.Atoi(get("days")); err == nil && days > 0 {
		since = time.Now().AddDate(0, 0, -days)
	}
	return func(e users.AuditEvent) bool {
		switch {
		case user != "" && e.User != user,
			category != "" && auditCategory(e) != category,
			result == "ok" && auditResult(e) != "ok",
			result == "failed" && auditResult(e) == "ok",
			!since.IsZero() && e.Time.Before(since):
			return false
		}
		if text != "" {
			hay := strings.ToLower(e.User + " " + e.IP + " " + e.Action + " " + e.Target + " " + e.Detail)
			if !strings.Contains(hay, text) {
				return false
			}
		}
		return true
	}
}

const auditPageSize = 50

func (d *Dashboard) handleAPIAudit(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	events, total, brokenAt, err := d.audit.Query(auditFilter(r.URL.Query()), (page-1)*auditPageSize, auditPageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c := d.catalog(r)
	out := make([]auditEventJSON, 0, len(events))
	for _, e := range events {
		out = append(out, auditEventJSON{Time: e.Time, User: e.User, IP: e.IP, Phrase: auditPhrase(c, e),
			Target: e.Target, Detail: e.Detail, Result: auditResult(e), Action: e.Action})
	}
	writeJSON(w, map[string]any{"events": out, "total": total, "page": page, "page_size": auditPageSize, "broken_at": brokenAt})
}

// handleAPIAuditExport downloads the filtered events as CSV.
func (d *Dashboard) handleAPIAuditExport(w http.ResponseWriter, r *http.Request) {
	events, _, _, err := d.audit.Query(auditFilter(r.URL.Query()), 0, 1<<30)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c := d.catalog(r)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="hitstor-audit-`+time.Now().Format("2006-01-02")+`.csv"`)
	w.Write([]byte("\xef\xbb\xbf")) // BOM: Excel reads the accents right
	cw := csv.NewWriter(w)
	cw.Write([]string{c.T("Time"), c.T("User"), c.T("Address"), c.T("What they did"), c.T("Result"), c.T("Detail"), c.T("Action")})
	for _, e := range events {
		phrase := strings.ReplaceAll(auditPhrase(c, e), "{target}", e.Target)
		cw.Write([]string{e.Time.Local().Format("2006-01-02 15:04:05"), e.User, e.IP, phrase, auditResult(e), e.Detail, e.Action})
	}
	cw.Flush()
	d.auditEvent(r, "", "audit.export", "", true, "")
}

// statusRecorder remembers the status a handler answered with.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// Flush keeps streaming responses working.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// auditTarget names what an audited request acts on, before the handler
// runs (a deleted job has no name afterwards).
func (d *Dashboard) auditTarget(r *http.Request, pattern string) string {
	switch {
	case strings.Contains(pattern, "/api/backups/{id}"):
		if j := d.registry.GetJob(r.PathValue("id")); j != nil {
			return j.Name
		}
		return r.PathValue("id")
	case strings.Contains(pattern, "{id}") && strings.Contains(pattern, "/snapshots/"):
		return shortID(r.PathValue("id"))
	case strings.Contains(pattern, "{name}"):
		return r.PathValue("name")
	case strings.Contains(pattern, "{agent}"):
		return r.PathValue("agent")
	case strings.Contains(pattern, "{file}"):
		return r.PathValue("file")
	}
	// Creations name the new thing in the body.
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || r.ContentLength > 1<<20 {
		return ""
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	var body map[string]any
	if json.Unmarshal(raw, &body) != nil {
		return ""
	}
	for _, k := range []string{"name", "agent_name", "target_agent", "agent"} {
		if v, ok := body[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
