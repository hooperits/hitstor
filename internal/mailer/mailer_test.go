package mailer

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"io"
	"math/big"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTP is a minimal SMTP server that records what it receives.
type fakeSMTP struct {
	t        *testing.T
	ln       net.Listener
	tls      *tls.Config // offered with STARTTLS, or from the start with implicit
	implicit bool
	auth     []string // mechanisms offered after TLS
	accept   func(mech, initial string, next func(challenge string) string) bool

	mu       sync.Mutex
	from     string
	rcpts    []string
	data     string
	authed   string
	starttls bool
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSMTP{t: t, ln: ln}
	t.Cleanup(func() { ln.Close() })
	return s
}

func (s *fakeSMTP) start() {
	ln := s.ln
	if s.implicit {
		ln = tls.NewListener(ln, s.tls)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
}

// got is what the server received so far.
type got struct {
	from, data, authed string
	rcpts              []string
	starttls           bool
}

func (s *fakeSMTP) got() got {
	s.mu.Lock()
	defer s.mu.Unlock()
	return got{from: s.from, data: s.data, authed: s.authed, rcpts: append([]string(nil), s.rcpts...), starttls: s.starttls}
}

func (s *fakeSMTP) port() int { return s.ln.Addr().(*net.TCPAddr).Port }

func (s *fakeSMTP) serve(c net.Conn) {
	defer c.Close()
	r, w := bufio.NewReader(c), bufio.NewWriter(c)
	say := func(line string) { w.WriteString(line + "\r\n"); w.Flush() }
	read := func() string { l, _ := r.ReadString('\n'); return strings.TrimRight(l, "\r\n") }
	_, isTLS := c.(*tls.Conn)
	say("220 fake ESMTP")
	for {
		line := read()
		cmd := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
		switch cmd {
		case "EHLO":
			say("250-fake")
			if s.tls != nil && !isTLS {
				say("250-STARTTLS")
			}
			if isTLS && len(s.auth) > 0 {
				say("250-AUTH " + strings.Join(s.auth, " "))
			}
			say("250 8BITMIME")
		case "STARTTLS":
			say("220 go ahead")
			tc := tls.Server(c, s.tls)
			if tc.Handshake() != nil {
				return
			}
			c, isTLS = tc, true
			r, w = bufio.NewReader(c), bufio.NewWriter(c)
			s.mu.Lock()
			s.starttls = true
			s.mu.Unlock()
		case "AUTH":
			parts := strings.SplitN(line, " ", 3)
			initial := ""
			if len(parts) == 3 {
				b, _ := base64.StdEncoding.DecodeString(parts[2])
				initial = string(b)
			}
			next := func(challenge string) string {
				say("334 " + base64.StdEncoding.EncodeToString([]byte(challenge)))
				b, _ := base64.StdEncoding.DecodeString(read())
				return string(b)
			}
			if s.accept(parts[1], initial, next) {
				s.mu.Lock()
				s.authed = parts[1]
				s.mu.Unlock()
				say("235 2.7.0 Authentication successful")
			} else {
				say("535 5.7.3 Authentication unsuccessful")
			}
		case "MAIL":
			s.mu.Lock()
			s.from = angle(line)
			s.mu.Unlock()
			say("250 OK")
		case "RCPT":
			to := angle(line)
			if strings.HasPrefix(to, "nobody@") {
				say("550 5.7.54 Unable to relay recipient in non-accepted domain")
				continue
			}
			s.mu.Lock()
			s.rcpts = append(s.rcpts, to)
			s.mu.Unlock()
			say("250 OK")
		case "DATA":
			say("354 go ahead")
			var b strings.Builder
			for {
				l := read()
				if l == "." {
					break
				}
				b.WriteString(strings.TrimPrefix(l, ".") + "\r\n")
			}
			s.mu.Lock()
			s.data = b.String()
			s.mu.Unlock()
			say("250 queued")
		case "QUIT":
			say("221 bye")
			return
		case "":
			return
		default:
			say("250 OK")
		}
	}
}

// angle returns the <address> of a MAIL or RCPT command.
func angle(line string) string {
	_, rest, _ := strings.Cut(line, "<")
	addr, _, _ := strings.Cut(rest, ">")
	return addr
}

func testTLSConfig(t *testing.T) *tls.Config {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "127.0.0.1"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
}

func testMessage() Message {
	return Message{To: []string{"ana@example.com"}, Subject: "Restablecer contraseña", Text: "Hola Ana,\nenlace: https://x/y", HTML: "<p>Hola <b>Ana</b></p>"}
}

func TestSendThroughRelayWithoutAuth(t *testing.T) {
	s := newFakeSMTP(t)
	s.start()
	m, err := New(Config{Host: "127.0.0.1", Port: s.port(), Security: SecurityNone, From: "hitstor@example.com", FromName: "HitStor"})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Send(context.Background(), testMessage()); err != nil {
		t.Fatal(err)
	}
	g := s.got()
	if g.from != "hitstor@example.com" || len(g.rcpts) != 1 || g.rcpts[0] != "ana@example.com" {
		t.Fatalf("envelope: from %q to %v", g.from, g.rcpts)
	}
	msg, err := mail.ReadMessage(strings.NewReader(g.data))
	if err != nil {
		t.Fatal(err)
	}
	dec := new(mime.WordDecoder)
	if subj, _ := dec.DecodeHeader(msg.Header.Get("Subject")); subj != "Restablecer contraseña" {
		t.Errorf("subject %q", subj)
	}
	if got := msg.Header.Get("From"); got != `"HitStor" <hitstor@example.com>` {
		t.Errorf("from header %q", got)
	}
	if msg.Header.Get("Auto-Submitted") != "auto-generated" || msg.Header.Get("Message-ID") == "" {
		t.Errorf("headers %v", msg.Header)
	}
	_, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	mr := multipart.NewReader(msg.Body, params["boundary"])
	var bodies []string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(p) // multipart decodes quoted-printable
		bodies = append(bodies, string(b))
	}
	if len(bodies) != 2 || !strings.Contains(bodies[0], "Hola Ana,\r\nenlace") || !strings.Contains(bodies[1], "<b>Ana</b>") {
		t.Errorf("bodies %q", bodies)
	}
}

