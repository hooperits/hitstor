package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	goplugin "github.com/hashicorp/go-plugin"

	pb "github.com/hooperits/hitstor/api/proto/hitstor/plugin/v1"
	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/hook"
	"github.com/hooperits/hitstor/internal/notifier"
	"github.com/hooperits/hitstor/internal/scanner"
)

// OpenStorageBackend discovers, launches, and configures a plugin backend.
//
// name:     plugin name (maps to binary "hitstor-backend-<name>")
// dir:      directory to search first (from plugin_dir config); empty means skip
// settings: key-value pairs passed to the plugin's Configure RPC
//
// The returned backend.Backend must be closed by the caller, which kills the
// plugin subprocess.
func OpenStorageBackend(name, dir string, settings map[string]string) (backend.Backend, error) {
	binaryName := "hitstor-backend-" + name
	binaryPath := findPlugin(binaryName, dir)
	if binaryPath == "" {
		return nil, fmt.Errorf("plugin: backend %q not found (searched for %q in %q and $PATH)", name, binaryName, dir)
	}

	slog.Info("launching backend plugin", "name", name, "path", binaryPath)

	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: HandshakeConfig,
		Plugins: map[string]goplugin.Plugin{
			StorageBackendPluginName: &StorageBackendPlugin{},
		},
		Cmd:              exec.Command(binaryPath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("plugin: connect to %s: %w", name, err)
	}

	raw, err := rpcClient.Dispense(StorageBackendPluginName)
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("plugin: dispense %s: %w", name, err)
	}

	grpcClient, ok := raw.(*StorageBackendGRPCClient)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("plugin: %s returned unexpected type %T", name, raw)
	}

	grpcClient.pluginClient = client

	if len(settings) > 0 {
		_, err := grpcClient.client.Configure(context.Background(), &pb.ConfigureRequest{
			Settings: settings,
		})
		if err != nil {
			client.Kill()
			return nil, fmt.Errorf("plugin: configure %s: %w", name, err)
		}
	}

	return grpcClient, nil
}

// OpenNotifier discovers, launches, and configures a notifier plugin.
//
// name:     plugin name (maps to binary "hitstor-notifier-<name>")
// dir:      directory to search first (from plugin_dir config); empty means skip
// settings: key-value pairs passed to the plugin's Configure RPC
//
// The returned notifier.Notifier must be closed by the caller, which kills the
// plugin subprocess.
func OpenNotifier(name, dir string, settings map[string]string) (notifier.Notifier, error) {
	binaryName := "hitstor-notifier-" + name
	binaryPath := findPlugin(binaryName, dir)
	if binaryPath == "" {
		return nil, fmt.Errorf("plugin: notifier %q not found (searched for %q in %q and $PATH)", name, binaryName, dir)
	}

	slog.Info("launching notifier plugin", "name", name, "path", binaryPath)

	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: HandshakeConfig,
		Plugins: map[string]goplugin.Plugin{
			NotifierPluginName: &NotifierPlugin{},
		},
		Cmd:              exec.Command(binaryPath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("plugin: connect to %s: %w", name, err)
	}

	raw, err := rpcClient.Dispense(NotifierPluginName)
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("plugin: dispense %s: %w", name, err)
	}

	grpcClient, ok := raw.(*NotifierGRPCClient)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("plugin: %s returned unexpected type %T", name, raw)
	}

	grpcClient.pluginClient = client

	if len(settings) > 0 {
		_, err := grpcClient.client.Configure(context.Background(), &pb.ConfigureRequest{
			Settings: settings,
		})
		if err != nil {
			client.Kill()
			return nil, fmt.Errorf("plugin: configure %s: %w", name, err)
		}
	}

	return grpcClient, nil
}

// OpenHook discovers, launches, and configures a hook plugin.
//
// name:     plugin name (maps to binary "hitstor-hook-<name>")
// dir:      directory to search first (from plugin_dir config); empty means skip
// settings: key-value pairs passed to the plugin's Configure RPC
//
// The returned hook.Hook must be closed by the caller, which kills the
// plugin subprocess.
func OpenHook(name, dir string, settings map[string]string) (hook.Hook, error) {
	binaryName := "hitstor-hook-" + name
	binaryPath := findPlugin(binaryName, dir)
	if binaryPath == "" {
		return nil, fmt.Errorf("plugin: hook %q not found (searched for %q in %q and $PATH)", name, binaryName, dir)
	}

	slog.Info("launching hook plugin", "name", name, "path", binaryPath)

	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: HandshakeConfig,
		Plugins: map[string]goplugin.Plugin{
			HookPluginName: &HookPlugin{},
		},
		Cmd:              exec.Command(binaryPath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("plugin: connect to %s: %w", name, err)
	}

	raw, err := rpcClient.Dispense(HookPluginName)
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("plugin: dispense %s: %w", name, err)
	}

	grpcClient, ok := raw.(*HookGRPCClient)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("plugin: %s returned unexpected type %T", name, raw)
	}

	grpcClient.pluginClient = client

	if len(settings) > 0 {
		_, err := grpcClient.client.Configure(context.Background(), &pb.ConfigureRequest{
			Settings: settings,
		})
		if err != nil {
			client.Kill()
			return nil, fmt.Errorf("plugin: configure %s: %w", name, err)
		}
	}

	return grpcClient, nil
}

// OpenFilesystemScanner discovers, launches, and configures a scanner plugin.
//
// name:     plugin name (maps to binary "hitstor-scanner-<name>")
// dir:      directory to search first (from plugin_dir config); empty means skip
// settings: key-value pairs passed to the plugin's Configure RPC
//
// The returned scanner.Scanner must be closed by the caller, which kills the
// plugin subprocess.
func OpenFilesystemScanner(name, dir string, settings map[string]string) (scanner.Scanner, error) {
	binaryName := "hitstor-scanner-" + name
	binaryPath := findPlugin(binaryName, dir)
	if binaryPath == "" {
		return nil, fmt.Errorf("plugin: scanner %q not found (searched for %q in %q and $PATH)", name, binaryName, dir)
	}

	slog.Info("launching scanner plugin", "name", name, "path", binaryPath)

	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: HandshakeConfig,
		Plugins: map[string]goplugin.Plugin{
			FilesystemScannerPluginName: &FilesystemScannerPlugin{},
		},
		Cmd:              exec.Command(binaryPath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("plugin: connect to %s: %w", name, err)
	}

	raw, err := rpcClient.Dispense(FilesystemScannerPluginName)
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("plugin: dispense %s: %w", name, err)
	}

	grpcClient, ok := raw.(*FilesystemScannerGRPCClient)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("plugin: %s returned unexpected type %T", name, raw)
	}

	grpcClient.pluginClient = client

	if len(settings) > 0 {
		_, err := grpcClient.client.Configure(context.Background(), &pb.ConfigureRequest{
			Settings: settings,
		})
		if err != nil {
			client.Kill()
			return nil, fmt.Errorf("plugin: configure %s: %w", name, err)
		}
	}

	return grpcClient, nil
}

// findPlugin searches for a plugin binary in the given directory, then $PATH.
func findPlugin(name, dir string) string {
	if dir != "" {
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}
