package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"

	"github.com/hooperits/hitstor/internal/agent"
	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/ui"
	"github.com/spf13/cobra"
)

var (
	excludeFlags []string
	tagFlags     []string
	hostFlag     string
	parallelFlag int
	useVSSFlag   bool
)

func newBackupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup [paths...]",
		Short: "Back up files to the remote server",
		Long:  "Scan the specified paths, chunk and compress files locally, then upload new data to the HitStor server.",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runAgentBackup,
	}
	cmd.Flags().StringArrayVar(&excludeFlags, "exclude", nil, "exclude glob pattern (can be repeated)")
	cmd.Flags().StringArrayVar(&tagFlags, "tag", nil, "add tag to snapshot (can be repeated)")
	cmd.Flags().StringVar(&hostFlag, "host", "", "override hostname sent to server")
	cmd.Flags().IntVar(&parallelFlag, "parallel", 0, "parallel file workers (0 = auto)")
	cmd.Flags().BoolVar(&useVSSFlag, "use-vss", false, "consistent snapshot while reading: Windows VSS, Linux LVM/btrfs when available")
	return cmd
}

func runAgentBackup(cmd *cobra.Command, args []string) error {
	if err := requireServer(); err != nil {
		return err
	}
	if err := requireAPIKey(); err != nil {
		return err
	}

	// Resolve parallel workers.
	parallel := parallelFlag
	if parallel <= 0 {
		parallel = runtime.NumCPU()
		if parallel > 8 {
			parallel = 8
		}
	}

	// Merge config excludes with CLI excludes.
	allExcludes := append(loadedConfig.AgentExcludes, excludeFlags...)

	// Resolve hostname.
	hostname := hostFlag
	if hostname == "" && loadedConfig.AgentHostname != "" {
		hostname = loadedConfig.AgentHostname
	}

	// Setup progress reporting.
	progress := ui.NewProgress(os.Stdout, jsonFlag, quietFlag)
	bar := ui.NewProgressBar(os.Stdout, jsonFlag || quietFlag)

	if !quietFlag && !jsonFlag {
		fmt.Fprintf(os.Stdout, "connecting to %s...\n", serverFlag)
	}

	client, err := agent.NewClient(agent.ClientOptions{
		ServerAddr: serverFlag,
		CACertPath: caCertFlag,
		PinPath:    agent.DefaultServerPinPath,
	})
	if err != nil {
		return err
	}
	defer client.Close()

	b := &agent.Backup{
		Client:   client,
		APIKey:   apiKeyFlag,
		Paths:    args,
		Excludes: allExcludes,
		Hostname: hostname,
		Tags:     tagFlags,
		Parallel: parallel,
		UseVSS:   useVSSFlag,
	}

	// Load scanner plugin if configured.
	if loadedConfig.FsScannerPlugin != "" {
		s, err := hitstorplugin.OpenFilesystemScanner(
			loadedConfig.FsScannerPlugin, loadedConfig.PluginDir, nil)
		if err != nil {
			return fmt.Errorf("load scanner plugin %q: %w", loadedConfig.FsScannerPlugin, err)
		}
		defer s.Close()
		b.Scanner = s
	}

	b.SetProgress(func(path string, size uint64, isNew bool) {
		bar.Add(size)
		if !bar.Active() {
			progress.FileProcessed(path, size, isNew)
		}
	})
	b.SetScanComplete(func(files, dirs int, totalBytes uint64) {
		progress.ScanComplete(files, dirs)
		bar.Start(uint64(files), totalBytes)
	})

	// Support graceful cancellation.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, shutdownSignals...)
	go func() {
		<-sigCh
		cancel()
	}()

	result, err := b.Run(ctx)
	bar.Finish()
	if err != nil {
		return err
	}

	progress.AgentBackupComplete(
		result.SnapshotID,
		result.FilesNew,
		result.FilesUnchanged,
		result.BytesAdded,
		result.BytesProcessed,
		result.Duration,
	)

	return nil
}
