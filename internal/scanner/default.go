package scanner

import (
	"fmt"
	"os"

	"github.com/hooperits/hitstor/internal/fs"
)

// DefaultScanner implements Scanner using the built-in fs.Walk function.
type DefaultScanner struct {
	// Xattrs captures extended attributes into each node. Off by default so
	// ordinary backups do not pay two extra syscalls per entry.
	Xattrs bool
}

// NewDefault returns a new DefaultScanner.
func NewDefault() *DefaultScanner {
	return &DefaultScanner{}
}

// NewDefaultWithXattrs returns a DefaultScanner that also captures extended
// attributes, as required for bare-metal restore.
func NewDefaultWithXattrs() *DefaultScanner {
	return &DefaultScanner{Xattrs: true}
}

// Scan walks the given paths with exclude patterns and sends results on a channel.
func (s *DefaultScanner) Scan(paths []string, excludes []string) (<-chan ScanResult, error) {
	// Validate all paths exist before starting the scan.
	for _, p := range paths {
		_, err := os.Lstat(p)
		if err != nil {
			return nil, fmt.Errorf("scanner: path %s: %w", p, err)
		}
	}

	ch := make(chan ScanResult)

	go func() {
		defer close(ch)

		for _, root := range paths {
			walkOpts := fs.WalkOptions{Excludes: excludes, Xattrs: s.Xattrs}
			err := fs.WalkWithOptions(root, walkOpts, func(path string, node fs.Node) error {
				ch <- ScanResult{
					Path: path,
					Node: node,
				}
				return nil
			})
			if err != nil {
				ch <- ScanResult{
					Err: fmt.Errorf("scanner: walk %s: %w", root, err),
				}
			}
		}
	}()

	return ch, nil
}

// Close is a no-op for the default scanner.
func (s *DefaultScanner) Close() error { return nil }
