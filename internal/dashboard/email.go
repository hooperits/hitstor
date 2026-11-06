package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"image"
	"image/color"
	_ "image/jpeg" // older icon.png files are JPEG inside
	"image/png"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/i18n"
	"github.com/hooperits/hitstor/internal/mailer"
	"github.com/hooperits/hitstor/internal/users"
)

// mailState keeps the mailer built from the email settings, so a
// Microsoft 365 token serves several emails.
type mailState struct {
	mu  sync.Mutex
	key string
	m   *mailer.Mailer
}

// emailConfigured reports whether the dashboard can send account emails.
func (d *Dashboard) emailConfigured() bool {
	return d.cfg != nil && d.cfg.Email.Configured()
}

// mailerConfig turns email settings into a mailer configuration.
func mailerConfig(e config.EmailConfig, s config.EmailSecrets) mailer.Config {
	return mailer.Config{Host: e.Host, Port: e.Port, Security: e.Security, Auth: e.Auth,
		Username: e.Username, Password: s.Password,
		TenantID: e.TenantID, ClientID: e.ClientID, ClientSecret: s.ClientSecret,
		From: e.From, FromName: e.FromName, TLSSkipVerify: e.TLSSkipVerify}
}

// mailer returns the mailer of the saved email settings.
func (d *Dashboard) mailer() (*mailer.Mailer, error) {
	if !d.emailConfigured() {
		return nil, errors.New("email is not set up")
	}
	encKey, hasKey := d.encryptionKey()
	secrets, err := config.ResolveEmailSecrets(d.cfg.Email, encKey, hasKey)
	if err != nil {
		return nil, err
	}
	mc := mailerConfig(d.cfg.Email, secrets)
	key := fmt.Sprintf("%#v", mc)
	d.mail.mu.Lock()
	defer d.mail.mu.Unlock()
	if d.mail.m != nil && d.mail.key == key {
		return d.mail.m, nil
	}
	m, err := mailer.New(mc)
	if err != nil {
		return nil, err
	}
	d.mail.m, d.mail.key = m, key
	return m, nil
}

func (d *Dashboard) sendEmail(ctx context.Context, msg mailer.Message) error {
	if d.sendMailHook != nil {
		return d.sendMailHook(ctx, msg)
	}
	m, err := d.mailer()
	if err != nil {
		return err
	}
	return m.Send(ctx, msg)
}

// --- Messages ---

// emailLogoCID is the Content-ID of the logo in emails' HTML.
const emailLogoCID = "logo@hitstor"

var (
	emailLogoOnce sync.Once
	emailLogoPNG  []byte
)

// emailLogo is the dashboard's brand icon at 128 pixels (shown at 40), made
// once from static/icon.png so emails follow the brand without a file of
// their own. Nil if it cannot be made: emails go without it.
func emailLogo() []byte {
	emailLogoOnce.Do(func() {
		f, err := staticFS.Open("static/icon.png")
		if err != nil {
			return
		}
		defer f.Close()
		src, _, err := image.Decode(f)
		if err != nil {
			return
		}
		var buf bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if enc.Encode(&buf, shrink(src, 128)) == nil {
			emailLogoPNG = buf.Bytes()
		}
	})
	return emailLogoPNG
}

// shrink scales img down to size×size, each pixel the average of the
// source pixels it covers (premultiplied, so transparent edges stay clean).
func shrink(img image.Image, size int) *image.RGBA {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		y0, y1 := b.Min.Y+y*b.Dy()/size, b.Min.Y+(y+1)*b.Dy()/size
		for x := 0; x < size; x++ {
			x0, x1 := b.Min.X+x*b.Dx()/size, b.Min.X+(x+1)*b.Dx()/size
			var r, g, bl, a, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, ca := img.At(sx, sy).RGBA()
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			if n > 0 {
				dst.SetRGBA(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(bl / n >> 8), uint8(a / n >> 8)})
			}
		}
	}
	return dst
}

// emailContent is what an email says; renderEmail lays it out as text
// and as HTML.
type emailContent struct {
	Logo       bool // the HTML shows the logo (set by renderEmail)
	Lang       string
	Subject    string
	Paragraphs []string
	Button     string
	Link       string
	LinkHint   string
	Footer     []string
}

