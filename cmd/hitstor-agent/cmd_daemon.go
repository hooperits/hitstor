package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"

	"github.com/hooperits/hitstor/internal/agent"
	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
	"github.com/spf13/cobra"
)

const (
	defaultPollInterval = 5 * time.Minute
	minPollInterval     = 1 * time.Minute
	defaultListenPort   = 8422
)

var (
	pollIntervalFlag time.Duration
	listenPortFlag   int
)

func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run as a long-lived daemon with management server and backup polling",
		Long: `Start the agent in daemon mode. The agent listens on a gRPC port for
management commands from the HitStor server (registration, filesystem browsing,
configuration). If already registered (has server address and API key), it also
polls the server for backup schedule and runs backups automatically.`,
		RunE: runDaemon,
	}
	cmd.Flags().DurationVar(&pollIntervalFlag, "poll-interval", defaultPollInterval,
		"how often to check the server for schedule updates")
	cmd.Flags().IntVar(&listenPortFlag, "listen-port", defaultListenPort,
		"gRPC listen port for server management connections")
	return cmd
}

func runDaemon(cmd *cobra.Command, args []string) error {
	initAgentLogStore(loadedConfig)
	if pollIntervalFlag < minPollInterval {
		pollIntervalFlag = minPollInterval
	}

	// Load credentials from agent.env (Windows doesn't have systemd EnvironmentFile).
	loadEnvFile(agent.DefaultEnvPath)

	// Check listen port from environment variable if not set via flag.
	if !cmd.Flags().Changed("listen-port") {
		if env := os.Getenv("HITSTOR_AGENT_LISTEN_PORT"); env != "" {
			if _, err := fmt.Sscanf(env, "%d", &listenPortFlag); err != nil {
				return fmt.Errorf("invalid HITSTOR_AGENT_LISTEN_PORT: %s", env)
			}
		}
	}

	// Load or generate the agent token.
	token, err := agent.LoadToken(agent.DefaultTokenPath)
	if err != nil {
		slog.Info("no agent token found, generating new one")
		token, err = agent.GenerateToken(agent.DefaultTokenPath)
		if err != nil {
			return fmt.Errorf("generate agent token: %w", err)
		}
		slog.Info("agent token generated", "path", agent.DefaultTokenPath)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, shutdownSignals...)

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
	go func() {
		if err := agentSrv.Start(listenPortFlag); err != nil {
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
		slog.Info("daemon starting",
			"server", serverFlag,
			"poll_interval", pollIntervalFlag,
			"listen_port", listenPortFlag,
		)
		go daemonLoop(ctx, wakeUpCh)
	} else {
		slog.Info("agent waiting for registration from dashboard",
			"listen_port", listenPortFlag,
			"token_hint", token[:8]+"...",
		)
	}

	// Wait for shutdown signal or registration.
	for {
		select {
		case <-sigCh:
			slog.Info("received shutdown signal, stopping daemon")
			cancel()
			agentSrv.Stop()
			return nil
		case <-registerCh:
			loadConfig()
			slog.Info("registration complete, starting backup loop",
				"server", serverFlag,
			)
			go daemonLoop(ctx, wakeUpCh)
		case <-ctx.Done():
			agentSrv.Stop()
			return nil
		}
	}
}

func daemonLoop(ctx context.Context, wakeUpCh <-chan struct{}) {
	ticker := time.NewTicker(pollIntervalFlag)
	defer ticker.Stop()

	// Check immediately on startup.
	checkAndRunAll(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("daemon stopped")
			return
		case <-ticker.C:
			checkAndRunAll(ctx)
		case <-wakeUpCh:
			slog.Info("wakeup received, checking schedule immediately")
			checkAndRunAll(ctx)
		}
	}
}

