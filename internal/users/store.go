package users

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/crypto"
)

// Lockout and session limits, after NIST SP 800-63B-4 (AAL2) and the OWASP
// session management guidance.
const (
	lockAfterFailures = 10
	lockDuration      = 15 * time.Minute

	pendingSessionTTL = 10 * time.Minute // between password and second factor
	idleTimeout       = 30 * time.Minute
	adminIdleTimeout  = 15 * time.Minute
	absoluteTimeout   = 12 * time.Hour
	// StepUpWindow is how recent the last sign-in must be for sensitive
	// actions (user management, deletions, restores elsewhere).
	StepUpWindow = 15 * time.Minute

	touchEvery = time.Minute // how often activity is written to disk
)

// Session stages: a session is only fully signed in after the second
// factor; before that it may only complete it (or enroll one).
const (
	StageSecondFactor = "second-factor"
	StageEnroll       = "enroll"
	StageFull         = "full"
)

// Session is a signed-in browser. Only the token's hash is stored.
type Session struct {
	ID        string    `json:"id"` // short public id, to list and end sessions
	TokenHash string    `json:"token_hash"`
	UserID    string    `json:"user_id"`
	Stage     string    `json:"stage"`
	Created   time.Time `json:"created"`
	LastSeen  time.Time `json:"last_seen"`
	AuthAt    time.Time `json:"auth_at"` // last full sign-in or re-authentication
	IP        string    `json:"ip,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
	// PendingSecret holds a TOTP secret being enrolled.
	PendingSecret string `json:"pending_secret,omitempty"`
}

type storeData struct {
	Version  int        `json:"version"`
	Pepper   []byte     `json:"pepper"`
	Users    []*User    `json:"users"`
	Sessions []*Session `json:"sessions"`
}

// Store keeps the accounts in an encrypted file. It reloads the file when
// another process (the hitstor user command) changed it.
type Store struct {
	mu      sync.Mutex
	path    string
	key     [32]byte
	modTime time.Time
	size    int64
	data    storeData
	touched map[string]time.Time // session id -> last activity not yet saved
}

// Errors returned by the sign-in steps; callers show a generic message.
var (
	ErrBadCredentials = errors.New("invalid user name or password")
	ErrLocked         = errors.New("account temporarily locked after failed attempts")
	ErrBadCode        = errors.New("invalid code")
	ErrNoSession      = errors.New("not signed in")
	ErrUserExists     = errors.New("user already exists")
	ErrNoUser         = errors.New("no such user")
	ErrLastAdmin      = errors.New("the last enabled administrator cannot be removed, disabled or demoted")
	ErrBadUsername    = errors.New("user name must be 2-64 characters: letters, digits, '.', '_' or '-'")
	ErrNoRoles        = errors.New("give the user at least one role")
	ErrUnknownRole    = errors.New("unknown role")
)

// Open loads the store in dir, encrypted with key; a missing file is an
// empty store.
func Open(dir string, key [32]byte) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "users.json"), key: key, touched: map[string]time.Time{}}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reload(true); err != nil {
		return nil, err
	}
	return s, nil
}

// reload reads the file if it changed since it was last read.
func (s *Store) reload(force bool) error {
	fi, err := os.Stat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		if s.data.Pepper == nil {
			s.data = storeData{Version: 1, Pepper: randomBytes(32)}
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !force && fi.ModTime().Equal(s.modTime) && fi.Size() == s.size {
		return nil
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	plain, err := crypto.Decrypt(s.key, raw)
	if err != nil {
		return fmt.Errorf("users: decrypt %s: %w", s.path, err)
	}
	var d storeData
	if err := json.Unmarshal(plain, &d); err != nil {
		return fmt.Errorf("users: parse %s: %w", s.path, err)
	}
	s.data, s.modTime, s.size = d, fi.ModTime(), fi.Size()
	return nil
}

func (s *Store) save() error {
	for _, sess := range s.data.Sessions {
		if t, ok := s.touched[sess.ID]; ok && t.After(sess.LastSeen) {
			sess.LastSeen = t
		}
	}
	s.touched = map[string]time.Time{}
	plain, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	enc, err := crypto.Encrypt(s.key, plain)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, enc, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return err
	}
	if fi, err := os.Stat(s.path); err == nil {
		s.modTime, s.size = fi.ModTime(), fi.Size()
	}
	return nil
}

// update runs fn on fresh data and saves it.
func (s *Store) update(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reload(false); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	return s.save()
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func randomID(n int) string { return hex.EncodeToString(randomBytes(n)) }

func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

var usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,63}$`)

