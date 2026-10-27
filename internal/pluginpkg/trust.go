package pluginpkg

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// OfficialKeys are the base64 ed25519 public keys that sign official HitStor
// plugin packages. The matching private key lives only in the release
// pipeline.
var OfficialKeys = []string{
	"N0osfl0VEpJDEoy1D7nWj2tCqXrnCjcS8dX8Eb6Pq/M=",
}

// Verification failures.
var (
	ErrUnsigned      = errors.New("pluginpkg: package is not signed")
	ErrUntrustedKey  = errors.New("pluginpkg: package is signed by an untrusted key")
	ErrBadSignature  = errors.New("pluginpkg: signature does not match package contents")
	errBadKeyEncoded = errors.New("expected a base64 ed25519 key")
)

// TrustPolicy decides which packages may be installed.
type TrustPolicy struct {
	// Keys are the trusted publisher public keys.
	Keys []ed25519.PublicKey
	// AllowUnsigned accepts packages without a SIGNATURE. Meant for plugin
	// development only; a package signed by an unknown key is still rejected.
	AllowUnsigned bool
}

// NewTrustPolicy trusts the official keys plus extra base64 public keys.
func NewTrustPolicy(extraKeys []string, allowUnsigned bool) (TrustPolicy, error) {
	p := TrustPolicy{AllowUnsigned: allowUnsigned}
	for _, k := range append(append([]string{}, OfficialKeys...), extraKeys...) {
		pub, err := ParsePublicKey(k)
		if err != nil {
			return TrustPolicy{}, fmt.Errorf("pluginpkg: trusted key %q: %w", k, err)
		}
		p.Keys = append(p.Keys, pub)
	}
	return p, nil
}

func (p TrustPolicy) trusts(pub ed25519.PublicKey) bool {
	for _, k := range p.Keys {
		if k.Equal(pub) {
			return true
		}
	}
	return false
}

// ParsePublicKey decodes a base64 ed25519 public key.
func ParsePublicKey(s string) (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errBadKeyEncoded
	}
	return ed25519.PublicKey(b), nil
}

// ParsePrivateKey decodes a base64 ed25519 seed, the format of signing key
// files and of the release pipeline secret.
func ParsePrivateKey(s string) (ed25519.PrivateKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != ed25519.SeedSize {
		return nil, errors.New("pluginpkg: expected a base64 ed25519 seed (32 bytes)")
	}
	return ed25519.NewKeyFromSeed(b), nil
}

// EncodePublicKey returns the base64 form of a public key.
func EncodePublicKey(pub ed25519.PublicKey) string {
	return base64.StdEncoding.EncodeToString(pub)
}
