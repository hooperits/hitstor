//go:build windows

package pluginmgr

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// sealEntropy binds sealed data to this use, so another DPAPI blob cannot
// be swapped in.
var sealEntropy = []byte("hitstor plugin object settings")

// seal protects data with DPAPI for the local machine: the agent service
// (LocalSystem) and the agent CLI run by an administrator can both open it,
// and the store's ACL keeps everyone else from reading the file.
func seal(data []byte) ([]byte, error) {
	return dpapi(data, true)
}

func unseal(data []byte) ([]byte, error) {
	return dpapi(data, false)
}

func dpapi(data []byte, protect bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	entropy := windows.DataBlob{Size: uint32(len(sealEntropy)), Data: &sealEntropy[0]}
	var out windows.DataBlob
	var err error
	if protect {
		err = windows.CryptProtectData(&in, nil, &entropy, 0, nil,
			windows.CRYPTPROTECT_LOCAL_MACHINE|windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptUnprotectData(&in, nil, &entropy, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
