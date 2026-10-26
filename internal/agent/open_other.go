//go:build !windows

package agent

import "os"

// openFileForBackup opens a file for reading. On non-Windows platforms this
// is equivalent to os.Open. The Windows implementation uses backup semantics
// to bypass ACL restrictions.
func openFileForBackup(path string) (*os.File, error) {
	return os.Open(path)
}
