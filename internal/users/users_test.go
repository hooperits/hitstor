package users

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

var testKey = [32]byte{7}

func TestCheckPassword(t *testing.T) {
	for pw, ok := range map[string]bool{
		"short":                    false,
		"qwertyuiopasdfghjkl":      false, // common
		"carlos-is-my-name-123":    false, // contains the user name
		"MyHitStorPassword2026":    false, // product name
		"correct horse battery 42": true,
		strings.Repeat("x", 129):   false,
	} {
		if err := CheckPassword(pw, "carlos"); (err == nil) != ok {
			t.Errorf("%q: %v", pw, err)
		}
	}
	h, err := HashPassword("correct horse battery 42")
	if err != nil || !strings.HasPrefix(h, "$argon2id$") {
		t.Fatalf("hash %q %v", h, err)
	}
	if !VerifyPassword(h, "correct horse battery 42") || VerifyPassword(h, "correct horse battery 43") {
		t.Fatal("verify")
	}
}

func TestPermissions(t *testing.T) {
	op := &User{Grants: []Grant{{Role: RoleBackupOperator, Agents: []string{"sql01"}}, {Role: RoleViewer}}}
	if !op.Can(PermBackup, "sql01") || op.Can(PermBackup, "web01") || !op.Can(PermView, "web01") || op.Can(PermRestore, "sql01") {
		t.Fatal("operator scope")
	}
	if all, agents := op.Agents(PermBackup); all || !agents["sql01"] || len(agents) != 1 {
		t.Fatalf("agents = %v %v", all, agents)
	}
	if !op.CanAll(PermView) || op.CanAll(PermBackup) || op.Can(PermAdmin, "") {
		t.Fatal("global checks")
	}
	admin := &User{Grants: []Grant{{Role: RoleAdmin, Agents: []string{"ignored"}}}}
	if !admin.Can(PermRestore, "anything") || !admin.CanAll(PermAdmin) {
		t.Fatal("admin acts on every agent")
	}
	admin.Disabled = true
	if admin.Can(PermView, "") {
		t.Fatal("disabled user can do nothing")
	}
}

