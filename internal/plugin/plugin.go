package plugin

import (
	goplugin "github.com/hashicorp/go-plugin"
)

// HandshakeConfig is shared between the host and every HitStor plugin binary.
// The magic cookie prevents accidental execution of non-plugin binaries.
var HandshakeConfig = goplugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "HITSTOR_PLUGIN",
	MagicCookieValue: "hitstor-v1",
}

// Plugin type names used in the go-plugin negotiation map.
const (
	StorageBackendPluginName    = "storage_backend"
	NotifierPluginName          = "notifier"
	HookPluginName              = "hook"
	FilesystemScannerPluginName = "filesystem_scanner"
)

// Configurable is an optional interface that plugins can implement
// to receive settings via the Configure RPC.
type Configurable interface {
	Configure(settings map[string]string) error
}
