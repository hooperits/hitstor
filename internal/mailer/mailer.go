// Package mailer sends email through an SMTP server: an internal relay
// that accepts mail by address, a server that takes a user name and
// password, or Microsoft 365 (Exchange Online) with an OAuth 2.0 app
// token, since Exchange Online no longer accepts passwords for SMTP.
package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Connection security.
const (
	SecurityNone     = "none"     // plain SMTP, for relays on a trusted network
	SecuritySTARTTLS = "starttls" // upgrade to TLS after connecting (port 587)
	SecurityTLS      = "tls"      // TLS from the start (port 465)
)

// Authentication.
const (
	AuthNone         = "none"         // the server accepts mail by client address
	AuthPassword     = "password"     // user name and password (PLAIN or LOGIN)
	AuthMicrosoft365 = "microsoft365" // OAuth 2.0 client credentials (XOAUTH2)
)

// Microsoft365Host is Exchange Online's SMTP submission endpoint.
const Microsoft365Host = "smtp.office365.com"

const defaultTimeout = 30 * time.Second

// Config is how to reach the mail server and who the mail comes from.
type Config struct {
	Host     string
	Port     int
	Security string
	Auth     string

	// AuthPassword.
	Username string
	Password string

	// AuthMicrosoft365: an app registered in Microsoft Entra ID with the
	// SMTP.SendAsApp permission, allowed to send as From. Username, when
	// set, is the mailbox to sign in as instead of From.
	TenantID     string
	ClientID     string
	ClientSecret string

	From     string // sender address
	FromName string // sender name, optional

	// TLSSkipVerify accepts any server certificate (self-signed relays).
	TLSSkipVerify bool
	Timeout       time.Duration

	// tokenURL replaces the Microsoft identity platform's token endpoint.
	// Tests only.
	tokenURL string
}

// Message is one email.
type Message struct {
	To      []string
	Subject string
	Text    string // plain-text body
	HTML    string // HTML body, optional
	// Inline are images the HTML shows with src="cid:<CID>". Mail clients
	// block remote images by default, but show these.
	Inline []Inline
}

// Inline is an image sent inside the message.
type Inline struct {
	CID         string // Content-ID, without angle brackets
	ContentType string // image/png
	Name        string // file name
	Data        []byte
}

// Mailer sends messages with one configuration.
type Mailer struct {
	cfg    Config
	tokens *tokenSource
}

// New checks cfg and returns a mailer for it.
func New(cfg Config) (*Mailer, error) {
	cfg.Host = strings.TrimSpace(cfg.Host)
	if cfg.Security == "" {
		cfg.Security = SecuritySTARTTLS
	}
	if cfg.Auth == "" {
		cfg.Auth = AuthNone
	}
	if cfg.Port == 0 {
		cfg.Port = DefaultPort(cfg.Security)
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	m := &Mailer{cfg: cfg}
	if cfg.Auth == AuthMicrosoft365 {
		m.tokens = newTokenSource(cfg.TenantID, cfg.ClientID, cfg.ClientSecret, cfg.tokenURL)
	}
	return m, nil
}

// DefaultPort is the usual port of a connection security.
func DefaultPort(security string) int {
	switch security {
	case SecurityTLS:
		return 465
	case SecurityNone:
		return 25
	}
	return 587
}

func (c Config) validate() error {
	if c.Host == "" {
		return errors.New("mail server host is required")
	}
	if strings.ContainsAny(c.Host, " /\r\n") {
		return fmt.Errorf("invalid mail server host %q", c.Host)
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid mail server port %d", c.Port)
	}
	if !slices.Contains([]string{SecurityNone, SecuritySTARTTLS, SecurityTLS}, c.Security) {
		return fmt.Errorf("unknown connection security %q", c.Security)
	}
	switch c.Auth {
	case AuthNone:
	case AuthPassword:
		if c.Username == "" || c.Password == "" {
			return errors.New("user name and password are required")
		}
	case AuthMicrosoft365:
		if c.TenantID == "" || c.ClientID == "" || c.ClientSecret == "" {
			return errors.New("tenant ID, client ID and client secret are required for Microsoft 365")
		}
	default:
		return fmt.Errorf("unknown authentication %q", c.Auth)
	}
	if c.Auth != AuthNone && c.Security == SecurityNone {
		return errors.New("credentials need an encrypted connection: use STARTTLS or TLS")
	}
	if _, err := ParseAddress(c.From); err != nil {
		return fmt.Errorf("sender: %w", err)
	}
	if strings.ContainsAny(c.FromName, "\r\n") {
		return errors.New("invalid sender name")
	}
	return nil
}

// ParseAddress checks a bare email address (no display name).
func ParseAddress(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("email address is required")
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || a.Name != "" {
		return "", fmt.Errorf("invalid email address %q", s)
	}
	return s, nil
}

// Send delivers msg. The error carries the server's reply, which says why
// it refused the mail.
func (m *Mailer) Send(ctx context.Context, msg Message) error {
	if len(msg.To) == 0 {
		return errors.New("no recipients")
	}
	rcpts := make([]string, len(msg.To))
	for i, to := range msg.To {
		a, err := ParseAddress(to)
		if err != nil {
			return err
		}
		rcpts[i] = a
	}
	msg.To = rcpts
	data, err := buildMessage(m.cfg.From, m.cfg.FromName, msg, time.Now())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, m.cfg.Timeout)
	defer cancel()

	c, err := m.dial(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := m.authenticate(ctx, c); err != nil {
		return err
	}
	if err := c.Mail(m.cfg.From); err != nil {
		return fmt.Errorf("sender %s refused: %w", m.cfg.From, err)
	}
	for _, to := range msg.To {
		if err := c.Rcpt(to); err != nil {
			return fmt.Errorf("recipient %s refused: %w", to, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("start message: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("message refused: %w", err)
	}
	return c.Quit()
}

func (m *Mailer) tlsConfig() *tls.Config {
	return &tls.Config{ServerName: m.cfg.Host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: m.cfg.TLSSkipVerify}
}

// dial connects and greets the server, with TLS as configured.
func (m *Mailer) dial(ctx context.Context) (*smtp.Client, error) {
	addr := net.JoinHostPort(m.cfg.Host, strconv.Itoa(m.cfg.Port))
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", addr, err)
	}
	if dl, ok := ctx.Deadline(); ok {
		conn.SetDeadline(dl)
	}
	if m.cfg.Security == SecurityTLS {
		tc := tls.Client(conn, m.tlsConfig())
		if err := tc.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, fmt.Errorf("TLS with %s: %w", addr, err)
		}
		conn = tc
	}
	c, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("greeting from %s: %w", addr, err)
	}
	if err := c.Hello(helloName()); err != nil {
		c.Close()
		return nil, fmt.Errorf("EHLO: %w", err)
	}
	if m.cfg.Security == SecuritySTARTTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			c.Close()
			return nil, fmt.Errorf("%s does not offer STARTTLS: choose another connection security", addr)
		}
		if err := c.StartTLS(m.tlsConfig()); err != nil {
			c.Close()
			return nil, fmt.Errorf("STARTTLS with %s: %w", addr, err)
		}
	}
	return c, nil
}

