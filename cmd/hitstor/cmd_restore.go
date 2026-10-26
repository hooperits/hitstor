package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/restorer"
	"github.com/hooperits/hitstor/internal/ui"
	"github.com/spf13/cobra"
)

var includeFlags []string

func newRestoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "restore [snapshot-id] [target-path]",
		Short:             "Restore files from a snapshot",
		Args:              cobra.ExactArgs(2),
		RunE:              runRestore,
		ValidArgsFunction: completeSnapshotIDs,
	}

	cmd.Flags().StringArrayVar(&includeFlags, "include", nil, "restore only matching paths (repeatable)")

	return cmd
}

func runRestore(cmd *cobra.Command, args []string) error {
	progress := ui.NewProgress(os.Stdout, jsonFlag, quietFlag)

	repo, be, err := openRepository()
	if err != nil {
		return err
	}
	defer be.Close()

	snapshotIDStr := args[0]
	targetPath := args[1]

	// Resolve snapshot ID (support unique prefix matching).
	snapID, err := resolveSnapshotID(repo, snapshotIDStr)
	if err != nil {
		return err
	}

	progress.RestoreStart(snapID.String(), targetPath)
	startTime := time.Now()

	r := restorer.New(repo)

	r.SetParallel(resolveParallel())

	bar := ui.NewProgressBar(os.Stdout, jsonFlag || quietFlag)

	r.SetProgress(func(path string, size uint64) {
		bar.Add(size)
		if !bar.Active() {
			progress.FileRestored(path, size)
		}
	})
	r.SetTreeWalkComplete(func(files int, totalBytes uint64) {
		bar.Start(uint64(files), totalBytes)
	})

	if err := r.Restore(snapID, targetPath, includeFlags); err != nil {
		bar.Finish()
		return fmt.Errorf("restore: %w", err)
	}

	bar.Finish()

	duration := time.Since(startTime).Seconds()
	progress.RestoreComplete(r.FilesRestored, r.BytesRestored, duration)

	return nil
}

// resolveSnapshotID resolves a snapshot ID string, supporting unique prefix
// matching. If the string is a full 64-char hex ID, it's used directly.
// Otherwise, it's treated as a prefix and matched against all snapshot IDs.
func resolveSnapshotID(repo *repository.Repository, idStr string) (backend.ID, error) {
	// Support "latest" keyword — resolve to the most recent snapshot.
	if idStr == "latest" {
		snapIDs, err := repo.ListSnapshots()
		if err != nil {
			return backend.ID{}, fmt.Errorf("list snapshots: %w", err)
		}
		if len(snapIDs) == 0 {
			return backend.ID{}, fmt.Errorf("no snapshots found")
		}

		var latestID backend.ID
		var latestTime time.Time
		for _, id := range snapIDs {
			snap, err := repo.LoadSnapshot(id)
			if err != nil {
				continue
			}
			if snap.Time.After(latestTime) {
				latestTime = snap.Time
				latestID = id
			}
		}
		if latestTime.IsZero() {
			return backend.ID{}, fmt.Errorf("could not load any snapshots")
		}
		return latestID, nil
	}

	// Try full ID first.
	if len(idStr) == 64 {
		b, err := hex.DecodeString(idStr)
		if err == nil && len(b) == 32 {
			var id backend.ID
			copy(id[:], b)
			return id, nil
		}
	}

	// Prefix matching.
	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		return backend.ID{}, fmt.Errorf("list snapshots: %w", err)
	}

	var matches []backend.ID
	for _, id := range snapIDs {
		if strings.HasPrefix(id.String(), idStr) {
			matches = append(matches, id)
		}
	}

	switch len(matches) {
	case 0:
		return backend.ID{}, fmt.Errorf("snapshot %q not found", idStr)
	case 1:
		return matches[0], nil
	default:
		return backend.ID{}, fmt.Errorf("snapshot prefix %q is ambiguous (%d matches)", idStr, len(matches))
	}
}
