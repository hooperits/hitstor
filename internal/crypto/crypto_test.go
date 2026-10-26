package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	tests := []struct {
		name      string
		plaintext []byte
	}{
		{
			name:      "normal plaintext",
			plaintext: []byte("hello, hitstor backup engine"),
		},
		{
			name:      "empty plaintext",
			plaintext: []byte{},
		},
		{
			name:      "binary data",
			plaintext: []byte{0x00, 0xFF, 0x01, 0xFE, 0x02, 0xFD},
		},
		{
			name:      "large plaintext",
			plaintext: bytes.Repeat([]byte("A"), 1<<16),
		},
	}

	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, err := Encrypt(key, tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}

			decrypted, err := Decrypt(key, ciphertext)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}

			if !bytes.Equal(decrypted, tt.plaintext) {
				t.Errorf("decrypted data does not match original plaintext\ngot:  %x\nwant: %x", decrypted, tt.plaintext)
			}
		})
	}
}

func TestDifferentNoncesProduceDifferentCiphertext(t *testing.T) {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	plaintext := []byte("same data encrypted twice")

	ct1, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("first Encrypt: %v", err)
	}

	ct2, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("second Encrypt: %v", err)
	}

	if bytes.Equal(ct1, ct2) {
		t.Error("encrypting the same plaintext twice produced identical ciphertext; nonces should differ")
	}

	// Both should still decrypt correctly.
	d1, err := Decrypt(key, ct1)
	if err != nil {
		t.Fatalf("Decrypt ct1: %v", err)
	}
	d2, err := Decrypt(key, ct2)
	if err != nil {
		t.Fatalf("Decrypt ct2: %v", err)
	}

	if !bytes.Equal(d1, plaintext) || !bytes.Equal(d2, plaintext) {
		t.Error("decrypted data does not match original plaintext")
	}
}

func TestTamperedCiphertextFailsAuthentication(t *testing.T) {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	plaintext := []byte("sensitive data that must not be tampered with")

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	tests := []struct {
		name   string
		modify func([]byte) []byte
	}{
		{
			name: "flip byte in ciphertext body",
			modify: func(ct []byte) []byte {
				tampered := make([]byte, len(ct))
				copy(tampered, ct)
				// Flip a byte in the ciphertext portion (after the nonce).
				idx := nonceSize + 1
				tampered[idx] ^= 0xFF
				return tampered
			},
		},
		{
			name: "flip byte in nonce",
			modify: func(ct []byte) []byte {
				tampered := make([]byte, len(ct))
				copy(tampered, ct)
				tampered[0] ^= 0xFF
				return tampered
			},
		},
		{
			name: "flip byte in tag",
			modify: func(ct []byte) []byte {
				tampered := make([]byte, len(ct))
				copy(tampered, ct)
				tampered[len(tampered)-1] ^= 0xFF
				return tampered
			},
		},
		{
			name: "truncated ciphertext",
			modify: func(ct []byte) []byte {
				return ct[:nonceSize] // Only the nonce, missing ciphertext+tag.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tampered := tt.modify(ciphertext)
			_, err := Decrypt(key, tampered)
			if err == nil {
				t.Error("Decrypt should have returned an error for tampered ciphertext")
			}
		})
	}
}

func TestEmptyPlaintextEncryptDecrypt(t *testing.T) {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	plaintext := []byte{}

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt empty plaintext: %v", err)
	}

	// Ciphertext should be nonce(12) + tag(16) = 28 bytes for empty plaintext.
	expectedLen := nonceSize + tagSize
	if len(ciphertext) != expectedLen {
		t.Errorf("ciphertext length = %d, want %d for empty plaintext", len(ciphertext), expectedLen)
	}

	decrypted, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt empty plaintext: %v", err)
	}

	if len(decrypted) != 0 {
		t.Errorf("decrypted length = %d, want 0", len(decrypted))
	}
}
