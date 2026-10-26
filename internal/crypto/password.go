package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/scrypt"
)

// hashedPasswordPrefix marks a stored password as an scrypt hash in the form
// scrypt$N$r$p$base64(salt)$base64(hash).
const hashedPasswordPrefix = "scrypt$"

const passwordHashLen = 32

// HashPassword derives an scrypt hash of password suitable for storage in
// configuration files, using a fresh random 16-byte salt.
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("crypto: generate password salt: %w", err)
	}
	dk, err := scrypt.Key([]byte(password), salt, DefaultN, DefaultR, DefaultP, passwordHashLen)
	if err != nil {
		return "", fmt.Errorf("crypto: scrypt hash password: %w", err)
	}
	return fmt.Sprintf("%s%d$%d$%d$%s$%s",
		hashedPasswordPrefix, DefaultN, DefaultR, DefaultP,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(dk)), nil
}

// IsHashedPassword reports whether stored is an scrypt password hash produced
// by HashPassword, as opposed to a legacy plaintext password.
func IsHashedPassword(stored string) bool {
	return strings.HasPrefix(stored, hashedPasswordPrefix)
}

// VerifyPassword checks input against a stored password in constant time.
// stored may be an scrypt hash from HashPassword or a legacy plaintext value.
// An empty stored password never matches.
func VerifyPassword(stored, input string) bool {
	if stored == "" {
		return false
	}
	if !IsHashedPassword(stored) {
		return subtle.ConstantTimeCompare([]byte(stored), []byte(input)) == 1
	}

	parts := strings.Split(strings.TrimPrefix(stored, hashedPasswordPrefix), "$")
	if len(parts) != 5 {
		return false
	}
	n, err1 := strconv.Atoi(parts[0])
	r, err2 := strconv.Atoi(parts[1])
	p, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil || n <= 1 || r <= 0 || p <= 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(want) == 0 {
		return false
	}
	got, err := scrypt.Key([]byte(input), salt, n, r, p, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}
