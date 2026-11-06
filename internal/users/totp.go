package users

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"image/png"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// TOTP after RFC 6238 with the parameters every authenticator app
// supports: SHA-1, 6 digits, 30-second steps, one step of clock skew.
const (
	totpIssuer = "HitStor"
	totpPeriod = 30
	totpSkew   = 1
)

var totpOpts = totp.ValidateOpts{Period: totpPeriod, Skew: 0, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}

// NewTOTP returns a new 160-bit secret for account, the otpauth:// URL
// authenticator apps import, and that URL as a PNG QR code.
func NewTOTP(account string) (secret, url string, qrPNG []byte, err error) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: totpIssuer, AccountName: account, SecretSize: 20,
		Period: totpPeriod, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		return "", "", nil, err
	}
	img, err := key.Image(240, 240)
	if err != nil {
		return "", "", nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", "", nil, err
	}
	return key.Secret(), key.URL(), buf.Bytes(), nil
}

// checkTOTP returns the time step code belongs to, within one step of now,
// or false. Steps at or before lastStep are refused: each code is good once.
func checkTOTP(secret, code string, lastStep int64, now time.Time) (int64, bool) {
	code = strings.TrimSpace(strings.ReplaceAll(code, " ", ""))
	if len(code) != 6 {
		return 0, false
	}
	step := now.Unix() / totpPeriod
	for d := int64(-totpSkew); d <= totpSkew; d++ {
		s := step + d
		if s <= lastStep {
			continue
		}
		want, err := totp.GenerateCodeCustom(secret, time.Unix(s*totpPeriod, 0).UTC(), totpOpts)
		if err == nil && subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

// Recovery codes: ten per user, 100 random bits each, shown once and kept
// as keyed hashes. Each works once, in place of a TOTP code.
const recoveryCodeCount = 10

func newRecoveryCodes(pepper []byte) (codes, hashes []string, err error) {
	for range recoveryCodeCount {
		b := make([]byte, 13)
		if _, err := rand.Read(b); err != nil {
			return nil, nil, err
		}
		raw := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)[:20]
		codes = append(codes, raw[0:5]+"-"+raw[5:10]+"-"+raw[10:15]+"-"+raw[15:20])
		hashes = append(hashes, recoveryHash(pepper, raw))
	}
	return codes, hashes, nil
}

func normalizeRecovery(code string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
}

func recoveryHash(pepper []byte, normalized string) string {
	m := hmac.New(sha256.New, pepper)
	m.Write([]byte(normalized))
	return hex.EncodeToString(m.Sum(nil))
}
