//go:build windows

package pluginmgr

import "golang.org/x/sys/windows"

// storeSDDL grants full control to SYSTEM and Administrators only, inherited
// by everything below, with inheritance from the parent blocked. Plugins run
// as the agent (LocalSystem), so a writable store would let any local user
// plant code that runs as SYSTEM.
const storeSDDL = "D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

func hardenDir(dir string) error {
	sd, err := windows.SecurityDescriptorFromString(storeSDDL)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil)
}
