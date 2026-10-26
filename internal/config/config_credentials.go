package config

import (
	"encoding/base64"
	"fmt"

	"github.com/hooperits/hitstor/internal/crypto"
)

// EncryptCredential encrypts a plaintext string with AES-256-GCM and returns
// a base64-encoded ciphertext suitable for storing in YAML config.
func EncryptCredential(key [32]byte, plaintext string) (string, error) {
	ciphertext, err := crypto.Encrypt(key, []byte(plaintext))
	if err != nil {
		return "", fmt.Errorf("config: encrypt credential: %w", err)
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptCredential decodes a base64-encoded AES-256-GCM ciphertext and decrypts it.
func DecryptCredential(key [32]byte, encoded string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("config: base64 decode: %w", err)
	}
	plaintext, err := crypto.Decrypt(key, ciphertext)
	if err != nil {
		return "", fmt.Errorf("config: decrypt credential: %w", err)
	}
	return string(plaintext), nil
}
