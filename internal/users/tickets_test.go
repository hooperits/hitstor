package users

import (
	"errors"
	"testing"
	"time"
)

func TestEmailAddresses(t *testing.T) {
	s, err := Open(t.TempDir(), testKey)
	if err != nil {
		t.Fatal(err)
	}
	const pw = "correct horse battery 42"
	if _, err := s.Add("ana", "", "Ana.Perez@Example.com ", pw, []Grant{{Role: RoleViewer}}, false); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.Get("ana"); u.Email != "ana.perez@example.com" {
		t.Fatalf("email stored as %q", u.Email)
	}
	if _, err := s.Add("ana2", "", "ana.perez@example.com", pw, []Grant{{Role: RoleViewer}}, false); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("duplicate email: %v", err)
	}
	for _, bad := range []string{"ana", "Ana <ana@example.com>", "ana@localhost", "a@b@example.com"} {
		if _, err := s.Add("x"+bad[:1], "", bad, pw, []Grant{{Role: RoleViewer}}, false); !errors.Is(err, ErrBadEmail) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if _, err := s.Add("luis", "", "", pw, []Grant{{Role: RoleViewer}}, false); err != nil {
		t.Fatal(err) // no address is fine, and does not collide with others without one
	}
	if _, err := s.Add("eva", "", "", pw, []Grant{{Role: RoleViewer}}, false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEmail("luis", "ANA.perez@example.com"); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("taken: %v", err)
	}
	if err := s.SetEmail("luis", "luis@example.com"); err != nil {
		t.Fatal(err)
	}
	for _, login := range []string{"LUIS", "Luis@Example.com"} {
		if u, err := s.FindAccount(login); err != nil || u.Username != "luis" {
			t.Errorf("FindAccount(%q) = %q, %v", login, u.Username, err)
		}
	}
	if _, err := s.FindAccount("nobody@example.com"); !errors.Is(err, ErrNoUser) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestInviteTicket(t *testing.T) {
	s, _ := Open(t.TempDir(), testKey)
	if _, err := s.Add("ana", "", "ana@example.com", TemporaryPassword(), []Grant{{Role: RoleViewer}}, true); err != nil {
		t.Fatal(err)
	}
	token, u, err := s.IssueTicket("ana", TicketInvite)
	if err != nil || u.Email != "ana@example.com" {
		t.Fatal(err)
	}
	purpose, u, err := s.Ticket(token)
	if err != nil || purpose != TicketInvite || u.Username != "ana" {
		t.Fatalf("ticket: %q %q %v", purpose, u.Username, err)
	}
	if _, err := s.RedeemTicket(token, "short", ""); !errors.Is(err, ErrPasswordShort) {
		t.Fatalf("weak password: %v", err)
	}
	const pw = "my own phrase for hitting"
	if _, err := s.RedeemTicket(token, "a good long phrase 2026", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RedeemTicket(token, pw, ""); !errors.Is(err, ErrBadTicket) {
		t.Fatalf("second use: %v", err)
	}
	u, _ = s.Get("ana")
	if u.MustChangePassword || !VerifyPassword(u.PasswordHash, "a good long phrase 2026") {
		t.Fatal("password not set")
	}
	// A later reload (another process) sees the ticket gone.
	s2, _ := Open(s.path[:len(s.path)-len("/users.json")], testKey)
	if _, _, err := s2.Ticket(token); !errors.Is(err, ErrBadTicket) {
		t.Fatalf("persisted: %v", err)
	}
}

func TestResetTicketNeedsSecondFactor(t *testing.T) {
	s, _ := Open(t.TempDir(), testKey)
	const pw = "correct horse battery 42"
	s.Add("ana", "", "ana@example.com", pw, []Grant{{Role: RoleViewer}}, false)
	enrollUser(t, s, "ana", pw)
	u, _ := s.Get("ana")

	token, _, err := s.IssueTicket("ana", TicketReset)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.IssueTicket("ana", TicketReset); !errors.Is(err, ErrTooSoon) {
		t.Fatalf("second reset right away: %v", err)
	}
	const next = "another good phrase 77"
	if _, err := s.RedeemTicket(token, next, ""); !errors.Is(err, ErrBadCode) {
		t.Fatalf("without code: %v", err)
	}
	if u2, _ := s.Get("ana"); u2.FailedLogins != 1 {
		t.Fatalf("failure not counted: %d", u2.FailedLogins)
	}
	if _, err := s.RedeemTicket(token, next, code(t, u.TOTPSecret, time.Now().Add(30*time.Second))); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Login("ana", next, "", ""); err != nil {
		t.Fatalf("new password: %v", err)
	}
	if n := len(s.Sessions("ana")); n != 0 {
		t.Fatalf("full sessions after reset: %d", n)
	}
}

func TestTicketsEndWithPasswordChangesAndAccounts(t *testing.T) {
	s, _ := Open(t.TempDir(), testKey)
	const pw = "correct horse battery 42"
	for _, n := range []string{"ana", "luis", "eva", "admin"} {
		s.Add(n, "", n+"@example.com", pw, []Grant{{Role: RoleViewer}}, false)
	}
	s.SetGrants("admin", []Grant{{Role: RoleAdmin}})
	issue := func(name string) string {
		tok, _, err := s.IssueTicket(name, TicketInvite)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	old := issue("ana")
	newer := issue("ana")
	if _, _, err := s.Ticket(old); !errors.Is(err, ErrBadTicket) {
		t.Fatal("an older link still works")
	}
	s.SetPassword("ana", "set by an administrator 1", true, "")
	if _, _, err := s.Ticket(newer); !errors.Is(err, ErrBadTicket) {
		t.Fatal("link survives a password reset")
	}
	tok := issue("luis")
	s.SetDisabled("luis", true)
	if _, _, err := s.Ticket(tok); !errors.Is(err, ErrBadTicket) {
		t.Fatal("link of a disabled account")
	}
	if _, _, err := s.IssueTicket("luis", TicketReset); !errors.Is(err, ErrNoUser) {
		t.Fatal("link issued to a disabled account")
	}
	tok = issue("eva")
	s.SetEmail("eva", "eva.nueva@example.com")
	if _, _, err := s.Ticket(tok); !errors.Is(err, ErrBadTicket) {
		t.Fatal("link sent to the old address still works")
	}
	tok = issue("eva")
	s.Remove("eva")
	if _, _, err := s.Ticket(tok); !errors.Is(err, ErrBadTicket) {
		t.Fatal("link of a removed account")
	}
	if _, _, err := s.Ticket(""); !errors.Is(err, ErrBadTicket) {
		t.Fatal("empty token")
	}
}

// enrollUser signs name in and turns on its second factor.
func enrollUser(t *testing.T, s *Store, name, pw string) {
	t.Helper()
	token, _, err := s.Login(name, pw, "", "")
	if err != nil {
		t.Fatal(err)
	}
	secret, _, _, err := s.BeginEnrollment(token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishEnrollment(token, code(t, secret, time.Now())); err != nil {
		t.Fatal(err)
	}
	s.Logout(token)
}
