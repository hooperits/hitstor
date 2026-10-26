package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/ui"
	"github.com/spf13/cobra"
)

var (
	snapshotHostFilter string
	snapshotPathFilter string
)

func newSnapshotsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "snapshots",
		Aliases: []string{"ls"},
		Short:   "List all snapshots in the repository",
		RunE:    runSnapshots,
	}

	cmd.Flags().StringVar(&snapshotHostFilter, "host", "", "filter by hostname")
	cmd.Flags().StringVar(&snapshotPathFilter, "path", "", "filter by backed-up path")

	return cmd
}

func runSnapshots(cmd *cobra.Command, args []string) error {
	repo, be, err := openRepository()
	if err != nil {
		return err
	}
	defer be.Close()

	// List and load all snapshots.
	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		return fmt.Errorf("list snapshots: %w", err)
	}

	var snapshots []snapshotEntry

	for _, id := range snapIDs {
		snap, err := repo.LoadSnapshot(id)
		if err != nil {
			color := ui.NewColorWriter(os.Stderr, jsonFlag || quietFlag)
		fmt.Fprintf(os.Stderr, "%s\n", color.Yellow(fmt.Sprintf("warning: failed to load snapshot %s: %v", id, err)))
			continue
		}

		// Apply filters.
		if snapshotHostFilter != "" && snap.Hostname != snapshotHostFilter {
			continue
		}
		if snapshotPathFilter != "" {
			found := false
			for _, p := range snap.Paths {
				if p == snapshotPathFilter {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		snapshots = append(snapshots, snapshotEntry{
			ID:   id.String(),
			Snap: snap,
		})
	}

	// Sort by time (newest first).
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Snap.Time.After(snapshots[j].Snap.Time)
	})

	if jsonFlag {
		return printSnapshotsJSON(snapshots)
	}

	return printSnapshotsTable(snapshots)
}

func printSnapshotsJSON(snapshots []snapshotEntry) error {
	type jsonSnap struct {
		ID         string   `json:"id"`
		Time       string   `json:"time"`
		Hostname   string   `json:"hostname"`
		Tags       []string `json:"tags,omitempty"`
		Paths      []string `json:"paths"`
		TotalFiles uint64   `json:"total_files"`
		TotalSize  uint64   `json:"total_size"`
	}

	items := make([]jsonSnap, len(snapshots))
	for i, s := range snapshots {
		items[i] = jsonSnap{
			ID:         s.ID[:8],
			Time:       s.Snap.Time.Format("2006-01-02T15:04:05Z07:00"),
			Hostname:   s.Snap.Hostname,
			Tags:       s.Snap.Tags,
			Paths:      s.Snap.Paths,
			TotalFiles: s.Snap.Summary.TotalFiles,
			TotalSize:  s.Snap.Summary.TotalSize,
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(items)
}

func printSnapshotsTable(snapshots []snapshotEntry) error {
	if len(snapshots) == 0 {
		fmt.Println("no snapshots found")
		return nil
	}

	color := ui.NewColorWriter(os.Stdout, jsonFlag || quietFlag)

	tbl := ui.NewTable(
		ui.Column{Header: "ID", Align: ui.AlignLeft, MinWidth: 8},
		ui.Column{Header: "Time", Align: ui.AlignLeft, MinWidth: 19},
		ui.Column{Header: "Host", Align: ui.AlignLeft},
		ui.Column{Header: "Tags", Align: ui.AlignLeft},
		ui.Column{Header: "Files", Align: ui.AlignRight},
		ui.Column{Header: "Size", Align: ui.AlignRight},
	)

	for _, s := range snapshots {
		tags := strings.Join(s.Snap.Tags, ", ")
		tbl.AddRow(
			color.Cyan(s.ID[:8]),
			s.Snap.Time.Format("2006-01-02 15:04:05"),
			s.Snap.Hostname,
			tags,
			ui.FormatNumber(s.Snap.Summary.TotalFiles),
			ui.FormatBytes(s.Snap.Summary.TotalSize),
		)
	}

	tbl.Render(os.Stdout, color)
	return nil
}

type snapshotEntry = struct {
	ID   string
	Snap *repository.Snapshot
}
