package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os/user"
	"time"

	"golang.org/x/crypto/scrypt"
)

// Default scrypt parameters.
const (
	DefaultN = 32768
	DefaultR = 8
	DefaultP = 1
)

// MasterKey holds the two 32-byte keys used by HitStor.
type MasterKey struct {
	EncryptKey [32]byte // AES-256-GCM encryption key
	IDKey      [32]byte // HMAC-SHA256 key for blob identification
}

// KeyFile represents the on-disk key file that stores an encrypted master key.
type KeyFile struct {
	Created  time.Time `json:"created"`
	Username string    `json:"username"`
	Hostname string    `json:"hostname"`
	KDF      string    `json:"kdf"`  // "scrypt"
	N        int       `json:"n"`    // 32768
	R        int       `json:"r"`    // 8
	P        int       `json:"p"`    // 1
	Salt     []byte    `json:"salt"` // 64-byte random salt
	Data     []byte    `json:"data"` // Encrypted master key
}

// NewMasterKey generates a new MasterKey with 64 random bytes split into
// EncryptKey (first 32 bytes) and IDKey (last 32 bytes).
func NewMasterKey() (*MasterKey, error) {
	var buf [64]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return nil, fmt.Errorf("crypto: generate master key: %w", err)
	}

	mk := &MasterKey{}
	copy(mk.EncryptKey[:], buf[:32])
	copy(mk.IDKey[:], buf[32:64])

	return mk, nil
}

// DeriveKey derives a 64-byte key from a password and salt using scrypt.
func DeriveKey(password string, salt []byte, n, r, p int) ([64]byte, error) {
	var derived [64]byte

	dk, err := scrypt.Key([]byte(password), salt, n, r, p, 64)
	if err != nil {
		return derived, fmt.Errorf("crypto: scrypt derive: %w", err)
	}

	copy(derived[:], dk)
	return derived, nil
}

// CreateKeyFile creates a KeyFile by encrypting the master key with a
// password-derived key. It generates a 64-byte random salt, derives a 64-byte
// key via scrypt, and encrypts the concatenated master key bytes (EncryptKey +
// IDKey = 64 bytes) using AES-256-GCM with the first 32 bytes of the derived key.
func CreateKeyFile(masterKey *MasterKey, password string) (*KeyFile, error) {
	salt := make([]byte, 64)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("crypto: generate salt: %w", err)
	}

	derived, err := DeriveKey(password, salt, DefaultN, DefaultR, DefaultP)
	if err != nil {
		return nil, err
	}

	// Use the first 32 bytes of the derived key for AES-256-GCM encryption.
	var encKey [32]byte
	copy(encKey[:], derived[:32])

	// Concatenate EncryptKey + IDKey into a single 64-byte plaintext.
	var plaintext [64]byte
	copy(plaintext[:32], masterKey.EncryptKey[:])
	copy(plaintext[32:], masterKey.IDKey[:])

	encrypted, err := Encrypt(encKey, plaintext[:])
	if err != nil {
		return nil, fmt.Errorf("crypto: encrypt master key: %w", err)
	}

	username := ""
	if u, err := user.Current(); err == nil {
		username = u.Username
	}

	hostname := ""
	// We intentionally do not fail if hostname lookup fails.

	kf := &KeyFile{
		Created:  time.Now(),
		Username: username,
		Hostname: hostname,
		KDF:      "scrypt",
		N:        DefaultN,
		R:        DefaultR,
		P:        DefaultP,
		Salt:     salt,
		Data:     encrypted,
	}

	return kf, nil
}

// OpenKeyFile decrypts the master key stored in a KeyFile using the given
// password. Returns an error if the password is wrong (authentication failure).
func OpenKeyFile(kf *KeyFile, password string) (*MasterKey, error) {
	derived, err := DeriveKey(password, kf.Salt, kf.N, kf.R, kf.P)
	if err != nil {
		return nil, err
	}

	var encKey [32]byte
	copy(encKey[:], derived[:32])

	plaintext, err := Decrypt(encKey, kf.Data)
	if err != nil {
		return nil, fmt.Errorf("crypto: open key file: %w", err)
	}

	if len(plaintext) != 64 {
		return nil, fmt.Errorf("crypto: unexpected master key length %d, want 64", len(plaintext))
	}

	mk := &MasterKey{}
	copy(mk.EncryptKey[:], plaintext[:32])
	copy(mk.IDKey[:], plaintext[32:64])

	return mk, nil
}

// BlobID computes HMAC-SHA256(idKey, data) and returns the 32-byte result.
// This is used for content-addressable blob identification.
func BlobID(idKey [32]byte, data []byte) [32]byte {
	mac := hmac.New(sha256.New, idKey[:])
	mac.Write(data)
	var result [32]byte
	copy(result[:], mac.Sum(nil))
	return result
}
