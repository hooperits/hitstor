package agent

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// GenerateToken creates a new 32-byte random token, writes it to path with
// mode 0600, and returns the 64-char hex string.
func GenerateToken(path string) (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	token := hex.EncodeToString(buf[:])

	if err := os.WriteFile(path, []byte(token+"\n"), 0600); err != nil {
		return "", fmt.Errorf("write token file: %w", err)
	}
	return token, nil
}

// LoadToken reads a previously generated token from path.
func LoadToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read token file: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if len(token) != 64 {
		return "", fmt.Errorf("invalid token length: expected 64, got %d", len(token))
	}
	return token, nil
}

// VerifyToken compares a provided token with the stored one using
// constant-time comparison to prevent timing attacks.
func VerifyToken(stored, provided string) bool {
	if len(stored) != 64 || len(provided) != 64 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(provided)) == 1
}
