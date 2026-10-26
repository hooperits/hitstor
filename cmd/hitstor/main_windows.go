//go:build windows

package main

import (
	"context"
	"log/slog"

	"github.com/hooperits/hitstor/internal/winsvc"
)

// runAsService checks if the process is running as a Windows Service.
// If so, it runs the command line the service was registered with
// (normally `serve --config ...`) under the service handler and returns true.
// Otherwise it returns false and the CLI takes over.
func runAsService() bool {
	if !winsvc.IsService() {
		return false
	}

	err := winsvc.Run(defaultServiceName, func(ctx context.Context) error {
		// The service stop request cancels ctx; runServe shuts down on it.
		if err := rootCmd.ExecuteContext(ctx); err != nil {
			slog.Error("service command failed", "error", err)
			return err
		}
		return nil
	})
	if err != nil {
		slog.Error("service run failed", "error", err)
	}
	return true
}
