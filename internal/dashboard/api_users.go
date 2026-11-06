package dashboard

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/i18n"
	"github.com/hooperits/hitstor/internal/users"
)

// userJSON is an account as the dashboard shows it: never its secrets.
type userJSON struct {
	Username      string        `json:"username"`
	DisplayName   string        `json:"display_name,omitempty"`
	Email         string        `json:"email,omitempty"`
	Grants        []users.Grant `json:"grants"`
	Disabled      bool          `json:"disabled"`
	TOTPEnabled   bool          `json:"totp_enabled"`
	RecoveryLeft  int           `json:"recovery_codes_left"`
	Locked        bool          `json:"locked"`
	MustChange    bool          `json:"must_change_password"`
	CreatedAt     time.Time     `json:"created_at"`
	LastLoginAt   *time.Time    `json:"last_login_at,omitempty"`
	PasswordSince time.Time     `json:"password_changed_at"`
}

func toUserJSON(u users.User) userJSON {
	return userJSON{Username: u.Username, DisplayName: u.DisplayName, Email: u.Email, Grants: u.Grants, Disabled: u.Disabled,
		TOTPEnabled: u.TOTPEnabled, RecoveryLeft: len(u.RecoveryCodes), Locked: u.Locked(time.Now()),
		MustChange: u.MustChangePassword, CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt, PasswordSince: u.PasswordChangedAt}
}

// usersReady answers 503 when the dashboard has no account store.
func (d *Dashboard) usersReady(w http.ResponseWriter) bool {
	if d.users == nil {
		http.Error(w, "user accounts need a repository", http.StatusServiceUnavailable)
		return false
	}
	return true
}

// decodeBody reads a JSON body, or a form for plain HTML forms.
func decodeBody(r *http.Request, v any) error {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return json.NewDecoder(r.Body).Decode(v)
	}
	if err := r.ParseForm(); err != nil {
		return err
	}
	m := map[string]string{}
	for k := range r.PostForm {
		m[k] = r.PostForm.Get(k)
	}
	raw, _ := json.Marshal(m)
	return json.Unmarshal(raw, v)
}

// userMessage says err in the request's language when it is one of the
// errors people cause (a weak password, a taken user name...).
func (d *Dashboard) userMessage(r *http.Request, err error) string {
	c := d.catalog(r)
	switch {
	case errors.Is(err, users.ErrPasswordShort):
		return c.Tf("The password needs at least {n} characters.", "n", users.MinPasswordLength)
	case errors.Is(err, users.ErrPasswordLong):
		return c.Tf("The password can have at most {n} characters.", "n", users.MaxPasswordLength)
	case errors.Is(err, users.ErrPasswordCommon):
		return c.T("That password is too common: choose another.")
	case errors.Is(err, users.ErrPasswordHasName):
		return c.T("The password cannot contain the user name or the product name.")
	case errors.Is(err, users.ErrPasswordUnchanged):
		return c.T("Choose a password different from the current one.")
	case errors.Is(err, users.ErrBadUsername):
		return c.T("User names have 2 to 64 characters: letters, digits, '.', '_' or '-'.")
	case errors.Is(err, users.ErrNoRoles):
		return c.T("Give the user at least one role.")
	case errors.Is(err, users.ErrUnknownRole):
		return c.T("Unknown role.")
	case errors.Is(err, users.ErrUserExists):
		return c.T("A user with that name already exists.")
	case errors.Is(err, users.ErrNoUser):
		return c.T("No such user.")
	case errors.Is(err, users.ErrLastAdmin):
		return c.T("The last active administrator cannot be removed, disabled or lose the role.")
	case errors.Is(err, users.ErrBadCredentials):
		return c.T("Wrong password.")
	case errors.Is(err, users.ErrBadCode):
		return c.T("Wrong code.")
	case errors.Is(err, users.ErrLocked):
		return c.T("Too many failed attempts: the account is locked for a few minutes.")
	case errors.Is(err, users.ErrBadEmail):
		return c.T("That email address is not valid.")
	case errors.Is(err, users.ErrEmailTaken):
		return c.T("Another user has that email address.")
	case errors.Is(err, users.ErrTooSoon):
		return c.T("A link was sent moments ago: wait a couple of minutes before sending another.")
	}
	return err.Error()
}

