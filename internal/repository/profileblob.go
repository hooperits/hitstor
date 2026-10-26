package repository

import (
	"encoding/json"
	"fmt"

	"github.com/hooperits/hitstor/internal/compress"
	"github.com/hooperits/hitstor/internal/crypto"
)

// EncodeSystemProfile serializes a system profile for storage as a data blob,
// using the same content-addressed, compressed representation as tree blobs.
//
// The profile is supplied as an already-marshalled value so this package does
// not depend on internal/sysprofile, keeping the dependency pointing one way.
func EncodeSystemProfile(idKey [32]byte, profile any) (blobID ID, compressedData []byte, uncompressedLen int, err error) {
	jsonBytes, err := json.Marshal(profile)
	if err != nil {
		return ID{}, nil, 0, fmt.Errorf("repository: marshal system profile: %w", err)
	}

	blobID = ID(crypto.BlobID(idKey, jsonBytes))

	compressedData, err = compress.Compress(jsonBytes)
	if err != nil {
		return ID{}, nil, 0, fmt.Errorf("repository: compress system profile: %w", err)
	}

	return blobID, compressedData, len(jsonBytes), nil
}

// LoadSystemProfile reads the system-profile blob referenced by a snapshot and
// decodes it into out, which must be a pointer.
func (r *Repository) LoadSystemProfile(blobID ID, out any) error {
	blob, packID := r.LookupBlob(blobID)
	if blob == nil {
		return fmt.Errorf("repository: system profile blob %s not found in index", blobID)
	}

	compressedData, err := r.LoadBlob(*packID, *blob)
	if err != nil {
		return fmt.Errorf("repository: load system profile blob: %w", err)
	}

	jsonBytes, err := compress.Decompress(compressedData)
	if err != nil {
		return fmt.Errorf("repository: decompress system profile: %w", err)
	}

	if err := json.Unmarshal(jsonBytes, out); err != nil {
		return fmt.Errorf("repository: decode system profile: %w", err)
	}
	return nil
}
