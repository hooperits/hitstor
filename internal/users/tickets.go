package users

import (
	"errors"
	"slices"
	"strings"
	"time"
)

// Ticket purposes: what an emailed link lets its holder do. Both choose
// the account's password; they differ in the email and how long they last.
const (
	TicketInvite = "invite" // a new account chooses its first password
	TicketReset  = "reset"  // a user who forgot their password chooses another
)

// How long links work, and how soon another reset link may be sent (so a
// stranger cannot flood someone's mailbox).
const (
	InviteTTL     = 72 * time.Hour
	ResetTTL      = time.Hour
	resetCooldown = 2 * time.Minute
)

// Ticket is an emailed one-use link. Only the token's hash is stored.
type Ticket struct {
	Hash    string    `json:"hash"`
	UserID  string    `json:"user_id"`
	Purpose string    `json:"purpose"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
}

var (
	ErrBadTicket = errors.New("the link is not valid: it was already used, it expired or a newer one was sent")
	ErrTooSoon   = errors.New("a link was sent moments ago")
)

func (s *Store) dropTickets(userID string) {
	s.data.Tickets = slices.DeleteFunc(s.data.Tickets, func(t *Ticket) bool { return t.UserID == userID })
}

func (s *Store) pruneTickets(now time.Time) {
	s.data.Tickets = slices.DeleteFunc(s.data.Tickets, func(t *Ticket) bool {
		return !now.Before(t.Expires) || s.userByID(t.UserID) == nil
	})
}

// FindAccount returns the account named login, or whose email address it
// is: a user who forgot their password may know either.
func (s *Store) FindAccount(login string) (User, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reload(false)
	u := s.userByName(login)
	if u == nil {
		u = s.userByEmail(login)
	}
	if u == nil {
		return User{}, ErrNoUser
	}
	return copyUser(u), nil
}

// IssueTicket makes a link for an enabled account and returns its token.
// It replaces the account's earlier links. A reset link is not issued
// again within a couple of minutes (ErrTooSoon).
func (s *Store) IssueTicket(name, purpose string) (string, User, error) {
	ttl := ResetTTL
	if purpose == TicketInvite {
		ttl = InviteTTL
	}
	token := randomID(32)
	var out User
	err := s.update(func() error {
		now := time.Now().UTC()
		s.pruneTickets(now)
		u := s.userByName(strings.ToLower(name))
		if u == nil || u.Disabled {
			return ErrNoUser
		}
		if purpose == TicketReset {
			for _, t := range s.data.Tickets {
				if t.UserID == u.ID && t.Purpose == TicketReset && now.Sub(t.Created) < resetCooldown {
					return ErrTooSoon
				}
			}
		}
		s.dropTickets(u.ID)
		s.data.Tickets = append(s.data.Tickets, &Ticket{Hash: tokenHash(token), UserID: u.ID, Purpose: purpose,
			Created: now, Expires: now.Add(ttl)})
		out = copyUser(u)
		return nil
	})
	if err != nil {
		return "", User{}, err
	}
	return token, out, nil
}

// ticket returns the valid ticket of token and its account.
func (s *Store) ticket(token string, now time.Time) (*Ticket, *User) {
	if token == "" {
		return nil, nil
	}
	h := tokenHash(token)
	for _, t := range s.data.Tickets {
		if t.Hash != h || !now.Before(t.Expires) {
			continue
		}
		if u := s.userByID(t.UserID); u != nil && !u.Disabled {
			return t, u
		}
	}
	return nil, nil
}

// Ticket returns the purpose and account of a link without using it, so
// its page can say whose password is being set.
func (s *Store) Ticket(token string) (string, User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reload(false)
	t, u := s.ticket(token, time.Now().UTC())
	if t == nil {
		return "", User{}, ErrBadTicket
	}
	return t.Purpose, copyUser(u), nil
}

// RedeemTicket uses a link to set the account's password. An account with
// two-step verification also needs a code from it (or a recovery code):
// the link alone, from a mailbox someone else read, is not enough. The
// account's sessions and links end.
func (s *Store) RedeemTicket(token, password, code string) (User, error) {
	now := time.Now().UTC()
	var out User
	var counted bool
	err := s.update(func() error {
		t, u := s.ticket(token, now)
		if t == nil {
			return ErrBadTicket
		}
		if err := CheckPassword(password, u.Username); err != nil {
			return err
		}
		if u.TOTPEnabled {
			if u.Locked(now) {
				return ErrLocked
			}
			if !s.verifySecondFactor(u, code, now) {
				s.fail(u, now)
				counted = true
				return ErrBadCode
			}
		}
		hash, err := HashPassword(password)
		if err != nil {
			return err
		}
		u.PasswordHash, u.PasswordChangedAt, u.MustChangePassword = hash, now, false
		u.FailedLogins, u.LockedUntil = 0, nil
		s.dropSessions(u.ID, "")
		s.dropTickets(u.ID)
		out = copyUser(u)
		return nil
	})
	if counted {
		// The failure count changed: keep it even though update failed.
		s.mu.Lock()
		s.save()
		s.mu.Unlock()
	}
	return out, err
}
