package repository

import (
	"encoding/hex"
	"fmt"

	"github.com/hooperits/hitstor/internal/backend"
)

// ID is an alias for backend.ID used throughout the repository package.
type ID = backend.ID

// BlobType represents the type of a blob stored in a pack file.
type BlobType string

const (
	DataBlob BlobType = "data"
	TreeBlob BlobType = "tree"
)

// Blob represents a blob stored in the repository.
type Blob struct {
	ID                 ID       `json:"id"`
	Type               BlobType `json:"type"`
	Length             int      `json:"length"`
	UncompressedLength int      `json:"uncompressed_length"`
}

// PackedBlob extends Blob with its location within a pack file.
type PackedBlob struct {
	ID                 ID       `json:"id"`
	Type               BlobType `json:"type"`
	Offset             int      `json:"offset"`
	Length             int      `json:"length"`
	UncompressedLength int      `json:"uncompressed_length"`
}

// IDFromHex parses a hex-encoded string into an ID.
func IDFromHex(s string) (ID, error) {
	var id ID
	b, err := hex.DecodeString(s)
	if err != nil {
		return id, err
	}
	if len(b) != 32 {
		return id, fmt.Errorf("invalid ID length: %d", len(b))
	}
	copy(id[:], b)
	return id, nil
}
