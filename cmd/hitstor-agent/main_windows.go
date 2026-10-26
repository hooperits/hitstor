//go:build windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/hooperits/hitstor/internal/agent"
	"github.com/hooperits/hitstor/internal/winsvc"
)

// runAsService checks if the process is running as a Windows Service.
// If so, it enters the service handler loop and returns true.
// Otherwise it returns false and the CLI takes over.
func runAsService() bool {
	if !winsvc.IsService() {
		return false
	}

	if err := winsvc.Run("HitStorAgent", serviceBackup); err != nil {
		slog.Error("service run failed", "error", err)
	}
	return true
}

// serviceBackup is the main function run when executing as a Windows Service.
// It runs the full daemon: agent gRPC management server + backup polling loop.
func serviceBackup(ctx context.Context) error {
	// Load config (same as CLI PersistentPreRunE).
	loadConfig()
	setupLogging()

	// Load credentials from agent.env (Windows doesn't have systemd EnvironmentFile).
	loadEnvFile(agent.DefaultEnvPath)

	pollIntervalFlag = defaultPollInterval

	// Check listen port from environment variable.
	listenPort := defaultListenPort
	if env := os.Getenv("HITSTOR_AGENT_LISTEN_PORT"); env != "" {
		if _, err := fmt.Sscanf(env, "%d", &listenPort); err != nil {
			slog.Error("invalid HITSTOR_AGENT_LISTEN_PORT", "value", env)
		}
	}

	// Load or generate the agent token.
	if err := os.MkdirAll(agent.DefaultConfigDir, 0755); err != nil {
		slog.Error("create config dir", "error", err)
	}
	token, err := agent.LoadToken(agent.DefaultTokenPath)
	if err != nil {
		slog.Info("no agent token found, generating new one")
		token, err = agent.GenerateToken(agent.DefaultTokenPath)
		if err != nil {
			return fmt.Errorf("generate agent token: %w", err)
		}
		slog.Info("agent token generated", "path", agent.DefaultTokenPath)
	}

	// Start the agent-side gRPC management server.
	agentSrv := agent.NewAgentServer(agent.AgentServerOptions{
		Token:      token,
		EnvPath:    agent.DefaultEnvPath,
		ConfigPath: configFlag,
		Version:    Version,
	})

	// When the server registers us, start the backup loop.
	registerCh := make(chan struct{}, 1)
	agentSrv.OnRegister = func(serverAddr, apiKey, caCertPath string) {
		serverFlag = serverAddr
		apiKeyFlag = apiKey
		if caCertPath != "" {
			caCertFlag = caCertPath
		}
		select {
		case registerCh <- struct{}{}:
		default:
		}
	}

	// Start gRPC management server in a goroutine.
	svcCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		if err := agentSrv.Start(listenPort); err != nil {
			slog.Error("agent gRPC server failed", "error", err)
			cancel()
		}
	}()

	// Auto-detect server CA cert if not explicitly set.
	if caCertFlag == "" {
		defaultCACert := filepath.Join(agent.DefaultConfigDir, "server-ca.crt")
		if _, err := os.Stat(defaultCACert); err == nil {
			caCertFlag = defaultCACert
		}
	}

	// Check if already configured (legacy install or previous registration).
	configured := serverFlag != "" && apiKeyFlag != ""

	wakeUpCh := agentSrv.WakeUpCh

	if configured {
		slog.Info("service daemon starting",
			"server", serverFlag,
			"poll_interval", pollIntervalFlag,
			"listen_port", listenPort,
		)
		go daemonLoop(svcCtx, wakeUpCh)
	} else {
		slog.Info("agent waiting for registration from dashboard",
			"listen_port", listenPort,
			"token_hint", token[:8]+"...",
		)
	}

	// Wait for service stop or registration.
	for {
		select {
		case <-svcCtx.Done():
			agentSrv.Stop()
			return nil
		case <-registerCh:
			loadConfig()
			slog.Info("registration complete, starting backup loop",
				"server", serverFlag,
			)
			go daemonLoop(svcCtx, wakeUpCh)
		}
	}
}
