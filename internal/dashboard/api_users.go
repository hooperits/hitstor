package dashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/users"
)

// userJSON is an account as the dashboard shows it: never its secrets.
type userJSON struct {
	Username      string        `json:"username"`
	DisplayName   string        `json:"display_name,omitempty"`
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
	return userJSON{Username: u.Username, DisplayName: u.DisplayName, Grants: u.Grants, Disabled: u.Disabled,
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
	}
	return err.Error()
}

func (d *Dashboard) userError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, users.ErrNoUser):
		status = http.StatusNotFound
	case errors.Is(err, users.ErrUserExists), errors.Is(err, users.ErrLastAdmin):
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
	data := d.newPageData("My account", "Password, second factor and sessions", "account", map[string]any{
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
	Current   bool      `json:"current"`
}

func (d *Dashboard) sessionsJSON(r *http.Request, name string) []sessionJSON {
	cur := ""
	if a := authOf(r); a != nil {
		cur = a.session.ID
	}
	out := []sessionJSON{}
	for _, s := range d.users.Sessions(name) {
		out = append(out, sessionJSON{ID: s.ID, Created: s.Created, LastSeen: s.LastSeen, IP: s.IP, UserAgent: s.UserAgent, Current: s.ID == cur})
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

// handleAPIReauth confirms the signed-in user before a sensitive action.
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
	data := d.newPageData("Users", "Accounts, roles and the agents they cover", "users", map[string]any{
		"Agents": agentNames,
		"Roles":  users.Roles,
	})
	d.render(w, r, "users", data)
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

// temporaryPassword is shown once to the administrator, who passes it on;
// the user must change it at first sign-in.
func (d *Dashboard) handleAPIAddUser(w http.ResponseWriter, r *http.Request) {
	if !d.usersReady(w) {
		return
	}
	var req struct {
		Username    string        `json:"username"`
		DisplayName string        `json:"display_name"`
		Grants      []users.Grant `json:"grants"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	pw := users.TemporaryPassword()
	u, err := d.users.Add(req.Username, req.DisplayName, pw, req.Grants, true)
	if err != nil {
		d.auditEvent(r, "", "user.add", req.Username, false, err.Error())
		d.userError(w, r, err)
		return
	}
	d.auditEvent(r, "", "user.add", u.Username, true, grantsText(u.Grants))
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"user": toUserJSON(u), "temporary_password": pw})
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
		Grants   []users.Grant `json:"grants"`
		Disabled *bool         `json:"disabled"`
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

func (d *Dashboard) handleAPIResetUserPassword(w http.ResponseWriter, r *http.Request) {
	if !d.usersReady(w) {
		return
	}
	name := r.PathValue("name")
	pw := users.TemporaryPassword()
	if err := d.users.SetPassword(name, pw, true, ""); err != nil {
		d.auditEvent(r, "", "user.password-reset", name, false, err.Error())
		d.userError(w, r, err)
		return
	}
	d.auditEvent(r, "", "user.password-reset", name, true, "")
	writeJSON(w, map[string]any{"temporary_password": pw})
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
	events, brokenAt, err := d.audit.Recent(500)
	data := d.newPageData("Audit log", "Sign-ins, changes and denied requests", "audit", map[string]any{
		"Events":   events,
		"BrokenAt": brokenAt,
		"Err":      err,
	})
	d.render(w, r, "audit", data)
}
