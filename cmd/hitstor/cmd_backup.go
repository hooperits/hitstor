package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/hooperits/hitstor/internal/archiver"
	"github.com/hooperits/hitstor/internal/cache"
	"github.com/hooperits/hitstor/internal/hook"
	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/scanner"
	"github.com/hooperits/hitstor/internal/ui"
	"github.com/spf13/cobra"
)

var (
	excludeFlags    []string
	tagFlags        []string
	hostFlag        string
	backupDryRun    bool
	backupBareMetal bool
)

func newBackupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup [paths...]",
		Short: "Back up files to the repository",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runBackup,
	}

	cmd.Flags().StringArrayVar(&excludeFlags, "exclude", nil, "exclude glob pattern (repeatable)")
	cmd.Flags().StringArrayVar(&tagFlags, "tag", nil, "add tag to snapshot (repeatable)")
	cmd.Flags().StringVar(&hostFlag, "host", "", "override hostname in snapshot")
	cmd.Flags().BoolVar(&backupDryRun, "dry-run", false, "scan and report what would be backed up without writing data")
	cmd.Flags().BoolVar(&backupBareMetal, "bare-metal", false,
		"capture what a bare-metal restore needs: extended attributes (SELinux, capabilities, ACLs) and the host's disk and boot layout")

	return cmd
}

func runBackup(cmd *cobra.Command, args []string) error {
	progress := ui.NewProgress(os.Stdout, jsonFlag, quietFlag)

	repo, be, err := openRepository()
	if err != nil {
		return err
	}
	defer be.Close()

	// Acquire exclusive lock.
	if err := lockRepository(repo); err != nil {
		return fmt.Errorf("lock repository: %w", err)
	}
	defer repo.Unlock()

	progress.StartBackup()

	a := archiver.New(repo)

	a.SetParallel(resolveParallel())
	a.SetDryRun(backupDryRun)
	a.SetCaptureSystemProfile(backupBareMetal)

	bar := ui.NewProgressBar(os.Stdout, jsonFlag || quietFlag)

	a.SetProgress(func(path string, size uint64, isNew bool) {
		bar.Add(size)
		if !bar.Active() {
			progress.FileProcessed(path, size, isNew)
		}
	})
	a.SetScanComplete(func(files, dirs int, totalBytes uint64) {
		progress.ScanComplete(files, dirs)
		bar.Start(uint64(files), totalBytes)
	})

	// Open files cache for incremental backups.
	cacheDir, err := os.UserCacheDir()
	if err == nil {
		cachePath := filepath.Join(cacheDir, "hitstor", repo.Cfg().ID, "cache.db")
		fc, err := cache.Open(cachePath)
		if err == nil {
			a.SetCache(fc)
			defer fc.Close()
		}
	}

	// Load scanner plugin if configured, or default to the builtin (shipped) plugin when available.
	// This ensures the modular architecture is used by default for filesystem scans.
	scannerName := loadedConfig.FsScannerPlugin
	if scannerName == "" || scannerName == "builtin" || scannerName == "fs" {
		if s, err := hitstorplugin.OpenFilesystemScanner("builtin", loadedConfig.PluginDir, nil); err == nil {
			defer s.Close()
			a.SetScanner(s)
		} else if scannerName != "" {
			return fmt.Errorf("load builtin scanner plugin: %w", err)
		}
		// else: empty config and no builtin plugin found -> agent uses in-process default walker
	} else if scannerName != "" {
		s, err := hitstorplugin.OpenFilesystemScanner(scannerName, loadedConfig.PluginDir, nil)
		if err != nil {
			return fmt.Errorf("load scanner plugin %q: %w", scannerName, err)
		}
		defer s.Close()
		a.SetScanner(s)
	}

	// Bare-metal capture needs extended attributes, which the scanner plugin
	// protocol does not carry yet. Fall back to the in-process walker so the
	// metadata is not silently dropped.
	if backupBareMetal {
		if scannerName != "" && scannerName != "builtin" && scannerName != "fs" {
			slog.Warn("--bare-metal overrides the configured scanner plugin",
				"plugin", scannerName,
				"reason", "the scanner plugin protocol does not carry extended attributes")
		}
		a.SetScanner(scanner.NewDefaultWithXattrs())
	}

	// Merge config excludes with CLI excludes.
	allExcludes := append(configExcludes, excludeFlags...)

	// Load hook plugins.
	var hooks []hook.Hook
	for _, name := range loadedConfig.HookPlugins {
		h, err := hitstorplugin.OpenHook(name, loadedConfig.PluginDir, nil)
		if err != nil {
			return fmt.Errorf("load hook plugin %q: %w", name, err)
		}
		defer h.Close()
		hooks = append(hooks, h)
	}

	// Run pre-backup hooks (blocking — abort on error).
	if len(hooks) > 0 {
		ctx := context.Background()
		evt := hook.HookEvent{
			Agent:    hostFlag,
			Hostname: hostFlag,
			Paths:    args,
			Excludes: allExcludes,
			Tags:     tagFlags,
		}
		for _, h := range hooks {
			if err := h.PreBackup(ctx, evt); err != nil {
				return fmt.Errorf("pre-backup hook: %w", err)
			}
		}
	}

	snapID, snap, err := a.Backup(args, allExcludes, hostFlag, tagFlags)
	if err != nil {
		bar.Finish()
		return fmt.Errorf("backup: %w", err)
	}

	bar.Finish()

	if backupDryRun {
		return printDryRunSummary(snap)
	}

	// Run post-backup hooks (non-blocking — log errors).
	if len(hooks) > 0 {
		ctx := context.Background()
		evt := hook.HookEvent{
			Agent:      hostFlag,
			Hostname:   hostFlag,
			Paths:      args,
			Excludes:   allExcludes,
			Tags:       tagFlags,
			SnapshotID: snapID.String(),
			Details: map[string]string{
				"files_new":       fmt.Sprintf("%d", snap.Summary.FilesNew),
				"files_unchanged": fmt.Sprintf("%d", snap.Summary.FilesUnchanged),
				"bytes_added":     fmt.Sprintf("%d", snap.Summary.BytesAdded),
			},
		}
		for _, h := range hooks {
			if err := h.PostBackup(ctx, evt); err != nil {
				slog.Warn("post-backup hook error", "error", err)
			}
		}
	}

	progress.BackupComplete(
		snapID.String(),
		snap.Summary.FilesNew,
		snap.Summary.FilesUnchanged,
		snap.Summary.DirsNew,
		snap.Summary.BytesAdded,
		snap.Summary.BytesProcessed,
		snap.Summary.TotalDuration,
	)

	return nil
}

func printDryRunSummary(snap *repository.Snapshot) error {
	if jsonFlag {
		data := map[string]any{
			"dry_run":     true,
			"total_files": snap.Summary.TotalFiles,
			"total_dirs":  snap.Summary.DirsNew,
			"total_bytes": snap.Summary.TotalSize,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(data)
	}

	color := ui.NewColorWriter(os.Stdout, quietFlag)
	fmt.Fprintln(os.Stdout, color.Yellow("Dry run — no data written"))
	fmt.Fprintf(os.Stdout, "  Files:       %s total (%s)\n",
		ui.FormatNumber(snap.Summary.TotalFiles), ui.FormatBytes(snap.Summary.TotalSize))
	fmt.Fprintf(os.Stdout, "  Directories: %s\n", ui.FormatNumber(snap.Summary.DirsNew))
	return nil
}