// helloName is the name this host gives in EHLO.
func helloName() string {
	if h, err := os.Hostname(); err == nil && h != "" && !strings.ContainsAny(h, " \r\n") {
		return h
	}
	return "localhost"
}

func (m *Mailer) authenticate(ctx context.Context, c *smtp.Client) error {
	var a smtp.Auth
	switch m.cfg.Auth {
	case AuthNone:
		return nil
	case AuthPassword:
		a = &passwordAuth{username: m.cfg.Username, password: m.cfg.Password}
	case AuthMicrosoft365:
		token, err := m.tokens.token(ctx)
		if err != nil {
			return err
		}
		user := m.cfg.Username
		if user == "" {
			user = m.cfg.From
		}
		a = &xoauth2{username: user, token: token}
	}
	if ok, _ := c.Extension("AUTH"); !ok {
		return errors.New("the mail server does not offer authentication: set authentication to none")
	}
	if err := c.Auth(a); err != nil {
		if m.tokens != nil {
			m.tokens.reset() // a revoked token: get a new one next time
		}
		return fmt.Errorf("authentication failed: %w", err)
	}
	return nil
}

// passwordAuth is PLAIN when the server offers it, else LOGIN (which
// Exchange offers alone). Like smtp.PlainAuth, it refuses to send the
// password over a connection without TLS.
type passwordAuth struct {
	username, password string
	login              bool
}

func (a *passwordAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS {
		return "", nil, errors.New("unencrypted connection")
	}
	if slices.Contains(server.Auth, "PLAIN") {
		return "PLAIN", []byte("\x00" + a.username + "\x00" + a.password), nil
	}
	if slices.Contains(server.Auth, "LOGIN") {
		a.login = true
		return "LOGIN", nil, nil
	}
	return "", nil, fmt.Errorf("the server offers neither PLAIN nor LOGIN (it offers %s)", strings.Join(server.Auth, ", "))
}

func (a *passwordAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	if !a.login {
		return nil, errors.New("unexpected server challenge")
	}
	switch strings.ToLower(strings.TrimSpace(string(fromServer))) {
	case "username:":
		return []byte(a.username), nil
	case "password:":
		return []byte(a.password), nil
	}
	return nil, fmt.Errorf("unexpected server challenge %q", fromServer)
}

// xoauth2 signs in with an OAuth 2.0 access token.
type xoauth2 struct{ username, token string }

func (a *xoauth2) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS {
		return "", nil, errors.New("unencrypted connection")
	}
	return "XOAUTH2", []byte("user=" + a.username + "\x01auth=Bearer " + a.token + "\x01\x01"), nil
}

// Next answers the server's error details with an empty line, after which
// the server sends its final error reply.
func (a *xoauth2) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		return []byte{}, nil
	}
	return nil, nil
}
