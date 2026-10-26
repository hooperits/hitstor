//go:build windows

package local

import "os"

// AcquireFlock is a no-op on Windows; JSON lock files remain the coordination mechanism.
func (l *Local) AcquireFlock() (*os.File, error) {
	return nil, nil
}