func checkAndRunAll(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	client, err := agent.NewClient(agent.ClientOptions{
		ServerAddr: serverFlag,
		CACertPath: caCertFlag,
		PinPath:    agent.DefaultServerPinPath,
	})
	if err != nil {
		slog.Error("failed to connect to server", "error", err)
		return
	}
	defer client.Close()

	// Query schedule.
	schedCtx, schedCancel := context.WithTimeout(ctx, 30*time.Second)
	schedule, err := client.GetSchedule(schedCtx, apiKeyFlag)
	schedCancel()
	if err != nil {
		slog.Error("failed to get schedule", "error", err)
		return
	}

	// Multi-job mode: server returned backup jobs.
	if len(schedule.Jobs) > 0 {
		now := time.Now()
		for _, job := range schedule.Jobs {
			if !agent.AgentJobRunnable(job) {
				continue
			}
			// RunNow overrides scheduling — run immediately.
			if job.RunNow {
				slog.Info("manual backup requested (run now)",
					"job", job.Name,
					"job_id", job.JobID,
				)
				runJobBackup(ctx, client, job)
				continue
			}
			if !job.Enabled {
				continue
			}
			if job.NextRunUnix > 0 && now.Unix() < job.NextRunUnix {
				nextRun := time.Unix(job.NextRunUnix, 0)
				slog.Debug("job not due yet",
					"job", job.Name,
					"next_run", nextRun.Format(time.RFC3339),
					"wait", time.Until(nextRun).Round(time.Second),
				)
				continue
			}
			runJobBackup(ctx, client, job)
		}
		return
	}

	// Legacy fallback: single schedule with local config paths.
	if !schedule.Enabled {
		slog.Debug("scheduling disabled for this agent")
		return
	}

	paths := loadedConfig.AgentPaths
	if len(paths) == 0 {
		slog.Debug("no backup paths configured locally and no jobs from server")
		return
	}

	now := time.Now()
	if schedule.NextRunUnix > 0 && now.Unix() < schedule.NextRunUnix {
		nextRun := time.Unix(schedule.NextRunUnix, 0)
		slog.Debug("next backup not due yet",
			"next_run", nextRun.Format(time.RFC3339),
			"wait", time.Until(nextRun).Round(time.Second),
		)
		return
	}

	slog.Info("starting scheduled backup (legacy)",
		"paths", paths,
		"interval_hours", schedule.IntervalHours,
	)

	parallel := runtime.NumCPU()
	if parallel > 8 {
		parallel = 8
	}

	b := &agent.Backup{
		Client:   client,
		APIKey:   apiKeyFlag,
		Paths:    paths,
		Excludes: loadedConfig.AgentExcludes,
		Hostname: loadedConfig.AgentHostname,
		Parallel: parallel,
	}

	result, err := b.Run(ctx)
	if err != nil {
		slog.Error("scheduled backup failed", "error", err)
		return
	}

	slog.Info("scheduled backup complete",
		"snapshot", result.SnapshotID,
		"files_new", result.FilesNew,
		"files_unchanged", result.FilesUnchanged,
		"files_skipped", result.FilesSkipped,
		"bytes_added", result.BytesAdded,
		"duration", result.Duration,
	)
	if result.FilesSkipped > 0 {
		slog.Warn("some files could not be backed up (enable consistent snapshots / VSS for open files)", "files_skipped", result.FilesSkipped)
	}
}

// runJobBackup runs a single backup for a server-managed backup job.
func runJobBackup(ctx context.Context, client *agent.Client, job agent.JobSchedule) {
	slog.Info("starting job backup",
		"job_id", job.JobID,
		"job_name", job.Name,
		"job_type", job.JobType,
		"paths", job.Paths,
		"interval_hours", job.IntervalHours,
	)

	parallel := runtime.NumCPU()
	if parallel > 8 {
		parallel = 8
	}

	b := &agent.Backup{
		Client:   client,
		APIKey:   apiKeyFlag,
		Paths:    job.Paths,
		Excludes: job.Excludes,
		Hostname: loadedConfig.AgentHostname,
		JobID:    job.JobID,
		Tags:     []string{fmt.Sprintf("job:%s", job.JobID)},
		Parallel: parallel,
		UseVSS:   job.UseVSS,
	}

	if job.UseVSS {
		slog.Info("consistent snapshot mode enabled for job", "job_id", job.JobID)
	}

	// Prefer the shipped filesystem scanner plugin (hitstor-scanner-builtin).
	// Falls back to in-process walker if the plugin cannot be loaded.
	// Database/SaaS job types are out of product scope (filesystem only).
	if job.JobType != "" && job.JobType != "files" {
		slog.Error("unsupported job type for filesystem agent",
			"job_id", job.JobID,
			"job_type", job.JobType,
		)
		return
	}
	scannerName := loadedConfig.FsScannerPlugin
	if scannerName == "" || scannerName == "builtin" || scannerName == "fs" {
		if s, err := hitstorplugin.OpenFilesystemScanner("builtin", loadedConfig.PluginDir, nil); err == nil {
			b.Scanner = s
			slog.Info("using builtin filesystem scanner plugin", "job_id", job.JobID)
		} else if scannerName != "" {
			slog.Warn("builtin scanner plugin not found, using in-process walker", "error", err)
		}
	} else if scannerName != "" {
		if s, err := hitstorplugin.OpenFilesystemScanner(scannerName, loadedConfig.PluginDir, nil); err == nil {
			b.Scanner = s
			slog.Info("using configured filesystem scanner plugin", "name", scannerName, "job_id", job.JobID)
		} else {
			slog.Error("failed to load configured scanner plugin, falling back to in-process", "name", scannerName, "error", err)
		}
	}

	// Ensure any plugin scanner we attached gets closed (subprocess cleanup).
	if b.Scanner != nil {
		defer b.Scanner.Close()
	}

	result, err := b.Run(ctx)
	if err != nil {
		if errors.Is(err, agent.ErrSessionCancelled) {
			slog.Warn("backup cancelled by server",
				"job_id", job.JobID,
				"job_name", job.Name,
			)
			return
		}
		slog.Error("job backup failed",
			"job_id", job.JobID,
			"job_name", job.Name,
			"error", err,
		)
		return
	}

	slog.Info("job backup complete",
		"job_id", job.JobID,
		"job_name", job.Name,
		"snapshot", result.SnapshotID,
		"files_new", result.FilesNew,
		"files_unchanged", result.FilesUnchanged,
		"files_skipped", result.FilesSkipped,
		"bytes_added", result.BytesAdded,
		"duration", result.Duration,
	)
	if result.FilesSkipped > 0 {
		slog.Warn("some files could not be backed up (enable consistent snapshots for open files)",
			"job_id", job.JobID,
			"files_skipped", result.FilesSkipped,
		)
	}
}