// NormalizeUsername lowercases a user name and checks its form.
func NormalizeUsername(name string) (string, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	if !usernameRe.MatchString(n) {
		return "", ErrBadUsername
	}
	return n, nil
}

func (s *Store) userByName(name string) *User {
	for _, u := range s.data.Users {
		if u.Username == name {
			return u
		}
	}
	return nil
}

func (s *Store) userByID(id string) *User {
	for _, u := range s.data.Users {
		if u.ID == id {
			return u
		}
	}
	return nil
}

func copyUser(u *User) User {
	c := *u
	c.Grants = slices.Clone(u.Grants)
	for i := range c.Grants {
		c.Grants[i].Agents = slices.Clone(c.Grants[i].Agents)
	}
	c.RecoveryCodes = slices.Clone(u.RecoveryCodes)
	return c
}

// Count returns how many accounts exist.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reload(false)
	return len(s.data.Users)
}

// List returns every account, sorted by user name.
func (s *Store) List() []User {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reload(false)
	out := make([]User, 0, len(s.data.Users))
	for _, u := range s.data.Users {
		out = append(out, copyUser(u))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out
}

// Get returns the account named name.
func (s *Store) Get(name string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reload(false)
	u := s.userByName(strings.ToLower(name))
	if u == nil {
		return User{}, ErrNoUser
	}
	return copyUser(u), nil
}

func validGrants(grants []Grant) error {
	if len(grants) == 0 {
		return ErrNoRoles
	}
	for _, g := range grants {
		if !ValidRole(g.Role) {
			return fmt.Errorf("%w %q", ErrUnknownRole, g.Role)
		}
	}
	return nil
}

// Add creates an account. mustChange makes the user pick a new password at
// first sign-in (for one set by an administrator).
func (s *Store) Add(name, displayName, password string, grants []Grant, mustChange bool) (User, error) {
	name, err := NormalizeUsername(name)
	if err != nil {
		return User{}, err
	}
	if err := CheckPassword(password, name); err != nil {
		return User{}, err
	}
	if err := validGrants(grants); err != nil {
		return User{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	var out User
	err = s.update(func() error {
		if s.userByName(name) != nil {
			return ErrUserExists
		}
		now := time.Now().UTC()
		u := &User{ID: randomID(8), Username: name, DisplayName: displayName, Grants: grants,
			PasswordHash: hash, PasswordChangedAt: now, MustChangePassword: mustChange, CreatedAt: now}
		s.data.Users = append(s.data.Users, u)
		out = copyUser(u)
		return nil
	})
	return out, err
}

// enabledAdmins counts enabled administrators other than except.
func (s *Store) enabledAdmins(except string) int {
	n := 0
	for _, u := range s.data.Users {
		if u.ID != except && !u.Disabled && u.IsAdmin() {
			n++
		}
	}
	return n
}

// SetGrants changes an account's roles. The last enabled administrator
// keeps the administrator role.
func (s *Store) SetGrants(name string, grants []Grant) error {
	if err := validGrants(grants); err != nil {
		return err
	}
	return s.update(func() error {
		u := s.userByName(strings.ToLower(name))
		if u == nil {
			return ErrNoUser
		}
		was := u.IsAdmin()
		u.Grants = grants
		if was && !u.IsAdmin() && s.enabledAdmins(u.ID) == 0 {
			u.Grants = append(grants, Grant{Role: RoleAdmin})
			return ErrLastAdmin
		}
		return nil
	})
}

// SetDisabled disables or enables an account; disabling ends its sessions.
func (s *Store) SetDisabled(name string, disabled bool) error {
	return s.update(func() error {
		u := s.userByName(strings.ToLower(name))
		if u == nil {
			return ErrNoUser
		}
		if disabled && u.IsAdmin() && s.enabledAdmins(u.ID) == 0 {
			return ErrLastAdmin
		}
		u.Disabled = disabled
		if disabled {
			s.dropSessions(u.ID, "")
		} else {
			u.FailedLogins, u.LockedUntil = 0, nil
		}
		return nil
	})
}

// Remove deletes an account and its sessions.
func (s *Store) Remove(name string) error {
	return s.update(func() error {
		u := s.userByName(strings.ToLower(name))
		if u == nil {
			return ErrNoUser
		}
		if u.IsAdmin() && s.enabledAdmins(u.ID) == 0 {
			return ErrLastAdmin
		}
		s.dropSessions(u.ID, "")
		s.data.Users = slices.DeleteFunc(s.data.Users, func(x *User) bool { return x.ID == u.ID })
		return nil
	})
}

// SetPassword sets an account's password. mustChange asks the user to
// choose another at next sign-in; the account's other sessions end,
// except keepSession.
func (s *Store) SetPassword(name, password string, mustChange bool, keepSession string) error {
	name = strings.ToLower(name)
	if err := CheckPassword(password, name); err != nil {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return s.update(func() error {
		u := s.userByName(name)
		if u == nil {
			return ErrNoUser
		}
		u.PasswordHash, u.PasswordChangedAt, u.MustChangePassword = hash, time.Now().UTC(), mustChange
		u.FailedLogins, u.LockedUntil = 0, nil
		s.dropSessions(u.ID, keepSession)
		return nil
	})
}

// ResetSecondFactor removes an account's TOTP and recovery codes: the user
// enrolls again at next sign-in. Its sessions end.
func (s *Store) ResetSecondFactor(name string) error {
	return s.update(func() error {
		u := s.userByName(strings.ToLower(name))
		if u == nil {
			return ErrNoUser
		}
		u.TOTPSecret, u.TOTPEnabled, u.TOTPLastStep, u.RecoveryCodes = "", false, 0, nil
		s.dropSessions(u.ID, "")
		return nil
	})
}

// --- Sign-in ---

// fail records a failed attempt and locks the account after too many.
func (s *Store) fail(u *User, now time.Time) {
	u.FailedLogins++
	if u.FailedLogins >= lockAfterFailures {
		until := now.Add(lockDuration)
		u.LockedUntil, u.FailedLogins = &until, 0
	}
}

// Login checks a user name and password and opens a session that still
// needs the second factor (or its enrollment). It returns the session
// token.
func (s *Store) Login(name, password, ip, userAgent string) (string, User, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	now := time.Now().UTC()
	var token string
	var out User
	err := s.update(func() error {
		u := s.userByName(name)
		if u == nil || u.Disabled {
			VerifyPassword(dummyHash, password)
			return ErrBadCredentials
		}
		if u.Locked(now) {
			return ErrLocked
		}
		if !VerifyPassword(u.PasswordHash, password) {
			s.fail(u, now)
			return ErrBadCredentials
		}
		stage := StageSecondFactor
		if !u.TOTPEnabled {
			stage = StageEnroll
		}
		token = s.newSession(u.ID, stage, ip, userAgent, now)
		out = copyUser(u)
		return nil
	})
	if errors.Is(err, ErrBadCredentials) || errors.Is(err, ErrLocked) {
		// The failure count was changed: keep it even though update failed.
		s.mu.Lock()
		s.save()
		s.mu.Unlock()
	}
	return token, out, err
}

func (s *Store) newSession(userID, stage, ip, ua string, now time.Time) string {
	token := randomID(32)
	s.data.Sessions = append(s.data.Sessions, &Session{ID: randomID(6), TokenHash: tokenHash(token), UserID: userID,
		Stage: stage, Created: now, LastSeen: now, IP: ip, UserAgent: ua})
	s.pruneSessions(now)
	return token
}

func (s *Store) sessionByToken(token string) *Session {
	h := tokenHash(token)
	for _, sess := range s.data.Sessions {
		if sess.TokenHash == h {
			return sess
		}
	}
	return nil
}

// expired reports whether sess is no longer valid at now.
func (s *Store) expired(sess *Session, u *User, now time.Time) bool {
	if u == nil || u.Disabled {
		return true
	}
	last := sess.LastSeen
	if t, ok := s.touched[sess.ID]; ok && t.After(last) {
		last = t
	}
	if sess.Stage != StageFull {
		return now.Sub(sess.Created) > pendingSessionTTL
	}
	idle := idleTimeout
	if u.IsAdmin() {
		idle = adminIdleTimeout
	}
	return now.Sub(last) > idle || now.Sub(sess.Created) > absoluteTimeout
}

func (s *Store) pruneSessions(now time.Time) {
	s.data.Sessions = slices.DeleteFunc(s.data.Sessions, func(sess *Session) bool {
		return s.expired(sess, s.userByID(sess.UserID), now)
	})
}

func (s *Store) dropSessions(userID, keepID string) {
	s.data.Sessions = slices.DeleteFunc(s.data.Sessions, func(sess *Session) bool {
		return sess.UserID == userID && sess.ID != keepID
	})
}

// Authenticate returns the session and user of token, and records the
// activity. A session past its stage's lifetime is gone.
func (s *Store) Authenticate(token string) (Session, User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reload(false)
	now := time.Now().UTC()
	sess := s.sessionByToken(token)
	if sess == nil {
		return Session{}, User{}, ErrNoSession
	}
	u := s.userByID(sess.UserID)
	if s.expired(sess, u, now) {
		s.data.Sessions = slices.DeleteFunc(s.data.Sessions, func(x *Session) bool { return x == sess })
		s.save()
		return Session{}, User{}, ErrNoSession
	}
	s.touched[sess.ID] = now
	if now.Sub(sess.LastSeen) > touchEvery {
		s.save()
	}
	return *sess, copyUser(u), nil
}

// verifySecondFactor checks a TOTP code or, failing that, a recovery code,
// and spends it.
func (s *Store) verifySecondFactor(u *User, code string, now time.Time) bool {
	if step, ok := checkTOTP(u.TOTPSecret, code, u.TOTPLastStep, now); ok {
		u.TOTPLastStep = step
		return true
	}
	h := recoveryHash(s.data.Pepper, normalizeRecovery(code))
	for i, rc := range u.RecoveryCodes {
		if rc == h {
			u.RecoveryCodes = slices.Delete(u.RecoveryCodes, i, i+1)
			return true
		}
	}
	return false
}

// CompleteSecondFactor finishes a sign-in with a TOTP or recovery code.
// usedRecovery is true when a recovery code was spent.
func (s *Store) CompleteSecondFactor(token, code string) (usedRecovery bool, remaining int, err error) {
	now := time.Now().UTC()
	err = s.update(func() error {
		sess := s.sessionByToken(token)
		if sess == nil || sess.Stage != StageSecondFactor {
			return ErrNoSession
		}
		u := s.userByID(sess.UserID)
		if s.expired(sess, u, now) {
			return ErrNoSession
		}
		if u.Locked(now) {
			return ErrLocked
		}
		before := len(u.RecoveryCodes)
		if !s.verifySecondFactor(u, code, now) {
			s.fail(u, now)
			return ErrBadCode
		}
		usedRecovery, remaining = len(u.RecoveryCodes) < before, len(u.RecoveryCodes)
		s.signIn(sess, u, now)
		return nil
	})
	if errors.Is(err, ErrBadCode) {
		s.mu.Lock()
		s.save()
		s.mu.Unlock()
	}
	return usedRecovery, remaining, err
}

// signIn turns sess into a full session: a new id (the token stays the
// browser's, but the record is fresh) and a recorded sign-in.
func (s *Store) signIn(sess *Session, u *User, now time.Time) {
	sess.Stage, sess.AuthAt, sess.LastSeen, sess.Created = StageFull, now, now, now
	sess.ID, sess.PendingSecret = randomID(6), ""
	u.FailedLogins, u.LockedUntil, u.LastLoginAt = 0, nil, &now
}

// BeginEnrollment starts TOTP enrollment for an enrolling session, or a
// signed-in user replacing their authenticator, and returns the secret,
// the otpauth URL and its QR code.
func (s *Store) BeginEnrollment(token string) (secret, url string, qrPNG []byte, err error) {
	err = s.update(func() error {
		sess := s.sessionByToken(token)
		if sess == nil || (sess.Stage != StageEnroll && sess.Stage != StageFull) {
			return ErrNoSession
		}
		u := s.userByID(sess.UserID)
		if s.expired(sess, u, time.Now().UTC()) {
			return ErrNoSession
		}
		secret, url, qrPNG, err = NewTOTP(u.Username)
		if err != nil {
			return err
		}
		sess.PendingSecret = secret
		return nil
	})
	return secret, url, qrPNG, err
}

// FinishEnrollment checks a code from the new authenticator, turns TOTP on
// and returns fresh recovery codes, shown once. An enrolling session
// becomes fully signed in.
func (s *Store) FinishEnrollment(token, code string) (recovery []string, err error) {
	now := time.Now().UTC()
	err = s.update(func() error {
		sess := s.sessionByToken(token)
		if sess == nil || sess.PendingSecret == "" {
			return ErrNoSession
		}
		u := s.userByID(sess.UserID)
		if s.expired(sess, u, now) {
			return ErrNoSession
		}
		step, ok := checkTOTP(sess.PendingSecret, code, 0, now)
		if !ok {
			return ErrBadCode
		}
		codes, hashes, err := newRecoveryCodes(s.data.Pepper)
		if err != nil {
			return err
		}
		u.TOTPSecret, u.TOTPEnabled, u.TOTPLastStep, u.RecoveryCodes = sess.PendingSecret, true, step, hashes
		recovery = codes
		if sess.Stage == StageEnroll {
			s.signIn(sess, u, now)
		} else {
			sess.PendingSecret = ""
			s.dropSessions(u.ID, sess.ID) // a new authenticator: other browsers sign in again
		}
		return nil
	})
	return recovery, err
}

// NewRecoveryCodes replaces a user's recovery codes.
func (s *Store) NewRecoveryCodes(name string) (codes []string, err error) {
	err = s.update(func() error {
		u := s.userByName(strings.ToLower(name))
		if u == nil {
			return ErrNoUser
		}
		var hashes []string
		codes, hashes, err = newRecoveryCodes(s.data.Pepper)
		if err != nil {
			return err
		}
		u.RecoveryCodes = hashes
		return nil
	})
	return codes, err
}

// Reauthenticate confirms the signed-in user with their password and a
// second-factor code, for sensitive actions.
func (s *Store) Reauthenticate(token, password, code string) error {
	now := time.Now().UTC()
	err := s.update(func() error {
		sess := s.sessionByToken(token)
		if sess == nil || sess.Stage != StageFull {
			return ErrNoSession
		}
		u := s.userByID(sess.UserID)
		if s.expired(sess, u, now) {
			return ErrNoSession
		}
		if u.Locked(now) {
			return ErrLocked
		}
		if !VerifyPassword(u.PasswordHash, password) || !s.verifySecondFactor(u, code, now) {
			s.fail(u, now)
			return ErrBadCredentials
		}
		sess.AuthAt = now
		u.FailedLogins = 0
		return nil
	})
	if errors.Is(err, ErrBadCredentials) {
		s.mu.Lock()
		s.save()
		s.mu.Unlock()
	}
	return err
}

// Logout ends the session of token.
func (s *Store) Logout(token string) error {
	return s.update(func() error {
		h := tokenHash(token)
		s.data.Sessions = slices.DeleteFunc(s.data.Sessions, func(x *Session) bool { return x.TokenHash == h })
		return nil
	})
}

// Sessions lists a user's sessions, newest first.
func (s *Store) Sessions(name string) []Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reload(false)
	u := s.userByName(strings.ToLower(name))
	var out []Session
	if u == nil {
		return out
	}
	now := time.Now().UTC()
	for _, sess := range s.data.Sessions {
		if sess.UserID == u.ID && sess.Stage == StageFull && !s.expired(sess, u, now) {
			c := *sess
			c.TokenHash, c.PendingSecret = "", ""
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

// EndSession ends one of a user's sessions by its public id.
func (s *Store) EndSession(name, id string) error {
	return s.update(func() error {
		u := s.userByName(strings.ToLower(name))
		if u == nil {
			return ErrNoUser
		}
		s.data.Sessions = slices.DeleteFunc(s.data.Sessions, func(x *Session) bool { return x.UserID == u.ID && x.ID == id })
		return nil
	})
}

// ChangeOwnPassword changes the signed-in user's password after checking
// the current one; other sessions end.
func (s *Store) ChangeOwnPassword(token, current, next string) error {
	sess, u, err := s.Authenticate(token)
	if err != nil {
		return err
	}
	if !VerifyPassword(u.PasswordHash, current) {
		return ErrBadCredentials
	}
	if current == next {
		return ErrPasswordUnchanged
	}
	return s.SetPassword(u.Username, next, false, sess.ID)
}