func (d *Dashboard) userError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, users.ErrNoUser):
		status = http.StatusNotFound
	case errors.Is(err, users.ErrUserExists), errors.Is(err, users.ErrLastAdmin), errors.Is(err, users.ErrEmailTaken):
		status = http.StatusConflict
	case errors.Is(err, users.ErrBadCredentials), errors.Is(err, users.ErrBadCode), errors.Is(err, users.ErrLocked):
		status = http.StatusForbidden
	}
	http.Error(w, d.userMessage(r, err), status)
}

// --- The user's own account ---

func (d *Dashboard) handleAccountPage(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	data := d.newPageData("My account", "Your profile, security and sessions", "account", map[string]any{
		"User": toUserJSON(*u),
	})
	d.render(w, r, "account", data)
}

func (d *Dashboard) handleAccountPasswordPage(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	d.renderLogin(w, r, loginPage{Mode: "change-password", Username: u.Username})
}

func (d *Dashboard) handleAPIAccountPassword(w http.ResponseWriter, r *http.Request) {
	a := authOf(r)
	if a == nil || a.user == nil || !d.usersReady(w) {
		http.Error(w, "no account", http.StatusBadRequest)
		return
	}
	var req struct {
		Current  string `json:"current"`
		Password string `json:"password"`
		Confirm  string `json:"confirm"`
	}
	form := !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json")
	fail := func(msg string) {
		if form {
			d.renderLogin(w, r, loginPage{Mode: "change-password", Username: a.user.Username, Error: msg})
			return
		}
		http.Error(w, msg, http.StatusBadRequest)
	}
	if err := decodeBody(r, &req); err != nil {
		fail(d.catalog(r).T("Invalid request."))
		return
	}
	if req.Password != req.Confirm {
		fail(d.catalog(r).T("The passwords do not match."))
		return
	}
	if err := d.users.ChangeOwnPassword(a.token, req.Current, req.Password); err != nil {
		d.auditEvent(r, "", "password.change", "", false, err.Error())
		if errors.Is(err, users.ErrBadCredentials) {
			fail(d.catalog(r).T("The current password is not correct."))
			return
		}
		fail(d.userMessage(r, err))
		return
	}
	d.auditEvent(r, "", "password.change", "", true, "")
	if form {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (d *Dashboard) handleAPIAccountRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil || !d.usersReady(w) {
		http.Error(w, "no account", http.StatusBadRequest)
		return
	}
	codes, err := d.users.NewRecoveryCodes(u.Username)
	if err != nil {
		d.userError(w, r, err)
		return
	}
	d.auditEvent(r, "", "recovery-codes.new", "", true, "")
	writeJSON(w, map[string]any{"codes": codes})
}

type sessionJSON struct {
	ID        string    `json:"id"`
	Created   time.Time `json:"created"`
	LastSeen  time.Time `json:"last_seen"`
	IP        string    `json:"ip,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
	Browser   string    `json:"browser"` // "Chrome on Windows"
	Current   bool      `json:"current"`
}

// describeBrowser names the browser and system of a User-Agent header.
func describeBrowser(c *i18n.Catalog, ua string) string {
	browser := ""
	for _, b := range []struct{ token, name string }{
		{"Edg/", "Edge"}, {"OPR/", "Opera"}, {"Firefox/", "Firefox"}, {"Chrome/", "Chrome"},
		{"Safari/", "Safari"}, {"curl/", "curl"}, {"PowerShell", "PowerShell"},
	} {
		if strings.Contains(ua, b.token) {
			browser = b.name
			break
		}
	}
	system := ""
	for _, o := range []struct{ token, name string }{
		{"Windows", "Windows"}, {"iPhone", "iOS"}, {"iPad", "iOS"}, {"Mac OS X", "macOS"},
		{"Android", "Android"}, {"Linux", "Linux"},
	} {
		if strings.Contains(ua, o.token) {
			system = o.name
			break
		}
	}
	switch {
	case browser != "" && system != "":
		return c.Tf("{browser} on {system}", "browser", browser, "system", system)
	case browser != "":
		return browser
	case system != "":
		return system
	}
	return c.T("Unknown browser")
}

func (d *Dashboard) sessionsJSON(r *http.Request, name string) []sessionJSON {
	cur := ""
	if a := authOf(r); a != nil {
		cur = a.session.ID
	}
	out := []sessionJSON{}
	for _, s := range d.users.Sessions(name) {
		out = append(out, sessionJSON{ID: s.ID, Created: s.Created, LastSeen: s.LastSeen, IP: s.IP, UserAgent: s.UserAgent,
			Browser: describeBrowser(d.catalog(r), s.UserAgent), Current: s.ID == cur})
	}
	return out
}

func (d *Dashboard) handleAPIAccountSessions(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil || !d.usersReady(w) {
		writeJSON(w, []sessionJSON{})
		return
	}
	writeJSON(w, d.sessionsJSON(r, u.Username))
}

func (d *Dashboard) handleAPIAccountEndSession(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil || !d.usersReady(w) {
		http.Error(w, "no account", http.StatusBadRequest)
		return
	}
	if err := d.users.EndSession(u.Username, r.PathValue("id")); err != nil {
		d.userError(w, r, err)
		return
	}
	d.auditEvent(r, "", "session.end", r.PathValue("id"), true, "")
	writeJSON(w, map[string]any{"ok": true})
}

// handleAPIAccountEndOtherSessions signs the user out of every other browser.
func (d *Dashboard) handleAPIAccountEndOtherSessions(w http.ResponseWriter, r *http.Request) {
	a := authOf(r)
	if a == nil || a.user == nil || !d.usersReady(w) {
		http.Error(w, "no account", http.StatusBadRequest)
		return
	}
	n, err := d.users.EndOtherSessions(a.user.Username, a.session.ID)
	if err != nil {
		d.userError(w, r, err)
		return
	}
	d.auditEvent(r, "", "session.end-others", "", true, strconv.Itoa(n))
	writeJSON(w, map[string]any{"ended": n})
}

// handleAPIAccountProfile changes the signed-in user's display name.
func (d *Dashboard) handleAPIAccountProfile(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil || !d.usersReady(w) {
		http.Error(w, "no account", http.StatusBadRequest)
		return
	}
	var req struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := d.users.SetDisplayName(u.Username, req.DisplayName); err != nil {
		d.userError(w, r, err)
		return
	}
	d.auditEvent(r, "", "profile.change", "", true, strings.TrimSpace(req.DisplayName))
	writeJSON(w, map[string]any{"ok": true})
}

// handleAPIReauth confirms the signed-in user before a sensitive action.
// sessionState tells the page when its session ends, so it can warn first.
// Seconds are counted on the server: the browser's clock may be off.
type sessionState struct {
	ExpiresIn int  `json:"expires_in"` // seconds
	Absolute  bool `json:"absolute"`   // the 12-hour limit: activity cannot extend it
}

func (d *Dashboard) writeSessionState(w http.ResponseWriter, r *http.Request) {
	a := authOf(r)
	if a == nil || a.user == nil {
		writeJSON(w, map[string]any{"accounts": false})
		return
	}
	at, absolute := a.session.Expires(*a.user)
	writeJSON(w, sessionState{ExpiresIn: int(time.Until(at).Seconds()), Absolute: absolute})
}

// handleAPISession reports the session's remaining time; it does not count
// as activity.
func (d *Dashboard) handleAPISession(w http.ResponseWriter, r *http.Request) {
	d.writeSessionState(w, r)
}

// handleAPISessionActivity records that the user is working on the page
// (being a POST, protect already counted it) and reports the new limit.
func (d *Dashboard) handleAPISessionActivity(w http.ResponseWriter, r *http.Request) {
	d.writeSessionState(w, r)
}

func (d *Dashboard) handleAPIReauth(w http.ResponseWriter, r *http.Request) {
	a := authOf(r)
	if a == nil || a.user == nil || !d.usersReady(w) {
		writeJSON(w, map[string]any{"ok": true})
		return
	}
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := decodeBody(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := d.users.Reauthenticate(a.token, req.Password, req.Code); err != nil {
		d.auditEvent(r, "", "reauth", "", false, err.Error())
		d.userError(w, r, err)
		return
	}
	d.auditEvent(r, "", "reauth", "", true, "")
	writeJSON(w, map[string]any{"ok": true})
}

// --- Administration ---

func (d *Dashboard) handleUsersPage(w http.ResponseWriter, r *http.Request) {
	var agentNames []string
	for _, a := range d.registry.ListAgents() {
		agentNames = append(agentNames, a.Name)
	}
	data := d.newPageData("Users and access", "Who can sign in, and what they can do on each agent", "users", map[string]any{
		"Agents":       agentNames,
		"Roles":        users.Roles,
		"EmailEnabled": d.emailConfigured(),
	})
	d.render(w, r, "users", data)
}

// handleUserPage is an account's page: roles, sessions and activity.
func (d *Dashboard) handleUserPage(w http.ResponseWriter, r *http.Request) {
	if !d.usersReady(w) {
		return
	}
	u, err := d.users.Get(r.PathValue("name"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var agentNames []string
	for _, a := range d.registry.ListAgents() {
		agentNames = append(agentNames, a.Name)
	}
	title := u.DisplayName
	if title == "" {
		title = u.Username
	}
	data := d.newPageData(title, "", "user", map[string]any{
		"User":         toUserJSON(u),
		"Agents":       agentNames,
		"Roles":        users.Roles,
		"Self":         currentUser(r) != nil && currentUser(r).Username == u.Username,
		"EmailEnabled": d.emailConfigured(),
	})
	data.TitleRaw = true
	data.Crumbs = []crumb{{URL: "/users", Label: "Users and access"}}
	d.render(w, r, "user", data)
}

func (d *Dashboard) handleAPIUser(w http.ResponseWriter, r *http.Request) {
	if !d.usersReady(w) {
		return
	}
	u, err := d.users.Get(r.PathValue("name"))
	if err != nil {
		d.userError(w, r, err)
		return
	}
	writeJSON(w, toUserJSON(u))
}

func (d *Dashboard) handleAPIUsers(w http.ResponseWriter, r *http.Request) {
	if !d.usersReady(w) {
		return
	}
	out := []userJSON{}
	for _, u := range d.users.List() {
		out = append(out, toUserJSON(u))
	}
	writeJSON(w, out)
}

// handleAPIAddUser creates an account. With email set up and an address,
// the user gets an invitation link to choose their own password, and the
// temporary password is never shown. Otherwise (or if the email fails) the
// administrator sees it once and passes it on; the user must change it at
// first sign-in.
func (d *Dashboard) handleAPIAddUser(w http.ResponseWriter, r *http.Request) {
	if !d.usersReady(w) {
		return
	}
	var req struct {
		Username    string        `json:"username"`
		DisplayName string        `json:"display_name"`
		Email       string        `json:"email"`
		Grants      []users.Grant `json:"grants"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	pw := users.TemporaryPassword()
	u, err := d.users.Add(req.Username, req.DisplayName, req.Email, pw, req.Grants, true)
	if err != nil {
		d.auditEvent(r, "", "user.add", req.Username, false, err.Error())
		d.userError(w, r, err)
		return
	}
	d.auditEvent(r, "", "user.add", u.Username, true, grantsText(u.Grants))
	resp := map[string]any{"user": toUserJSON(u)}
	if u.Email != "" && d.emailConfigured() {
		err := d.sendAccountLink(r, u.Username, users.TicketInvite)
		if err == nil {
			resp["invited"] = true
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, resp)
			return
		}
		resp["email_error"] = err.Error()
	}
	resp["temporary_password"] = pw
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, resp)
}

