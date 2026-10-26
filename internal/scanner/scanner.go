// Package scanner defines the filesystem scanning interface and default
// implementation for the HitStor backup tool. Plugin scanners implement
// the Scanner interface to provide custom directory traversal logic.
package scanner

import "github.com/hooperits/hitstor/internal/fs"

// ScanResult represents a single entry from the filesystem scan.
type ScanResult struct {
	Path string  // Absolute path to the file/dir/symlink.
	Node fs.Node // Metadata for the entry.
	Err  error   // Non-nil if there was an error reading this entry.
}

// Scanner is the filesystem scanning interface. Plugins implement this
// to provide custom directory traversal (e.g., cloud storage, filtered scans).
type Scanner interface {
	// Scan walks the given paths with exclude patterns and sends ScanResult
	// entries on the returned channel. The channel is closed when scanning
	// is complete. Errors for individual files are sent as ScanResults with
	// non-nil Err (scanning continues). A fatal error returns from Scan itself.
	Scan(paths []string, excludes []string) (<-chan ScanResult, error)

	// Close releases resources held by the scanner.
	Close() error
}
