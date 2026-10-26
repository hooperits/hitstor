package crypto

import (
	"strings"
	"testing"
)

func TestHashPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("s3cret")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if !strings.HasPrefix(hash, "scrypt$") {
		t.Errorf("hash = %q, want scrypt$ prefix", hash)
	}
	if !VerifyPassword(hash, "s3cret") {
		t.Error("VerifyPassword(hash, correct) = false, want true")
	}
	if VerifyPassword(hash, "wrong") {
		t.Error("VerifyPassword(hash, wrong) = true, want false")
	}
}

func TestHashPasswordUniqueSalt(t *testing.T) {
	h1, err := HashPassword("same")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	h2, err := HashPassword("same")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if h1 == h2 {
		t.Error("two hashes of the same password are identical - salt not random")
	}
}

func TestVerifyPasswordPlaintextFallback(t *testing.T) {
	// Stored values without the scrypt$ prefix are legacy plaintext passwords.
	if !VerifyPassword("legacy-pass", "legacy-pass") {
		t.Error("VerifyPassword(plaintext, correct) = false, want true")
	}
	if VerifyPassword("legacy-pass", "nope") {
		t.Error("VerifyPassword(plaintext, wrong) = true, want false")
	}
}

func TestVerifyPasswordMalformedHash(t *testing.T) {
	tests := []struct {
		name   string
		stored string
	}{
		{"missing fields", "scrypt$only-one-field"},
		{"bad params", "scrypt$x$y$z$AAAA$BBBB"},
		{"bad base64 salt", "scrypt$32768$8$1$!!!$BBBB"},
		{"bad base64 hash", "scrypt$32768$8$1$AAAA$!!!"},
		{"empty", "scrypt$"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if VerifyPassword(tt.stored, "anything") {
				t.Errorf("VerifyPassword(%q) = true, want false", tt.stored)
			}
		})
	}
}

func TestVerifyPasswordEmptyStored(t *testing.T) {
	// An empty stored password must never authenticate anything.
	if VerifyPassword("", "") {
		t.Error("VerifyPassword(\"\", \"\") = true, want false")
	}
	if VerifyPassword("", "x") {
		t.Error("VerifyPassword(\"\", \"x\") = true, want false")
	}
}

func TestIsHashedPassword(t *testing.T) {
	hash, err := HashPassword("p")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if !IsHashedPassword(hash) {
		t.Error("IsHashedPassword(hash) = false, want true")
	}
	if IsHashedPassword("plaintext") {
		t.Error("IsHashedPassword(plaintext) = true, want false")
	}
}
