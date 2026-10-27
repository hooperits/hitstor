//go:build !windows

package pluginmgr

import "os"

// hardenDir limits the store to its owner (root for an installed agent).
// Plugins run as the agent, so a writable store would let other local users
// plant code that runs with the agent's privileges.
func hardenDir(dir string) error {
	return os.Chmod(dir, 0o700)
}
