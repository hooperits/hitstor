package plugin

import (
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/hook"
	"github.com/hooperits/hitstor/internal/notifier"
	"github.com/hooperits/hitstor/internal/scanner"
)

// ServeStorageBackend starts the go-plugin gRPC server for a storage backend.
// Plugin binaries call this from main() with their backend.Backend implementation.
func ServeStorageBackend(impl backend.Backend) {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: HandshakeConfig,
		Plugins: map[string]goplugin.Plugin{
			StorageBackendPluginName: &StorageBackendPlugin{Impl: impl},
		},
		GRPCServer: func(opts []grpc.ServerOption) *grpc.Server {
			return grpc.NewServer(opts...)
		},
	})
}

// ServeNotifier starts the go-plugin gRPC server for a notifier.
// Plugin binaries call this from main() with their notifier.Notifier implementation.
func ServeNotifier(impl notifier.Notifier) {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: HandshakeConfig,
		Plugins: map[string]goplugin.Plugin{
			NotifierPluginName: &NotifierPlugin{Impl: impl},
		},
		GRPCServer: func(opts []grpc.ServerOption) *grpc.Server {
			return grpc.NewServer(opts...)
		},
	})
}

// ServeHook starts the go-plugin gRPC server for a hook.
// Plugin binaries call this from main() with their hook.Hook implementation.
func ServeHook(impl hook.Hook) {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: HandshakeConfig,
		Plugins: map[string]goplugin.Plugin{
			HookPluginName: &HookPlugin{Impl: impl},
		},
		GRPCServer: func(opts []grpc.ServerOption) *grpc.Server {
			return grpc.NewServer(opts...)
		},
	})
}

// ServeFilesystemScanner starts the go-plugin gRPC server for a filesystem scanner.
// Plugin binaries call this from main() with their scanner.Scanner implementation.
func ServeFilesystemScanner(impl scanner.Scanner) {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: HandshakeConfig,
		Plugins: map[string]goplugin.Plugin{
			FilesystemScannerPluginName: &FilesystemScannerPlugin{Impl: impl},
		},
		GRPCServer: func(opts []grpc.ServerOption) *grpc.Server {
			return grpc.NewServer(opts...)
		},
	})
}
