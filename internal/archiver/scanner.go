package archiver

import "github.com/hooperits/hitstor/internal/scanner"

// ScanResult is an alias for scanner.ScanResult for backward compatibility.
type ScanResult = scanner.ScanResult

// Scan walks the given paths (with exclude patterns) and sends ScanResult
// entries on the returned channel. The channel is closed when scanning is
// complete. Errors for individual files are sent as ScanResults with non-nil
// Err (scanning continues). A fatal error (e.g., root path doesn't exist)
// returns an error from Scan itself.
func Scan(paths []string, excludes []string) (<-chan ScanResult, error) {
	return scanner.NewDefault().Scan(paths, excludes)
}