func TestRelayRefusesRecipient(t *testing.T) {
	s := newFakeSMTP(t)
	s.start()
	m, _ := New(Config{Host: "127.0.0.1", Port: s.port(), Security: SecurityNone, From: "hitstor@example.com"})
	err := m.Send(context.Background(), Message{To: []string{"nobody@example.org"}, Subject: "x", Text: "x"})
	if err == nil || !strings.Contains(err.Error(), "Unable to relay") {
		t.Fatalf("want the server's refusal, got %v", err)
	}
}

func TestSendWithPasswordOverSTARTTLS(t *testing.T) {
	for _, mech := range []string{"PLAIN", "LOGIN"} {
		t.Run(mech, func(t *testing.T) {
			s := newFakeSMTP(t)
			s.tls = testTLSConfig(t)
			s.auth = []string{mech}
			s.accept = func(m, initial string, next func(string) string) bool {
				switch m {
				case "PLAIN":
					return initial == "\x00ana\x00secreto"
				case "LOGIN":
					return next("Username:") == "ana" && next("Password:") == "secreto"
				}
				return false
			}
			s.start()
			m, err := New(Config{Host: "127.0.0.1", Port: s.port(), Security: SecuritySTARTTLS, Auth: AuthPassword,
				Username: "ana", Password: "secreto", From: "ana@example.com", TLSSkipVerify: true})
			if err != nil {
				t.Fatal(err)
			}
			if err := m.Send(context.Background(), testMessage()); err != nil {
				t.Fatal(err)
			}
			if g := s.got(); !g.starttls || g.authed != mech {
				t.Fatalf("starttls %v, auth %q", g.starttls, g.authed)
			}
		})
	}
}

func TestSTARTTLSChecksTheCertificate(t *testing.T) {
	s := newFakeSMTP(t)
	s.tls = testTLSConfig(t)
	s.start()
	m, _ := New(Config{Host: "127.0.0.1", Port: s.port(), Security: SecuritySTARTTLS, From: "a@example.com"})
	if err := m.Send(context.Background(), testMessage()); err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("self-signed certificate accepted: %v", err)
	}
}