func code(t *testing.T, secret string, at time.Time) string {
	c, err := totp.GenerateCodeCustom(secret, at, totpOpts)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSignInEnrollAndSecondFactor(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, testKey)
	if err != nil {
		t.Fatal(err)
	}
	const pw = "correct horse battery 42"
	if _, err := s.Add("Carlos", "", pw, []Grant{{Role: RoleAdmin}}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add("carlos", "", pw, []Grant{{Role: RoleViewer}}, false); !errors.Is(err, ErrUserExists) {
		t.Fatalf("duplicate: %v", err)
	}

	// First sign-in: the second factor must be enrolled.
	tok, _, err := s.Login("carlos", pw, "10.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if sess, _, _ := s.Authenticate(tok); sess.Stage != StageEnroll {
		t.Fatalf("stage = %s", sess.Stage)
	}
	secret, url, png, err := s.BeginEnrollment(tok)
	if err != nil || !strings.HasPrefix(url, "otpauth://totp/HitStor:carlos") || len(png) == 0 {
		t.Fatalf("enroll: %v %s", err, url)
	}
	if _, err := s.FinishEnrollment(tok, "000000"); !errors.Is(err, ErrBadCode) {
		t.Fatalf("bad code: %v", err)
	}
	now := time.Now()
	recovery, err := s.FinishEnrollment(tok, code(t, secret, now))
	if err != nil || len(recovery) != recoveryCodeCount {
		t.Fatalf("finish: %v %v", recovery, err)
	}
	if sess, u, _ := s.Authenticate(tok); sess.Stage != StageFull || !u.TOTPEnabled {
		t.Fatalf("after enrollment: %+v", sess)
	}

	// Next sign-in: password, then a code; the enrollment code cannot be
	// used again.
	tok2, _, err := s.Login("carlos", pw, "10.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CompleteSecondFactor(tok2, code(t, secret, now)); !errors.Is(err, ErrBadCode) {
		t.Fatalf("replayed code: %v", err)
	}
	if used, left, err := s.CompleteSecondFactor(tok2, strings.ToLower(recovery[0])); err != nil || !used || left != recoveryCodeCount-1 {
		t.Fatalf("recovery code: %v %v %v", used, left, err)
	}
	tok3, _, _ := s.Login("carlos", pw, "", "")
	if _, _, err := s.CompleteSecondFactor(tok3, recovery[0]); !errors.Is(err, ErrBadCode) {
		t.Fatalf("recovery code used twice: %v", err)
	}

	// Another process (the hitstor user command) sees and changes the file.
	s2, err := Open(dir, testKey)
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.Sessions("carlos"); len(got) != 2 {
		t.Fatalf("sessions seen by another process: %d", len(got))
	}
	time.Sleep(10 * time.Millisecond)
	if err := s2.SetDisabled("carlos", true); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("last admin disabled: %v", err)
	}
	if err := s2.ResetSecondFactor("carlos"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Authenticate(tok); !errors.Is(err, ErrNoSession) {
		t.Fatalf("session after reset in another process: %v", err)
	}
}

func TestLockoutAndSessionTimeouts(t *testing.T) {
	s, _ := Open(t.TempDir(), testKey)
	const pw = "correct horse battery 42"
	s.Add("ana", "", pw, []Grant{{Role: RoleViewer}}, false)
	for range lockAfterFailures {
		if _, _, err := s.Login("ana", "wrong password here", "", ""); !errors.Is(err, ErrBadCredentials) {
			t.Fatalf("bad password: %v", err)
		}
	}
	if _, _, err := s.Login("ana", pw, "", ""); !errors.Is(err, ErrLocked) {
		t.Fatalf("after %d failures: %v", lockAfterFailures, err)
	}
	if _, _, err := s.Login("nobody", pw, "", ""); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("unknown user: %v", err)
	}

	// Idle and absolute limits.
	s.SetDisabled("ana", false)
	tok, _, _ := s.Login("ana", pw, "", "")
	s.mu.Lock()
	sess := s.sessionByToken(tok)
	sess.Stage, sess.LastSeen, sess.Created = StageFull, time.Now().Add(-idleTimeout-time.Minute), time.Now().Add(-time.Hour)
	s.mu.Unlock()
	if _, _, err := s.Authenticate(tok); !errors.Is(err, ErrNoSession) {
		t.Fatalf("idle session: %v", err)
	}
}

func TestAuditChain(t *testing.T) {
	dir := t.TempDir()
	a, _ := OpenAuditLog(dir)
	for _, act := range []string{"login", "user.add", "logout"} {
		if err := a.Append(AuditEvent{User: "carlos", Action: act, OK: true}); err != nil {
			t.Fatal(err)
		}
	}
	// Another process (the CLI) appends; both keep one chain.
	cli, _ := OpenAuditLog(dir)
	cli.Append(AuditEvent{User: "console", Action: "user.passwd", OK: true})
	a.Append(AuditEvent{User: "carlos", Action: "login", OK: true})
	events, broken, err := a.Recent(10)
	if err != nil || broken != 0 || len(events) != 5 || events[0].Action != "login" || events[1].Action != "user.passwd" {
		t.Fatalf("events = %+v broken=%d %v", events, broken, err)
	}
	// Editing a line breaks the chain after it.
	p := filepath.Join(dir, "audit.log")
	raw, _ := os.ReadFile(p)
	os.WriteFile(p, []byte(strings.Replace(string(raw), `"user.add"`, `"user.del"`, 1)), 0o600)
	if _, broken, _ := a.Recent(10); broken != 3 {
		t.Fatalf("tampering not detected: broken = %d", broken)
	}
}
