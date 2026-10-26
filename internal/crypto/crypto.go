// Package crypto provides AES-256-GCM encryption/decryption and key management
// for the HitStor encrypted backup tool.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

const (
	// nonceSize is the size of the GCM nonce in bytes.
	nonceSize = 12
	// tagSize is the size of the GCM authentication tag in bytes.
	tagSize = 16
	// minCiphertextLen is the minimum length of valid ciphertext (nonce + tag).
	minCiphertextLen = nonceSize + tagSize
)

// Encrypt encrypts plaintext using AES-256-GCM with the given 32-byte key.
// The output format is: nonce(12) || ciphertext || tag(16).
func Encrypt(key [32]byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}

	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}

	// Seal appends the ciphertext+tag to nonce, so the result is nonce || ciphertext || tag.
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	return ciphertext, nil
}

// Decrypt decrypts ciphertext produced by Encrypt using AES-256-GCM.
// It expects the input format: nonce(12) || ciphertext || tag(16).
// Returns an error if authentication fails (e.g., tampered data).
func Decrypt(key [32]byte, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < minCiphertextLen {
		return nil, errors.New("crypto: ciphertext too short")
	}

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}

	nonce := ciphertext[:nonceSize]
	data := ciphertext[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}

	return plaintext, nil
}