var emailHTML = htmltemplate.Must(htmltemplate.New("email").Parse(`<!DOCTYPE html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>{{.Subject}}</title></head>
<body style="margin:0;padding:24px 12px;background:#f3f4f6;font-family:'Segoe UI',Arial,sans-serif;color:#1f2937">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;background:#ffffff;border-radius:8px;overflow:hidden">
<tr><td style="background:#071018;padding:16px 32px"><table role="presentation" cellpadding="0" cellspacing="0"><tr>
{{if .Logo}}<td style="padding-right:12px;vertical-align:middle"><img src="cid:logo@hitstor" width="40" height="40" alt="" style="display:block;border:0"></td>{{end}}
<td style="vertical-align:middle;font-size:26px;font-weight:800;letter-spacing:-.5px"><span style="color:#f4f7fb">hit</span><span style="color:#ff7a45">stor</span></td>
</tr></table></td></tr>
<tr><td style="padding:28px 32px 8px">
{{range .Paragraphs}}<p style="margin:0 0 14px;font-size:15px;line-height:1.55">{{.}}</p>{{end}}
{{if .Link}}<p style="margin:22px 0"><a href="{{.Link}}" style="background:#cf4f1f;color:#ffffff;text-decoration:none;padding:12px 22px;border-radius:6px;display:inline-block;font-size:15px;font-weight:600">{{.Button}}</a></p>
<p style="margin:0 0 18px;font-size:13px;line-height:1.5;color:#6b7280">{{.LinkHint}}<br><a href="{{.Link}}" style="color:#cf4f1f;word-break:break-all">{{.Link}}</a></p>{{end}}
</td></tr>
{{if .Footer}}<tr><td style="padding:0 32px 24px">{{range .Footer}}<p style="margin:12px 0 0;font-size:13px;line-height:1.5;color:#6b7280">{{.}}</p>{{end}}</td></tr>{{end}}
</table></td></tr></table></body></html>`))

func renderEmail(to string, e emailContent) (mailer.Message, error) {
	var inline []mailer.Inline
	if logo := emailLogo(); logo != nil {
		e.Logo = true
		inline = append(inline, mailer.Inline{CID: emailLogoCID, ContentType: "image/png", Name: "hitstor.png", Data: logo})
	}
	var html bytes.Buffer
	if err := emailHTML.Execute(&html, e); err != nil {
		return mailer.Message{}, err
	}
	var text strings.Builder
	for _, p := range e.Paragraphs {
		text.WriteString(p + "\n\n")
	}
	if e.Link != "" {
		text.WriteString(e.Button + ":\n" + e.Link + "\n\n")
	}
	for _, p := range e.Footer {
		text.WriteString(p + "\n\n")
	}
	return mailer.Message{To: []string{to}, Subject: e.Subject, Text: strings.TrimSpace(text.String()) + "\n", HTML: html.String(),
		Inline: inline}, nil
}

// accountLink is the address of a ticket's page.
func (d *Dashboard) accountLink(token string) string {
	return strings.TrimRight(d.cfg.Email.DashboardURL, "/") + "/login/reset?token=" + url.QueryEscape(token)
}

// accountEmail is the invitation or password-reset email of u.
func (d *Dashboard) accountEmail(c *i18n.Catalog, u users.User, purpose, token string) (mailer.Message, error) {
	name := u.DisplayName
	if name == "" {
		name = u.Username
	}
	e := emailContent{Lang: c.Code, Link: d.accountLink(token),
		LinkHint: c.T("If the button does not work, copy this address into your browser:")}
	if purpose == users.TicketInvite {
		e.Subject = c.T("Your HitStor account")
		e.Paragraphs = []string{
			c.Tf("Hello {name},", "name", name),
			c.Tf("An administrator created a HitStor account for you. Your user name is {user}.", "user", u.Username),
			c.Tf("Choose your password with the button below. The link works once and expires in {hours} hours.", "hours", int(users.InviteTTL.Hours())),
		}
		e.Button = c.T("Choose my password")
		e.Footer = []string{
			c.T("Then you set up two-step verification with an authenticator app on your phone, such as Microsoft Authenticator or Google Authenticator."),
			c.T("If you did not expect this email, you can ignore it."),
		}
	} else {
		e.Subject = c.T("Reset your HitStor password")
		e.Paragraphs = []string{
			c.Tf("Hello {name},", "name", name),
			c.Tf("Someone asked to reset the password of the HitStor account {user}.", "user", u.Username),
			c.Tf("Choose a new password with the button below. The link works once and expires in {minutes} minutes.", "minutes", int(users.ResetTTL.Minutes())),
		}
		e.Button = c.T("Choose a new password")
		if u.TOTPEnabled {
			e.Footer = append(e.Footer, c.T("You will also need a code from your authenticator app."))
		}
		e.Footer = append(e.Footer, c.T("If you did not ask for this, ignore this email: your password does not change."))
	}
	return renderEmail(u.Email, e)
}

