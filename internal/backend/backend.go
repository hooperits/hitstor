package backend

import (
	"context"
	"encoding/hex"
	"io"
)

// FileType represents the type of file stored in the backend.
type FileType int

const (
	DataFile FileType = iota
	IndexFile
	SnapshotFile
	KeyFile
	LockFile
	ConfigFile
)

// String returns the directory name used for this file type.
func (ft FileType) String() string {
	switch ft {
	case DataFile:
		return "data"
	case IndexFile:
		return "index"
	case SnapshotFile:
		return "snapshots"
	case KeyFile:
		return "keys"
	case LockFile:
		return "locks"
	case ConfigFile:
		return "config"
	default:
		return "unknown"
	}
}

// ID is a 32-byte identifier used to address objects in the backend.
type ID [32]byte

// String returns the hex-encoded representation of the ID.
func (id ID) String() string {
	return hex.EncodeToString(id[:])
}

// FileInfo contains metadata about a file stored in the backend.
type FileInfo struct {
	Size int64
}

// Backend defines the interface for storage backends.
type Backend interface {
	// Save stores data from rd under the given file type and ID.
	Save(ctx context.Context, ft FileType, id ID, rd io.Reader) error

	// Load retrieves the data for the given file type and ID.
	Load(ctx context.Context, ft FileType, id ID) (io.ReadCloser, error)

	// Stat returns metadata about the file for the given file type and ID.
	Stat(ctx context.Context, ft FileType, id ID) (FileInfo, error)

	// Remove deletes the file for the given file type and ID.
	Remove(ctx context.Context, ft FileType, id ID) error

	// List returns all IDs stored for the given file type.
	List(ctx context.Context, ft FileType) ([]ID, error)

	// Close releases any resources held by the backend.
	Close() error
}
