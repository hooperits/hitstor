package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/ui"
	"github.com/spf13/cobra"
)

func newDuCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "du",
		Short: "Show repository disk usage",
		Args:  cobra.NoArgs,
		RunE:  runDu,
	}
}

func runDu(cmd *cobra.Command, args []string) error {
	repo, be, err := openRepository()
	if err != nil {
		return err
	}
	defer be.Close()

	ctx := context.Background()

	// Count snapshots.
	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		return fmt.Errorf("list snapshots: %w", err)
	}

	// Count and sum packs (data files).
	packIDs, err := be.List(ctx, backend.DataFile)
	if err != nil {
		return fmt.Errorf("list packs: %w", err)
	}

	// Count indexes.
	indexIDs, err := be.List(ctx, backend.IndexFile)
	if err != nil {
		return fmt.Errorf("list indexes: %w", err)
	}

	// Sum sizes across all file types.
	var totalBytes uint64
	for _, id := range packIDs {
		fi, err := be.Stat(ctx, backend.DataFile, id)
		if err != nil {
			continue
		}
		totalBytes += uint64(fi.Size)
	}
	for _, id := range indexIDs {
		fi, err := be.Stat(ctx, backend.IndexFile, id)
		if err != nil {
			continue
		}
		totalBytes += uint64(fi.Size)
	}
	for _, id := range snapIDs {
		fi, err := be.Stat(ctx, backend.SnapshotFile, id)
		if err != nil {
			continue
		}
		totalBytes += uint64(fi.Size)
	}

	if jsonFlag {
		data := map[string]any{
			"snapshots":   len(snapIDs),
			"packs":       len(packIDs),
			"indexes":     len(indexIDs),
			"total_bytes": totalBytes,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(data)
	}

	color := ui.NewColorWriter(os.Stdout, quietFlag)
	fmt.Fprintf(os.Stdout, "Snapshots:  %s\n", color.Cyan(fmt.Sprintf("%4d", len(snapIDs))))
	fmt.Fprintf(os.Stdout, "Packs:      %s\n", color.Cyan(fmt.Sprintf("%4d", len(packIDs))))
	fmt.Fprintf(os.Stdout, "Indexes:    %s\n", color.Cyan(fmt.Sprintf("%4d", len(indexIDs))))
	fmt.Fprintf(os.Stdout, "Total size: %s\n", color.Cyan(ui.FormatBytes(totalBytes)))

	return nil
}
