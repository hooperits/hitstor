//go:build windows

package agent

// setOwnership is a no-op on Windows. NTFS uses ACLs instead of POSIX
// uid/gid ownership, and the original ACLs are not preserved in backups.
func setOwnership(_ string, _, _ uint32) {}
