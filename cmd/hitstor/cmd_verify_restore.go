package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/hooperits/hitstor/internal/ui"
	"github.com/hooperits/hitstor/internal/verifyrestore"
	"github.com/spf13/cobra"
)

var (
	verifyMaxFiles int
	verifyMaxBytes uint64
	verifyKeepTemp bool
)

func newVerifyRestoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "verify-restore [snapshot-id]",
		Short:             "Test-restore a snapshot and re-hash files (restore verification)",
		Long: `Restore a snapshot (or a sample of its files) to a temporary directory,
re-chunk each restored file with the repository chunker, and compare content
blob IDs to the snapshot tree.

This is the primary trust signal that a filesystem backup is restorable.
Use "latest" as the snapshot id to verify the most recent snapshot.`,
		Args:              cobra.ExactArgs(1),
		RunE:              runVerifyRestore,
		ValidArgsFunction: completeSnapshotIDs,
	}
	cmd.Flags().IntVar(&verifyMaxFiles, "max-files", 0, "limit number of files to verify (0 = all)")
	cmd.Flags().Uint64Var(&verifyMaxBytes, "max-bytes", 0, "limit total bytes verified (0 = unlimited)")
	cmd.Flags().BoolVar(&verifyKeepTemp, "keep-temp", false, "keep the temporary restore directory")
	return cmd
}

func runVerifyRestore(cmd *cobra.Command, args []string) error {
	repo, be, err := openRepository()
	if err != nil {
		return err
	}
	defer be.Close()

	snapID, err := resolveSnapshotID(repo, args[0])
	if err != nil {
		return err
	}

	color := ui.NewColorWriter(os.Stdout, jsonFlag || quietFlag)
	if !quietFlag && !jsonFlag {
		fmt.Fprintf(os.Stdout, "%s\n", color.Cyan(fmt.Sprintf("verifying restore of snapshot %s…", snapID.String()[:12])))
	}

	res := verifyrestore.Verify(repo, snapID, verifyrestore.Options{
		MaxFiles: verifyMaxFiles,
		MaxBytes: verifyMaxBytes,
		Parallel: resolveParallel(),
		KeepTemp: verifyKeepTemp,
	})

	if jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		type out struct {
			OK           bool                      `json:"ok"`
			SnapshotID   string                    `json:"snapshot_id"`
			FilesChecked int                       `json:"files_checked"`
			FilesSampled int                       `json:"files_sampled"`
			BytesChecked uint64                    `json:"bytes_checked"`
			SymlinksOK   int                       `json:"symlinks_ok"`
			Sampled      bool                      `json:"sampled"`
			Duration     string                    `json:"duration"`
			Mismatches   []verifyrestore.Mismatch  `json:"mismatches,omitempty"`
			Error        string                    `json:"error,omitempty"`
			RestoreTarget string                   `json:"restore_target,omitempty"`
		}
		return enc.Encode(out{
			OK:            res.OK,
			SnapshotID:    res.SnapshotID,
			FilesChecked:  res.FilesChecked,
			FilesSampled:  res.FilesSampled,
			BytesChecked:  res.BytesChecked,
			SymlinksOK:    res.SymlinksOK,
			Sampled:       res.Sampled,
			Duration:      res.Duration.Round(time.Millisecond).String(),
			Mismatches:    res.Mismatches,
			Error:         res.Error,
			RestoreTarget: res.RestoreTarget,
		})
	}

	if res.Error != "" {
		return fmt.Errorf("verify-restore: %s", res.Error)
	}
	if !res.OK {
		for _, m := range res.Mismatches {
			fmt.Fprintf(os.Stderr, "%s\n", color.Red(fmt.Sprintf("mismatch %s: %s", m.Path, m.Reason)))
		}
		return fmt.Errorf("verify-restore failed: %d mismatch(es)", len(res.Mismatches))
	}

	if !quietFlag {
		sampleNote := ""
		if res.Sampled {
			sampleNote = fmt.Sprintf(" (sampled %d files)", res.FilesSampled)
		}
		fmt.Fprintf(os.Stdout, "%s\n", color.Green(fmt.Sprintf(
			"restore verification ok: %d files, %d bytes, %d symlinks in %s%s",
			res.FilesChecked, res.BytesChecked, res.SymlinksOK,
			res.Duration.Round(time.Millisecond), sampleNote,
		)))
		if verifyKeepTemp && res.RestoreTarget != "" {
			fmt.Fprintf(os.Stdout, "temp restore kept at %s\n", res.RestoreTarget)
		}
	}
	return nil
}
