package users

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
)

// Password rules, after NIST SP 800-63B-4 and OWASP ASVS 5: a minimum
// length (second factor is mandatory), a generous maximum, no composition
// rules or expiry, and no commonly used passwords.
const (
	MinPasswordLength = 12
	MaxPasswordLength = 128
)

// argon2id parameters (OWASP: at least 19 MiB, 2 passes). Hashes carry
// theirs, so these can grow without breaking stored ones.
const (
	argonTime    = 2
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 2
	argonKeyLen  = 32
)

//go:embed common-passwords.txt.gz
var commonPasswordsGz []byte

var (
	commonOnce      sync.Once
	commonPasswords map[string]bool
)

func isCommon(pw string) bool {
	commonOnce.Do(func() {
		commonPasswords = map[string]bool{}
		zr, err := gzip.NewReader(bytes.NewReader(commonPasswordsGz))
		if err != nil {
			return
		}
		sc := bufio.NewScanner(zr)
		for sc.Scan() {
			commonPasswords[sc.Text()] = true
		}
	})
	return commonPasswords[strings.ToLower(pw)]
}

// ErrWeakPassword is wrapped by every reason a password is refused.
var ErrWeakPassword = errors.New("password not accepted")

var (
	ErrPasswordShort     = fmt.Errorf("%w: use at least %d characters", ErrWeakPassword, MinPasswordLength)
	ErrPasswordLong      = fmt.Errorf("%w: use at most %d characters", ErrWeakPassword, MaxPasswordLength)
	ErrPasswordCommon    = fmt.Errorf("%w: it is a commonly used password", ErrWeakPassword)
	ErrPasswordHasName   = fmt.Errorf("%w: it contains the user name or the product name", ErrWeakPassword)
	ErrPasswordUnchanged = fmt.Errorf("%w: choose a password different from the current one", ErrWeakPassword)
)

// CheckPassword returns why pw cannot be username's password, or nil.
func CheckPassword(pw, username string) error {
	n := utf8.RuneCountInString(pw)
	switch {
	case n < MinPasswordLength:
		return ErrPasswordShort
	case n > MaxPasswordLength:
		return ErrPasswordLong
	case isCommon(pw):
		return ErrPasswordCommon
	}
	lower := strings.ToLower(pw)
	for _, word := range []string{strings.ToLower(username), "hitstor"} {
		if word != "" && strings.Contains(lower, word) {
			return ErrPasswordHasName
		}
	}
	return nil
}

// HashPassword returns pw's argon2id hash in PHC format.
func HashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(norm.NFC.String(pw)), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether pw matches hash.
func VerifyPassword(hash, pw string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(norm.NFC.String(pw)), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash makes a login for an unknown user cost as much as a real one.
var dummyHash, _ = HashPassword("not a real password, only spends time")

// TemporaryPassword returns a random password for an account whose owner
// must choose their own at the next sign-in (about 100 bits).
func TemporaryPassword() string {
	const alphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	out := make([]byte, 0, 20)
	var b [1]byte
	for len(out) < 20 {
		if len(out) == 6 || len(out) == 13 {
			out = append(out, '-')
			continue
		}
		rand.Read(b[:])
		if int(b[0]) >= 256-256%len(alphabet) { // no modulo bias
			continue
		}
		out = append(out, alphabet[int(b[0])%len(alphabet)])
	}
	return string(out)
}
