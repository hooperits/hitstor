//go:build !windows

package pluginmgr

// seal is the identity outside Windows: the file is mode 0600 in a store
// readable only by the agent's account (root), which is as far as an
// unattended service can protect a secret without an external key store.
func seal(data []byte) ([]byte, error) { return data, nil }

func unseal(data []byte) ([]byte, error) { return data, nil }
