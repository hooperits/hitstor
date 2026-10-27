//go:build windows

package agent

import "golang.org/x/sys/windows"

// configDirSDDL grants full control to SYSTEM and Administrators only,
// inherited by everything inside, with inheritance from ProgramData blocked.
// The directory holds the agent token, the TLS private key, and agent.env
// (API key, plugin secrets such as database passwords).
const configDirSDDL = "D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

// SecureConfigDir restricts the agent's config directory to SYSTEM and
// Administrators. Existing files inherit the new permissions.
func SecureConfigDir() error {
	sd, err := windows.SecurityDescriptorFromString(configDirSDDL)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(DefaultConfigDir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil)
}
