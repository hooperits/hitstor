package repository

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/hooperits/hitstor/internal/crypto"
)

// PackWriter accumulates encrypted blobs and produces a complete pack file.
//
// Pack file format:
//
//	[blob1_encrypted][blob2_encrypted]...[header_encrypted][header_length_4bytes]
type PackWriter struct {
	buf   bytes.Buffer // accumulates blob data
	blobs []PackedBlob // tracks blob metadata
	key   [32]byte     // encryption key (MasterKey.EncryptKey)
}

// NewPackWriter creates a new PackWriter with the given encryption key.
func NewPackWriter(encryptKey [32]byte) *PackWriter {
	return &PackWriter{
		key: encryptKey,
	}
}

// AddBlob encrypts the plaintext with AES-256-GCM and appends it to the pack
// buffer. It records the blob's metadata (offset, length of encrypted data) for
// later inclusion in the pack header.
func (pw *PackWriter) AddBlob(id ID, blobType BlobType, plaintext []byte, uncompressedLen int) error {
	encrypted, err := crypto.Encrypt(pw.key, plaintext)
	if err != nil {
		return fmt.Errorf("pack: encrypt blob: %w", err)
	}

	offset := pw.buf.Len()
	length := len(encrypted)

	pw.buf.Write(encrypted)

	pw.blobs = append(pw.blobs, PackedBlob{
		ID:                 id,
		Type:               blobType,
		Offset:             offset,
		Length:             length,
		UncompressedLength: uncompressedLen,
	})

	return nil
}

// Finalize builds the complete pack file by appending the encrypted header and
// a 4-byte big-endian header length. It returns the pack ID (SHA-256 of the
// complete data) and the raw pack bytes.
func (pw *PackWriter) Finalize() (packID ID, data []byte, err error) {
	// Serialize header as JSON array of PackedBlob entries.
	headerJSON, err := json.Marshal(pw.blobs)
	if err != nil {
		return ID{}, nil, fmt.Errorf("pack: marshal header: %w", err)
	}

	// Encrypt the header.
	encryptedHeader, err := crypto.Encrypt(pw.key, headerJSON)
	if err != nil {
		return ID{}, nil, fmt.Errorf("pack: encrypt header: %w", err)
	}

	// Append encrypted header to buffer.
	pw.buf.Write(encryptedHeader)

	// Append 4-byte big-endian header length.
	var headerLen [4]byte
	binary.BigEndian.PutUint32(headerLen[:], uint32(len(encryptedHeader)))
	pw.buf.Write(headerLen[:])

	// Compute SHA-256 of the complete pack data as the pack ID.
	data = pw.buf.Bytes()
	hash := sha256.Sum256(data)
	packID = hash

	return packID, data, nil
}

// Count returns the number of blobs added to the pack.
func (pw *PackWriter) Count() int {
	return len(pw.blobs)
}

// Size returns the current size of the accumulated pack data (blob data only,
// before finalization).
func (pw *PackWriter) Size() int {
	return pw.buf.Len()
}

// ReadPackHeader reads and decrypts the header from a pack file. It reads the
// last 4 bytes as a big-endian uint32 header length, extracts the encrypted
// header, decrypts it, and unmarshals it into a slice of PackedBlob.
func ReadPackHeader(encryptKey [32]byte, data []byte) ([]PackedBlob, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("pack: data too short to contain header length")
	}

	// Read header length from last 4 bytes.
	headerLen := binary.BigEndian.Uint32(data[len(data)-4:])

	// Validate header length.
	if int(headerLen) > len(data)-4 {
		return nil, fmt.Errorf("pack: header length %d exceeds available data %d", headerLen, len(data)-4)
	}

	// Extract encrypted header.
	headerStart := len(data) - 4 - int(headerLen)
	encryptedHeader := data[headerStart : len(data)-4]

	// Decrypt header.
	headerJSON, err := crypto.Decrypt(encryptKey, encryptedHeader)
	if err != nil {
		return nil, fmt.Errorf("pack: decrypt header: %w", err)
	}

	// Unmarshal header.
	var blobs []PackedBlob
	if err := json.Unmarshal(headerJSON, &blobs); err != nil {
		return nil, fmt.Errorf("pack: unmarshal header: %w", err)
	}

	return blobs, nil
}

// ReadPackBlob extracts and decrypts a single blob from the pack data using the
// offset and length from the PackedBlob metadata.
func ReadPackBlob(encryptKey [32]byte, data []byte, blob PackedBlob) ([]byte, error) {
	end := blob.Offset + blob.Length
	if end > len(data) {
		return nil, fmt.Errorf("pack: blob extends beyond pack data (offset=%d, length=%d, pack size=%d)",
			blob.Offset, blob.Length, len(data))
	}

	encrypted := data[blob.Offset:end]

	plaintext, err := crypto.Decrypt(encryptKey, encrypted)
	if err != nil {
		return nil, fmt.Errorf("pack: decrypt blob: %w", err)
	}

	return plaintext, nil
}
