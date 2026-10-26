package repository

import (
	"encoding/json"
	"fmt"

	"github.com/hooperits/hitstor/internal/crypto"
)

// IndexPack describes one pack file and the blobs it contains.
type IndexPack struct {
	PackID ID           `json:"pack_id"`
	Blobs  []PackedBlob `json:"blobs"`
}

// Index is the in-memory representation of a repository index.
// It maps blob IDs to their location (pack file + offset/length).
type Index struct {
	Packs []IndexPack `json:"packs"`
}

// EncodeIndex serializes the index to JSON, then encrypts with the given key.
func EncodeIndex(encryptKey [32]byte, idx *Index) ([]byte, error) {
	plaintext, err := json.Marshal(idx)
	if err != nil {
		return nil, fmt.Errorf("encode index: marshal: %w", err)
	}

	ciphertext, err := crypto.Encrypt(encryptKey, plaintext)
	if err != nil {
		return nil, fmt.Errorf("encode index: encrypt: %w", err)
	}

	return ciphertext, nil
}

// DecodeIndex decrypts, then deserializes an Index from encrypted JSON.
func DecodeIndex(encryptKey [32]byte, data []byte) (*Index, error) {
	plaintext, err := crypto.Decrypt(encryptKey, data)
	if err != nil {
		return nil, fmt.Errorf("decode index: decrypt: %w", err)
	}

	var idx Index
	if err := json.Unmarshal(plaintext, &idx); err != nil {
		return nil, fmt.Errorf("decode index: unmarshal: %w", err)
	}

	return &idx, nil
}