// sendAccountLink emails name a new link to choose their password, as an
// administrator asked, and audits it.
func (d *Dashboard) sendAccountLink(r *http.Request, name, purpose string) error {
	action := "user.reset-link"
	if purpose == users.TicketInvite {
		action = "user.invite"
	}
	err := func() error {
		token, u, err := d.users.IssueTicket(name, purpose)
		if err != nil {
			return err
		}
		msg, err := d.accountEmail(d.catalog(r), u, purpose, token)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		return d.sendEmail(ctx, msg)
	}()
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	d.auditEvent(r, "", action, name, err == nil, detail)
	return err
}

// --- Forgot password and emailed links (before signing in) ---

func (d *Dashboard) handleForgotPage(w http.ResponseWriter, r *http.Request) {
	if d.authMode() != authUsers {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	d.renderLogin(w, r, loginPage{Mode: "forgot"})
}

// handleForgotSubmit emails a reset link. The answer is the same whether or
// not the account exists or has an address, and the email goes out after
// the answer, so neither the page nor its timing tells who has an account.
func (d *Dashboard) handleForgotSubmit(w http.ResponseWriter, r *http.Request) {
	if d.authMode() != authUsers {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if !d.emailConfigured() {
		d.renderLogin(w, r, loginPage{Mode: "forgot"})
		return
	}
	ip := clientIP(r)
	if d.resetLimits.blocked(ip) {
		http.Error(w, "Too many requests; try again later", http.StatusTooManyRequests)
		return
	}
	d.resetLimits.fail(ip) // every request counts
	c := d.catalog(r)
	page := loginPage{Mode: "forgot", Notice: c.T("If that user name or email address belongs to an account with an email address, it will receive a link to choose a new password in a few minutes. Check the spam folder too.")}
	login := strings.ToLower(strings.TrimSpace(r.FormValue("login")))
	if login == "" || len(login) > 254 {
		d.renderLogin(w, r, page)
		return
	}
	u, err := d.users.FindAccount(login)
	if err == nil && u.Email == "" {
		err = errors.New("the account has no email address")
	}
	var token string
	if err == nil {
		token, u, err = d.users.IssueTicket(u.Username, users.TicketReset)
	}
	var msg mailer.Message
	if err == nil {
		msg, err = d.accountEmail(c, u, users.TicketReset, token)
	}
	if err != nil {
		who := login
		if u.Username != "" {
			who = u.Username
		}
		d.auditEvent(r, who, "password.reset-request", "", false, err.Error())
		d.renderLogin(w, r, page)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		err := d.sendEmail(ctx, msg)
		detail := ""
		if err != nil {
			detail = err.Error()
			slog.Warn("dashboard: password reset email not sent", "user", u.Username, "error", err)
		}
		if d.audit != nil {
			d.audit.Append(users.AuditEvent{User: u.Username, IP: ip, Action: "password.reset-request", OK: err == nil, Detail: detail})
		}
	}()
	d.renderLogin(w, r, page)
}

// ticketPage is the page of an emailed link: who it is for and what it
// needs.
func (d *Dashboard) ticketPage(token string) (loginPage, bool) {
	purpose, u, err := d.users.Ticket(token)
	if err != nil {
		return loginPage{Mode: "link-invalid"}, false
	}
	return loginPage{Mode: "reset", Purpose: purpose, Username: u.Username, Token: token, NeedCode: u.TOTPEnabled}, true
}

func (d *Dashboard) handleResetPage(w http.ResponseWriter, r *http.Request) {
	// The token is in the address: keep it out of Referer headers.
	w.Header().Set("Referrer-Policy", "no-referrer")
	if d.authMode() != authUsers {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	page, _ := d.ticketPage(r.URL.Query().Get("token"))
	d.renderLogin(w, r, page)
}

func (d *Dashboard) handleResetSubmit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	if d.authMode() != authUsers {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	ip := clientIP(r)
	if d.loginLimits.blocked(ip) {
		http.Error(w, "Too many failed attempts; try again later", http.StatusTooManyRequests)
		return
	}
	page, ok := d.ticketPage(r.FormValue("token"))
	if !ok {
		d.loginLimits.fail(ip)
		d.renderLogin(w, r, page)
		return
	}
	action := "password.reset"
	if page.Purpose == users.TicketInvite {
		action = "account.activate"
	}
	password := r.FormValue("password")
	if password != r.FormValue("confirm") {
		page.Error = d.catalog(r).T("The passwords do not match.")
		d.renderLogin(w, r, page)
		return
	}
	u, err := d.users.RedeemTicket(page.Token, password, r.FormValue("code"))
	if err != nil {
		d.auditEvent(r, page.Username, action, "", false, err.Error())
		switch {
		case errors.Is(err, users.ErrBadTicket):
			d.renderLogin(w, r, loginPage{Mode: "link-invalid"})
			return
		case errors.Is(err, users.ErrBadCode), errors.Is(err, users.ErrLocked):
			d.loginLimits.fail(ip)
		}
		page.Error = d.userMessage(r, err)
		d.renderLogin(w, r, page)
		return
	}
	d.auditEvent(r, u.Username, action, "", true, "")
	if !u.TOTPEnabled {
		// The password was just chosen: go on to set up two-step verification.
		if token, _, err := d.users.Login(u.Username, password, ip, r.UserAgent()); err == nil {
			setSessionCookie(w, r, token)
			http.Redirect(w, r, "/login/enroll", http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, "/login?changed=1", http.StatusSeeOther)
}

// --- Settings ---

// EmailSettingsView is the email section of GET /api/settings: never the
// secrets, only whether they are set.
type EmailSettingsView struct {
	Configured      bool   `json:"configured"`
	Host            string `json:"host"`
	Port            int    `json:"port,omitempty"`
	Security        string `json:"security"`
	Auth            string `json:"auth"`
	Username        string `json:"username,omitempty"`
	PasswordSet     bool   `json:"password_set"`
	TenantID        string `json:"tenant_id,omitempty"`
	ClientID        string `json:"client_id,omitempty"`
	ClientSecretSet bool   `json:"client_secret_set"`
	From            string `json:"from"`
	FromName        string `json:"from_name,omitempty"`
	TLSSkipVerify   bool   `json:"tls_skip_verify"`
	DashboardURL    string `json:"dashboard_url"`
}

func emailSettingsView(e config.EmailConfig) EmailSettingsView {
	v := EmailSettingsView{Configured: e.Configured(), Host: e.Host, Port: e.Port, Security: e.Security, Auth: e.Auth,
		Username: e.Username, PasswordSet: e.PasswordEncrypted != "", TenantID: e.TenantID, ClientID: e.ClientID,
		ClientSecretSet: e.ClientSecretEncrypted != "", From: e.From, FromName: e.FromName, TLSSkipVerify: e.TLSSkipVerify,
		DashboardURL: e.DashboardURL}
	if v.Security == "" {
		v.Security = mailer.SecuritySTARTTLS
	}
	if v.Auth == "" {
		v.Auth = mailer.AuthNone
	}
	return v
}

// EmailSettingsUpdate replaces the email settings. Empty secrets keep the
// saved ones; an empty host turns email off.
type EmailSettingsUpdate struct {
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Security      string `json:"security"`
	Auth          string `json:"auth"`
	Username      string `json:"username"`
	Password      string `json:"password,omitempty"`
	TenantID      string `json:"tenant_id"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret,omitempty"`
	From          string `json:"from"`
	FromName      string `json:"from_name"`
	TLSSkipVerify bool   `json:"tls_skip_verify"`
	DashboardURL  string `json:"dashboard_url"`
}

// emailFromUpdate checks req and returns the settings to save (secrets
// encrypted) and the mailer configuration they make.
func (d *Dashboard) emailFromUpdate(req EmailSettingsUpdate) (config.EmailConfig, mailer.Config, error) {
	trim := strings.TrimSpace
	e := config.EmailConfig{Host: trim(req.Host), Port: req.Port, Security: req.Security, Auth: req.Auth,
		From: trim(req.From), FromName: trim(req.FromName), TLSSkipVerify: req.TLSSkipVerify,
		DashboardURL: strings.TrimRight(trim(req.DashboardURL), "/")}
	if e.Host == "" {
		return config.EmailConfig{}, mailer.Config{}, nil
	}
	if e.Port == 0 {
		e.Port = mailer.DefaultPort(e.Security)
	}
	if err := checkDashboardURL(e.DashboardURL); err != nil {
		return e, mailer.Config{}, err
	}
	encKey, hasKey := d.encryptionKey()
	saved, err := config.ResolveEmailSecrets(d.cfg.Email, encKey, hasKey)
	if err != nil {
		return e, mailer.Config{}, err
	}
	var secrets config.EmailSecrets
	switch e.Auth {
	case mailer.AuthPassword:
		e.Username = trim(req.Username)
		secrets.Password = firstNonEmpty(req.Password, saved.Password)
	case mailer.AuthMicrosoft365:
		e.Username = trim(req.Username)
		e.TenantID, e.ClientID = trim(req.TenantID), trim(req.ClientID)
		secrets.ClientSecret = firstNonEmpty(trim(req.ClientSecret), saved.ClientSecret)
	}
	mc := mailerConfig(e, secrets)
	if _, err := mailer.New(mc); err != nil {
		return e, mc, err
	}
	for _, s := range []struct {
		plain string
		dst   *string
	}{{secrets.Password, &e.PasswordEncrypted}, {secrets.ClientSecret, &e.ClientSecretEncrypted}} {
		if s.plain == "" {
			continue
		}
		if !hasKey {
			return e, mc, errors.New("no encryption key available to store the secret (open a repository first)")
		}
		if *s.dst, err = config.EncryptCredential(encKey, s.plain); err != nil {
			return e, mc, err
		}
	}
	return e, mc, nil
}

// checkDashboardURL checks the base of the links in emails.
func checkDashboardURL(s string) error {
	u, err := url.Parse(s)
	if s == "" || err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" ||
		u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return errors.New("the dashboard address must be a URL such as https://backup.example.com:8421")
	}
	return nil
}

func (d *Dashboard) handleAPISettingsEmail(w http.ResponseWriter, r *http.Request) {
	cfg, err := d.requireConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var req EmailSettingsUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	e, _, err := d.emailFromUpdate(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg.Email = e
	if err := d.saveConfig(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, settingsUpdateResponse{OK: true})
}

// handleAPISettingsEmailTest sends a test email with the settings in the
// form, saved or not, to an address (by default the user's own).
func (d *Dashboard) handleAPISettingsEmailTest(w http.ResponseWriter, r *http.Request) {
	if _, err := d.requireConfig(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var req struct {
		EmailSettingsUpdate
		To string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	c := d.catalog(r)
	to := strings.TrimSpace(req.To)
	if u := currentUser(r); to == "" && u != nil {
		to = u.Email
	}
	if to == "" {
		writeJSON(w, settingsTestResponse{OK: false, Error: c.T("Enter the address to send the test email to.")})
		return
	}
	e, mc, err := d.emailFromUpdate(req.EmailSettingsUpdate)
	if err == nil && e.Host == "" {
		err = errors.New(c.T("Enter the mail server first."))
	}
	var m *mailer.Mailer
	if err == nil {
		m, err = mailer.New(mc)
	}
	var msg mailer.Message
	if err == nil {
		msg, err = renderEmail(to, emailContent{Lang: c.Code, Subject: c.T("HitStor test email"),
			Paragraphs: []string{
				c.T("This is a test email from HitStor."),
				c.T("If you received it, the dashboard can send invitations and password reset links."),
			},
			Footer: []string{c.Tf("Sent through {server}.", "server", e.Host+":"+strconv.Itoa(e.Port))}})
	}
	if err == nil {
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		if d.sendMailHook != nil {
			err = d.sendMailHook(ctx, msg)
		} else {
			err = m.Send(ctx, msg)
		}
	}
	if err != nil {
		writeJSON(w, settingsTestResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "to": to})
}
