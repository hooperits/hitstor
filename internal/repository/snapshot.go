package repository

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hooperits/hitstor/internal/crypto"
)

// SnapshotSummary contains statistics about a backup snapshot.
type SnapshotSummary struct {
	FilesNew        uint64  `json:"files_new"`
	FilesUnchanged  uint64  `json:"files_unchanged"`
	DirsNew         uint64  `json:"dirs_new"`
	BytesAdded      uint64  `json:"bytes_added"`
	BytesProcessed  uint64  `json:"bytes_processed"`
	TotalFiles      uint64  `json:"total_files"`
	TotalSize       uint64  `json:"total_size"`
	TotalDuration   float64 `json:"total_duration"`
	SuspiciousFiles uint64  `json:"suspicious_files,omitempty"`
}

// Snapshot represents a point-in-time backup snapshot.
type Snapshot struct {
	Time           time.Time       `json:"time"`
	Hostname       string          `json:"hostname"`
	Username       string          `json:"username"`
	Paths          []string        `json:"paths"`
	Excludes       []string        `json:"excludes,omitempty"`
	Tree           ID              `json:"tree"`
	Tags           []string        `json:"tags,omitempty"`
	Summary        SnapshotSummary `json:"summary"`
	Type           string          `json:"type,omitempty"`
	ParentSnapshot string          `json:"parent_snapshot,omitempty"`

	// SystemProfile references a data blob holding the host's storage and boot
	// layout at backup time (see internal/sysprofile). It makes the snapshot
	// self-describing enough to rebuild a bootable machine, rather than only
	// its file contents.
	//
	// Optional: snapshots taken without layout capture omit it.
	SystemProfile ID `json:"system_profile,omitempty"`
}

// EncodeSnapshot serializes the snapshot to JSON, then encrypts.
func EncodeSnapshot(encryptKey [32]byte, snap *Snapshot) ([]byte, error) {
	jsonData, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("encode snapshot: marshal: %w", err)
	}

	ciphertext, err := crypto.Encrypt(encryptKey, jsonData)
	if err != nil {
		return nil, fmt.Errorf("encode snapshot: encrypt: %w", err)
	}

	return ciphertext, nil
}

// DecodeSnapshot decrypts, then deserializes a Snapshot.
func DecodeSnapshot(encryptKey [32]byte, data []byte) (*Snapshot, error) {
	plaintext, err := crypto.Decrypt(encryptKey, data)
	if err != nil {
		return nil, fmt.Errorf("decode snapshot: decrypt: %w", err)
	}

	var snap Snapshot
	if err := json.Unmarshal(plaintext, &snap); err != nil {
		return nil, fmt.Errorf("decode snapshot: unmarshal: %w", err)
	}

	return &snap, nil
}