func grantsText(gs []users.Grant) string {
	var parts []string
	for _, g := range gs {
		scope := "all agents"
		if g.Role != users.RoleAdmin && len(g.Agents) > 0 {
			scope = strings.Join(g.Agents, ",")
		}
		parts = append(parts, string(g.Role)+"@"+scope)
	}
	return strings.Join(parts, "; ")
}

func (d *Dashboard) handleAPIUpdateUser(w http.ResponseWriter, r *http.Request) {
	if !d.usersReady(w) {
		return
	}
	name := r.PathValue("name")
	var req struct {
		Grants      []users.Grant `json:"grants"`
		Disabled    *bool         `json:"disabled"`
		DisplayName *string       `json:"display_name"`
		Email       *string       `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Grants != nil {
		if err := d.users.SetGrants(name, req.Grants); err != nil {
			d.auditEvent(r, "", "user.roles", name, false, err.Error())
			d.userError(w, r, err)
			return
		}
		d.auditEvent(r, "", "user.roles", name, true, grantsText(req.Grants))
	}
	if req.DisplayName != nil {
		if err := d.users.SetDisplayName(name, *req.DisplayName); err != nil {
			d.userError(w, r, err)
			return
		}
		d.auditEvent(r, "", "user.profile", name, true, strings.TrimSpace(*req.DisplayName))
	}
	if req.Email != nil {
		if err := d.users.SetEmail(name, *req.Email); err != nil {
			d.auditEvent(r, "", "user.email", name, false, err.Error())
			d.userError(w, r, err)
			return
		}
		d.auditEvent(r, "", "user.email", name, true, strings.ToLower(strings.TrimSpace(*req.Email)))
	}
	if req.Disabled != nil {
		if err := d.users.SetDisabled(name, *req.Disabled); err != nil {
			d.auditEvent(r, "", "user.disable", name, false, err.Error())
			d.userError(w, r, err)
			return
		}
		action := "user.enable"
		if *req.Disabled {
			action = "user.disable"
		}
		d.auditEvent(r, "", action, name, true, "")
	}
	u, err := d.users.Get(name)
	if err != nil {
		d.userError(w, r, err)
		return
	}
	writeJSON(w, toUserJSON(u))
}

func (d *Dashboard) handleAPIRemoveUser(w http.ResponseWriter, r *http.Request) {
	if !d.usersReady(w) {
		return
	}
	name := r.PathValue("name")
	if u := currentUser(r); u != nil && u.Username == strings.ToLower(name) {
		http.Error(w, "you cannot remove your own account", http.StatusConflict)
		return
	}
	if err := d.users.Remove(name); err != nil {
		d.auditEvent(r, "", "user.remove", name, false, err.Error())
		d.userError(w, r, err)
		return
	}
	d.auditEvent(r, "", "user.remove", name, true, "")
	writeJSON(w, map[string]any{"ok": true})
}

// handleAPIResetUserPassword sets a temporary password shown to the
// administrator or, with {"send_email": true}, emails the user a link to
// choose one (an invitation again if they never signed in). The current
// password keeps working until the link is used.
func (d *Dashboard) handleAPIResetUserPassword(w http.ResponseWriter, r *http.Request) {
	if !d.usersReady(w) {
		return
	}
	name := r.PathValue("name")
	var req struct {
		SendEmail bool `json:"send_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.SendEmail {
		d.emailUserLink(w, r, name)
		return
	}
	pw := users.TemporaryPassword()
	if err := d.users.SetPassword(name, pw, true, ""); err != nil {
		d.auditEvent(r, "", "user.password-reset", name, false, err.Error())
		d.userError(w, r, err)
		return
	}
	d.auditEvent(r, "", "user.password-reset", name, true, "")
	writeJSON(w, map[string]any{"temporary_password": pw})
}