func TestSTARTTLSRequired(t *testing.T) {
	s := newFakeSMTP(t) // offers no STARTTLS
	s.start()
	m, _ := New(Config{Host: "127.0.0.1", Port: s.port(), Security: SecuritySTARTTLS, From: "a@example.com"})
	if err := m.Send(context.Background(), testMessage()); err == nil || !strings.Contains(err.Error(), "does not offer STARTTLS") {
		t.Fatalf("got %v", err)
	}
}

func TestImplicitTLS(t *testing.T) {
	s := newFakeSMTP(t)
	s.tls = testTLSConfig(t)
	s.implicit = true
	s.start()
	m, _ := New(Config{Host: "127.0.0.1", Port: s.port(), Security: SecurityTLS, From: "a@example.com", TLSSkipVerify: true})
	if err := m.Send(context.Background(), testMessage()); err != nil {
		t.Fatal(err)
	}
	if g := s.got(); len(g.rcpts) != 1 {
		t.Fatalf("rcpts %v", g.rcpts)
	}
}

func TestMicrosoft365XOAUTH2(t *testing.T) {
	var tokenRequests int
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		tokenRequests++
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != "app-id" ||
			r.Form.Get("client_secret") != "app-secret" || r.Form.Get("scope") != exchangeScope {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"invalid_client","error_description":"AADSTS7000215: Invalid client secret provided.\r\nTrace ID: x"}`))
			return
		}
		w.Write([]byte(`{"access_token":"tok-` + strconv.Itoa(tokenRequests) + `","expires_in":3599,"token_type":"Bearer"}`))
	}))
	defer idp.Close()

	s := newFakeSMTP(t)
	s.tls = testTLSConfig(t)
	s.auth = []string{"LOGIN", "XOAUTH2"}
	s.accept = func(m, initial string, next func(string) string) bool {
		if m == "XOAUTH2" && initial == "user=hitstor@example.com\x01auth=Bearer tok-1\x01\x01" {
			return true
		}
		next(`{"status":"535"}`)
		return false
	}
	s.start()
	cfg := Config{Host: "127.0.0.1", Port: s.port(), Security: SecuritySTARTTLS, Auth: AuthMicrosoft365, TenantID: "tenant",
		ClientID: "app-id", ClientSecret: "app-secret", From: "hitstor@example.com", TLSSkipVerify: true, tokenURL: idp.URL}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // the second send reuses the token
		if err := m.Send(context.Background(), testMessage()); err != nil {
			t.Fatal(err)
		}
	}
	if g := s.got(); g.authed != "XOAUTH2" || tokenRequests != 1 {
		t.Fatalf("auth %q, token requests %d", g.authed, tokenRequests)
	}

	cfg.ClientSecret = "wrong"
	m, _ = New(cfg)
	err = m.Send(context.Background(), testMessage())
	if err == nil || !strings.Contains(err.Error(), "AADSTS7000215: Invalid client secret provided.") || strings.Contains(err.Error(), "Trace ID") {
		t.Fatalf("want the identity platform's reason, got %v", err)
	}
}

func TestConfigValidation(t *testing.T) {
	base := Config{Host: "relay.example.com", From: "a@example.com"}
	for name, mod := range map[string]func(*Config){
		"no host":              func(c *Config) { c.Host = "" },
		"bad sender":           func(c *Config) { c.From = "Ana <a@example.com>" },
		"password over plain":  func(c *Config) { c.Security, c.Auth, c.Username, c.Password = SecurityNone, AuthPassword, "a", "b" },
		"missing password":     func(c *Config) { c.Auth, c.Username = AuthPassword, "a" },
		"missing app secret":   func(c *Config) { c.Auth, c.TenantID, c.ClientID = AuthMicrosoft365, "t", "c" },
		"unknown security":     func(c *Config) { c.Security = "ssl" },
		"header in from name":  func(c *Config) { c.FromName = "x\r\nBcc: y@example.com" },
		"port out of range":    func(c *Config) { c.Port = 70000 },
		"unknown auth":         func(c *Config) { c.Auth = "ntlm" },
		"host with whitespace": func(c *Config) { c.Host = "relay example" },
	} {
		c := base
		mod(&c)
		if _, err := New(c); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if m, err := New(base); err != nil || m.cfg.Port != 587 || m.cfg.Security != SecuritySTARTTLS {
		t.Fatalf("defaults: %+v %v", m, err)
	}
	m, _ := New(base)
	if err := m.Send(context.Background(), Message{To: []string{"a@example.com\r\nRCPT TO:<x@example.com>"}, Subject: "x"}); err == nil {
		t.Error("recipient with a line break accepted")
	}
	if err := m.Send(context.Background(), Message{To: []string{"a@example.com"}, Subject: "x\r\nBcc: y@example.com"}); err == nil {
		t.Error("subject with a line break accepted")
	}
}

func TestInlineImages(t *testing.T) {
	img := bytes.Repeat([]byte{0x89, 'P', 'N', 'G', 0, 1, 2, 3}, 40)
	msg := testMessage()
	msg.HTML = `<img src="cid:logo@hitstor"><p>Hola</p>`
	msg.Inline = []Inline{{CID: "logo@hitstor", ContentType: "image/png", Name: "hitstor.png", Data: img}}
	raw, err := buildMessage("a@example.com", "", msg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	_, params, _ := mime.ParseMediaType(m.Header.Get("Content-Type"))
	alt := multipart.NewReader(m.Body, params["boundary"])
	if p, _ := alt.NextPart(); !strings.HasPrefix(p.Header.Get("Content-Type"), "text/plain") {
		t.Fatalf("first part %q", p.Header.Get("Content-Type"))
	}
	p, err := alt.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	mt, params, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
	if mt != "multipart/related" {
		t.Fatalf("second part %q", mt)
	}
	rel := multipart.NewReader(p, params["boundary"])
	html, _ := rel.NextPart()
	if body, _ := io.ReadAll(html); !strings.Contains(string(body), "cid:logo@hitstor") {
		t.Fatalf("html %q", body)
	}
	ip, err := rel.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if ip.Header.Get("Content-Id") != "<logo@hitstor>" || ip.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("image headers %v", ip.Header)
	}
	enc, _ := io.ReadAll(ip)
	got, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(enc), "\r\n", ""))
	if err != nil || !bytes.Equal(got, img) {
		t.Fatalf("image data differs: %v", err)
	}
	if _, err := rel.NextPart(); err != io.EOF {
		t.Fatalf("extra part: %v", err)
	}
	for _, line := range strings.Split(string(raw), "\r\n") {
		if len(line) > 998 {
			t.Fatal("line longer than SMTP allows")
		}
	}
	msg.Inline[0].CID = "x>\r\nBcc: y@example.com"
	if _, err := buildMessage("a@example.com", "", msg, time.Now()); err == nil {
		t.Fatal("header injection through the content ID")
	}
}

func TestAttachments(t *testing.T) {
	msg := testMessage()
	msg.Inline = []Inline{{CID: "logo@hitstor", ContentType: "image/png", Name: "hitstor.png", Data: []byte("png")}}
	pdf := bytes.Repeat([]byte("%PDF-1.4 report "), 100)
	msg.Attachments = []Attachment{{Name: "report.pdf", ContentType: "application/pdf", Data: pdf}}
	raw, err := buildMessage("a@example.com", "", msg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	mt, params, _ := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if mt != "multipart/mixed" {
		t.Fatalf("top level %q", mt)
	}
	mr := multipart.NewReader(m.Body, params["boundary"])
	body, _ := mr.NextPart()
	if bt, _, _ := mime.ParseMediaType(body.Header.Get("Content-Type")); bt != "multipart/alternative" {
		t.Fatalf("body %q", bt)
	}
	att, err := mr.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if att.FileName() != "report.pdf" || att.Header.Get("Content-Type") != `application/pdf; name="report.pdf"` {
		t.Fatalf("attachment headers %v", att.Header)
	}
	enc, _ := io.ReadAll(att)
	if got, _ := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(enc), "\r\n", "")); !bytes.Equal(got, pdf) {
		t.Fatal("attachment data differs")
	}
	if _, err := mr.NextPart(); err != io.EOF {
		t.Fatalf("extra part: %v", err)
	}
	msg.Attachments[0].Name = "x\r\nBcc: y@example.com"
	if _, err := buildMessage("a@example.com", "", msg, time.Now()); err == nil {
		t.Fatal("header injection through the file name")
	}
}
