package main

import (
	"fmt"
	"os"

	"github.com/hooperits/hitstor/internal/pruner"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/retention"
	"github.com/hooperits/hitstor/internal/ui"
	"github.com/spf13/cobra"
)

var (
	dryRunFlag   bool
	pruneFlag    bool
	keepLast     int
	keepHourly   int
	keepDaily    int
	keepWeekly   int
	keepMonthly  int
	keepYearly   int
	keepTags     []string
	filterHost   string
	filterPath   string
)

func newForgetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "forget [snapshot-id...]",
		Aliases:           []string{"rm"},
		Short:             "Remove snapshots by ID or retention policy",
		ValidArgsFunction: completeSnapshotIDs,
		Long: `Remove snapshots explicitly by ID or automatically via retention policies.

When snapshot IDs are provided, those specific snapshots are removed.
When --keep-* flags are provided, a retention policy is applied to
automatically select which snapshots to forget.`,
		Args: cobra.ArbitraryArgs,
		RunE: runForget,
	}

	cmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "only show what would be removed")
	cmd.Flags().BoolVar(&pruneFlag, "prune", false, "run prune after forgetting to reclaim space")
	cmd.Flags().IntVar(&keepLast, "keep-last", 0, "keep the N most recent snapshots")
	cmd.Flags().IntVar(&keepHourly, "keep-hourly", 0, "keep N hourly snapshots")
	cmd.Flags().IntVar(&keepDaily, "keep-daily", 0, "keep N daily snapshots")
	cmd.Flags().IntVar(&keepWeekly, "keep-weekly", 0, "keep N weekly snapshots")
	cmd.Flags().IntVar(&keepMonthly, "keep-monthly", 0, "keep N monthly snapshots")
	cmd.Flags().IntVar(&keepYearly, "keep-yearly", 0, "keep N yearly snapshots")
	cmd.Flags().StringSliceVar(&keepTags, "keep-tag", nil, "keep snapshots with this tag (repeatable)")
	cmd.Flags().StringVar(&filterHost, "host", "", "only apply policy to snapshots from this host")
	cmd.Flags().StringVar(&filterPath, "path", "", "only apply policy to snapshots with this path")

	return cmd
}

func runForget(cmd *cobra.Command, args []string) error {
	policy := retention.Policy{
		KeepLast:    keepLast,
		KeepHourly:  keepHourly,
		KeepDaily:   keepDaily,
		KeepWeekly:  keepWeekly,
		KeepMonthly: keepMonthly,
		KeepYearly:  keepYearly,
		KeepTags:    keepTags,
	}

	hasPolicyFlags := !policy.IsEmpty()
	hasExplicitIDs := len(args) > 0

	if !hasPolicyFlags && !hasExplicitIDs {
		return fmt.Errorf("either snapshot IDs or --keep-* retention flags are required")
	}
	if hasPolicyFlags && hasExplicitIDs {
		return fmt.Errorf("cannot combine explicit snapshot IDs with --keep-* retention flags")
	}

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

	progress := ui.NewProgress(os.Stdout, jsonFlag, quietFlag)

	color := ui.NewColorWriter(os.Stdout, jsonFlag || quietFlag)

	if hasPolicyFlags {
		return runForgetPolicy(repo, policy, progress, color)
	}
	return runForgetExplicit(repo, args, progress, color)
}

func runForgetPolicy(repo *repository.Repository, policy retention.Policy, progress *ui.Progress, color *ui.ColorWriter) error {
	// Load all snapshots.
	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		return fmt.Errorf("list snapshots: %w", err)
	}

	var snapInfos []retention.SnapshotInfo
	for _, id := range snapIDs {
		snap, err := repo.LoadSnapshot(id)
		if err != nil {
			return fmt.Errorf("load snapshot %s: %w", id, err)
		}
		snapInfos = append(snapInfos, retention.SnapshotInfo{
			ID:       id,
			Time:     snap.Time,
			Hostname: snap.Hostname,
			Paths:    snap.Paths,
			Tags:     snap.Tags,
		})
	}

	result, err := retention.Apply(snapInfos, policy, filterHost, filterPath)
	if err != nil {
		return err
	}

	if dryRunFlag {
		for _, s := range result.Keep {
			fmt.Fprintf(os.Stdout, "%s %s %s\n", color.Green("keep  "), color.Cyan(s.ID.String()[:8]), s.Time.Format("2006-01-02 15:04:05"))
		}
		for _, s := range result.Forget {
			fmt.Fprintf(os.Stdout, "%s %s %s\n", color.Yellow("forget"), color.Cyan(s.ID.String()[:8]), s.Time.Format("2006-01-02 15:04:05"))
		}
		progress.ForgetComplete(len(result.Keep), len(result.Forget))
		return nil
	}

	forgotten := 0
	for _, s := range result.Forget {
		if err := repo.RemoveSnapshot(s.ID); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", color.Yellow(fmt.Sprintf("warning: failed to remove snapshot %s: %v", s.ID, err)))
			continue
		}
		if !quietFlag {
			fmt.Fprintf(os.Stdout, "removed snapshot %s\n", color.Cyan(s.ID.String()[:8]))
		}
		forgotten++
	}

	progress.ForgetComplete(len(result.Keep), forgotten)

	if pruneFlag {
		return runPruneFromForget(repo, progress)
	}

	if forgotten > 0 && !pruneFlag {
		fmt.Fprintln(os.Stdout, "note: run 'hitstor prune' to reclaim storage space")
	}

	return nil
}

func runForgetExplicit(repo *repository.Repository, args []string, progress *ui.Progress, color *ui.ColorWriter) error {
	var resolved []struct {
		input string
		id    repository.ID
	}

	for _, idStr := range args {
		snapID, err := resolveSnapshotID(repo, idStr)
		if err != nil {
			return fmt.Errorf("snapshot %q: %w", idStr, err)
		}
		resolved = append(resolved, struct {
			input string
			id    repository.ID
		}{input: idStr, id: repository.ID(snapID)})
	}

	if dryRunFlag {
		for _, r := range resolved {
			fmt.Fprintf(os.Stdout, "would remove snapshot %s\n", color.Cyan(r.id.String()[:8]))
		}
		return nil
	}

	removed := 0
	for _, r := range resolved {
		if err := repo.RemoveSnapshot(r.id); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", color.Yellow(fmt.Sprintf("warning: failed to remove snapshot %s: %v", r.id, err)))
			continue
		}
		if !quietFlag {
			fmt.Fprintf(os.Stdout, "removed snapshot %s\n", color.Cyan(r.id.String()[:8]))
		}
		removed++
	}

	if !quietFlag {
		fmt.Fprintf(os.Stdout, "%d snapshot(s) removed\n", removed)
	}

	if pruneFlag {
		return runPruneFromForget(repo, progress)
	}

	if removed > 0 && !pruneFlag {
		fmt.Fprintln(os.Stdout, "note: run 'hitstor prune' to reclaim storage space")
	}

	return nil
}

// runPruneFromForget runs the pruner as part of a forget --prune operation.
func runPruneFromForget(repo *repository.Repository, progress *ui.Progress) error {
	p := pruner.New(repo)
	stats, err := p.Prune(pruner.Options{DryRun: dryRunFlag}, func(step string) {
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