// emailUserLink emails name a link to choose their password.
func (d *Dashboard) emailUserLink(w http.ResponseWriter, r *http.Request, name string) {
	u, err := d.users.Get(name)
	if err != nil {
		d.userError(w, r, err)
		return
	}
	c := d.catalog(r)
	switch {
	case !d.emailConfigured():
		http.Error(w, c.T("Email is not set up: an administrator sets it up in Settings."), http.StatusConflict)
		return
	case u.Email == "":
		http.Error(w, c.T("This user has no email address."), http.StatusConflict)
		return
	}
	purpose := users.TicketReset
	if u.LastLoginAt == nil {
		purpose = users.TicketInvite
	}
	if err := d.sendAccountLink(r, u.Username, purpose); err != nil {
		if errors.Is(err, users.ErrTooSoon) {
			d.userError(w, r, err)
			return
		}
		http.Error(w, c.Tf("The email could not be sent: {error}", "error", err.Error()), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"sent_to": u.Email, "purpose": purpose})
}

func (d *Dashboard) handleAPIResetUserSecondFactor(w http.ResponseWriter, r *http.Request) {
	if !d.usersReady(w) {
		return
	}
	name := r.PathValue("name")
	if err := d.users.ResetSecondFactor(name); err != nil {
		d.auditEvent(r, "", "user.second-factor-reset", name, false, err.Error())
		d.userError(w, r, err)
		return
	}
	d.auditEvent(r, "", "user.second-factor-reset", name, true, "")
	writeJSON(w, map[string]any{"ok": true})
}

func (d *Dashboard) handleAPIUserSessions(w http.ResponseWriter, r *http.Request) {
	if !d.usersReady(w) {
		return
	}
	writeJSON(w, d.sessionsJSON(r, r.PathValue("name")))
}

func (d *Dashboard) handleAPIEndUserSession(w http.ResponseWriter, r *http.Request) {
	if !d.usersReady(w) {
		return
	}
	name := r.PathValue("name")
	if err := d.users.EndSession(name, r.PathValue("id")); err != nil {
		d.userError(w, r, err)
		return
	}
	d.auditEvent(r, "", "user.session-end", name, true, r.PathValue("id"))
	writeJSON(w, map[string]any{"ok": true})
}

func (d *Dashboard) handleAuditPage(w http.ResponseWriter, r *http.Request) {
	var names []string
	if d.users != nil {
		for _, u := range d.users.List() {
			names = append(names, u.Username)
		}
	}
	data := d.newPageData("Audit log", "Who did what, and when", "audit", map[string]any{
		"Users":      names,
		"Categories": auditCategories,
	})
	d.render(w, r, "audit", data)
}
