package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/hooperits/hitstor/internal/pruner"
	"github.com/hooperits/hitstor/internal/ui"
	"github.com/spf13/cobra"
)

var maxRepackSizeFlag string

func newPruneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove unreferenced data from the repository",
		Long: `Remove pack files, index entries, and blobs that are no longer
referenced by any snapshot. Run this after 'forget' to reclaim storage space.`,
		Args: cobra.NoArgs,
		RunE: runPrune,
	}

	cmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "only show what would be removed")
	cmd.Flags().StringVar(&maxRepackSizeFlag, "max-repack-size", "", "limit total bytes repacked (e.g., 500MiB, 2GiB)")

	return cmd
}

func runPrune(cmd *cobra.Command, args []string) error {
	repo, be, err := openRepository()
	if err != nil {
		return err
	}
	defer be.Close()

	if err := lockRepository(repo); err != nil {
		return fmt.Errorf("lock repository: %w", err)
	}
	defer repo.Unlock()

	opts := pruner.Options{
		DryRun: dryRunFlag,
	}

	if maxRepackSizeFlag != "" {
		size, err := parseSize(maxRepackSizeFlag)
		if err != nil {
			return fmt.Errorf("invalid --max-repack-size: %w", err)
		}
		opts.MaxRepackSize = size
	}

	progress := ui.NewProgress(os.Stdout, jsonFlag, quietFlag)

	p := pruner.New(repo)
	stats, err := p.Prune(opts, func(step string) {
		if !quietFlag && !jsonFlag {
			fmt.Fprintf(os.Stdout, "%s\n", step)
		}
	})
	if err != nil {
		return err
	}

	progress.PruneComplete(
		stats.PacksDeleted, stats.PacksRepacked,
		stats.BytesFreed, stats.BytesRepacked,
		stats.IndexesDeleted, stats.IndexesWritten,
	)

	return nil
}

// parseSize parses a human-readable size string into bytes.
// Supports suffixes: B, KiB, MiB, GiB, TiB, KB, MB, GB, TB.
func parseSize(s string) (uint64, error) {
	s = strings.TrimSpace(s)

	multipliers := map[string]uint64{
		"B":   1,
		"KiB": 1024,
		"MiB": 1024 * 1024,
		"GiB": 1024 * 1024 * 1024,
		"TiB": 1024 * 1024 * 1024 * 1024,
		"KB":  1000,
		"MB":  1000 * 1000,
		"GB":  1000 * 1000 * 1000,
		"TB":  1000 * 1000 * 1000 * 1000,
	}

	for suffix, mult := range multipliers {
		if strings.HasSuffix(s, suffix) {
			numStr := strings.TrimSpace(strings.TrimSuffix(s, suffix))
			n, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid number %q: %w", numStr, err)
			}
			return uint64(n * float64(mult)), nil
		}
	}

	// No suffix — treat as bytes.
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: expected number with optional suffix (MiB, GiB, etc.)", s)
	}
	return n, nil
}
