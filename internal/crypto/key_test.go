package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestDeriveKeyProduces64Bytes(t *testing.T) {
	salt := make([]byte, 64)
	if _, err := rand.Read(salt); err != nil {
		t.Fatalf("generate salt: %v", err)
	}

	derived, err := DeriveKey("test-password", salt, DefaultN, DefaultR, DefaultP)
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}

	if len(derived) != 64 {
		t.Errorf("derived key length = %d, want 64", len(derived))
	}
}

func TestDeriveKeySamePasswordSaltIsDeterministic(t *testing.T) {
	salt := make([]byte, 64)
	if _, err := rand.Read(salt); err != nil {
		t.Fatalf("generate salt: %v", err)
	}

	password := "deterministic-password"

	dk1, err := DeriveKey(password, salt, DefaultN, DefaultR, DefaultP)
	if err != nil {
		t.Fatalf("first DeriveKey: %v", err)
	}

	dk2, err := DeriveKey(password, salt, DefaultN, DefaultR, DefaultP)
	if err != nil {
		t.Fatalf("second DeriveKey: %v", err)
	}

	if dk1 != dk2 {
		t.Error("same password + salt produced different derived keys")
	}
}

func TestDeriveKeyDifferentPasswordsProduceDifferentKeys(t *testing.T) {
	salt := make([]byte, 64)
	if _, err := rand.Read(salt); err != nil {
		t.Fatalf("generate salt: %v", err)
	}

	dk1, err := DeriveKey("password-one", salt, DefaultN, DefaultR, DefaultP)
	if err != nil {
		t.Fatalf("DeriveKey password-one: %v", err)
	}

	dk2, err := DeriveKey("password-two", salt, DefaultN, DefaultR, DefaultP)
	if err != nil {
		t.Fatalf("DeriveKey password-two: %v", err)
	}

	if dk1 == dk2 {
		t.Error("different passwords produced the same derived key")
	}
}

func TestCreateKeyFileThenOpenKeyFile(t *testing.T) {
	masterKey, err := NewMasterKey()
	if err != nil {
		t.Fatalf("NewMasterKey: %v", err)
	}

	password := "correct-horse-battery-staple"

	kf, err := CreateKeyFile(masterKey, password)
	if err != nil {
		t.Fatalf("CreateKeyFile: %v", err)
	}

	// Verify KeyFile metadata.
	if kf.KDF != "scrypt" {
		t.Errorf("KDF = %q, want %q", kf.KDF, "scrypt")
	}
	if kf.N != DefaultN {
		t.Errorf("N = %d, want %d", kf.N, DefaultN)
	}
	if kf.R != DefaultR {
		t.Errorf("R = %d, want %d", kf.R, DefaultR)
	}
	if kf.P != DefaultP {
		t.Errorf("P = %d, want %d", kf.P, DefaultP)
	}
	if len(kf.Salt) != 64 {
		t.Errorf("salt length = %d, want 64", len(kf.Salt))
	}
	if len(kf.Data) == 0 {
		t.Error("encrypted data is empty")
	}

	// Open and recover the master key.
	recovered, err := OpenKeyFile(kf, password)
	if err != nil {
		t.Fatalf("OpenKeyFile: %v", err)
	}

	if recovered.EncryptKey != masterKey.EncryptKey {
		t.Error("recovered EncryptKey does not match original")
	}
	if recovered.IDKey != masterKey.IDKey {
		t.Error("recovered IDKey does not match original")
	}
}

func TestOpenKeyFileWrongPasswordReturnsError(t *testing.T) {
	masterKey, err := NewMasterKey()
	if err != nil {
		t.Fatalf("NewMasterKey: %v", err)
	}

	kf, err := CreateKeyFile(masterKey, "correct-password")
	if err != nil {
		t.Fatalf("CreateKeyFile: %v", err)
	}

	_, err = OpenKeyFile(kf, "wrong-password")
	if err == nil {
		t.Error("OpenKeyFile with wrong password should return an error")
	}
}

func TestNewMasterKeyProducesUniqueKeys(t *testing.T) {
	mk1, err := NewMasterKey()
	if err != nil {
		t.Fatalf("first NewMasterKey: %v", err)
	}

	mk2, err := NewMasterKey()
	if err != nil {
		t.Fatalf("second NewMasterKey: %v", err)
	}

	if mk1.EncryptKey == mk2.EncryptKey {
		t.Error("two NewMasterKey calls produced the same EncryptKey")
	}
	if mk1.IDKey == mk2.IDKey {
		t.Error("two NewMasterKey calls produced the same IDKey")
	}
}

func TestBlobID(t *testing.T) {
	var idKey [32]byte
	if _, err := rand.Read(idKey[:]); err != nil {
		t.Fatalf("generate idKey: %v", err)
	}

	data := []byte("some blob data")

	// Same input produces same output.
	id1 := BlobID(idKey, data)
	id2 := BlobID(idKey, data)
	if id1 != id2 {
		t.Error("BlobID is not deterministic for same inputs")
	}

	// Different data produces different output.
	id3 := BlobID(idKey, []byte("different blob data"))
	if id1 == id3 {
		t.Error("BlobID produced same result for different data")
	}

	// Different key produces different output.
	var otherKey [32]byte
	if _, err := rand.Read(otherKey[:]); err != nil {
		t.Fatalf("generate otherKey: %v", err)
	}
	id4 := BlobID(otherKey, data)
	if id1 == id4 {
		t.Error("BlobID produced same result for different keys")
	}

	// Result is 32 bytes.
	if len(id1) != 32 {
		t.Errorf("BlobID length = %d, want 32", len(id1))
	}
}

func TestBlobIDEmptyData(t *testing.T) {
	var idKey [32]byte
	if _, err := rand.Read(idKey[:]); err != nil {
		t.Fatalf("generate idKey: %v", err)
	}

	// Should work with empty data.
	id := BlobID(idKey, []byte{})
	var zero [32]byte
	if bytes.Equal(id[:], zero[:]) {
		t.Error("BlobID of empty data should not be all zeros")
	}
}
