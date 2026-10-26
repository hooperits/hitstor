package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/compress"
	"github.com/hooperits/hitstor/internal/fs"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/ui"
	"github.com/spf13/cobra"
)

func newDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "diff <snapshot-id-1> <snapshot-id-2>",
		Short:             "Compare two snapshots and list differences",
		Args:              cobra.ExactArgs(2),
		RunE:              runDiff,
		ValidArgsFunction: completeSnapshotIDs,
	}
}

// diffTree is a local copy of the tree structure (same as restorer/archiver).
type diffTree struct {
	Nodes []fs.Node `json:"nodes"`
}

// diffEntry represents a single difference between two snapshots.
type diffEntry struct {
	Path   string `json:"path"`
	Change string `json:"change"` // "added", "removed", "modified"
}

func runDiff(cmd *cobra.Command, args []string) error {
	repo, be, err := openRepository()
	if err != nil {
		return err
	}
	defer be.Close()

	snap1ID, err := resolveSnapshotID(repo, args[0])
	if err != nil {
		return err
	}
	snap2ID, err := resolveSnapshotID(repo, args[1])
	if err != nil {
		return err
	}

	// Load both snapshots.
	snap1, err := repo.LoadSnapshot(snap1ID)
	if err != nil {
		return fmt.Errorf("load snapshot %s: %w", snap1ID.String()[:8], err)
	}
	snap2, err := repo.LoadSnapshot(snap2ID)
	if err != nil {
		return fmt.Errorf("load snapshot %s: %w", snap2ID.String()[:8], err)
	}

	// Build flat path→node maps for each snapshot.
	map1, err := buildPathMap(repo, snap1.Tree)
	if err != nil {
		return fmt.Errorf("walk snapshot %s tree: %w", snap1ID.String()[:8], err)
	}
	map2, err := buildPathMap(repo, snap2.Tree)
	if err != nil {
		return fmt.Errorf("walk snapshot %s tree: %w", snap2ID.String()[:8], err)
	}

	// Compare maps.
	var diffs []diffEntry

	// Files in snap2 but not snap1 → added.
	// Files in both but content differs → modified.
	for path, node2 := range map2 {
		node1, exists := map1[path]
		if !exists {
			diffs = append(diffs, diffEntry{Path: path, Change: "added"})
			continue
		}
		if !contentEqual(node1.Content, node2.Content) {
			diffs = append(diffs, diffEntry{Path: path, Change: "modified"})
		}
	}

	// Files in snap1 but not snap2 → removed.
	for path := range map1 {
		if _, exists := map2[path]; !exists {
			diffs = append(diffs, diffEntry{Path: path, Change: "removed"})
		}
	}

	// Sort by path.
	sort.Slice(diffs, func(i, j int) bool {
		return diffs[i].Path < diffs[j].Path
	})

	sid1 := snap1ID.String()[:8]
	sid2 := snap2ID.String()[:8]

	if jsonFlag {
		added := []string{}
		removed := []string{}
		modified := []string{}
		for _, d := range diffs {
			switch d.Change {
			case "added":
				added = append(added, d.Path)
			case "removed":
				removed = append(removed, d.Path)
			case "modified":
				modified = append(modified, d.Path)
			}
		}
		data := map[string]any{
			"snapshot1": sid1,
			"snapshot2": sid2,
			"added":     added,
			"removed":   removed,
			"modified":  modified,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(data)
	}

	color := ui.NewColorWriter(os.Stdout, quietFlag)
	fmt.Fprintf(os.Stdout, "comparing snapshots %s ↔ %s\n\n", color.Cyan(sid1), color.Cyan(sid2))

	if len(diffs) == 0 {
		fmt.Fprintln(os.Stdout, color.Green("no differences"))
		return nil
	}

	added, removed, modified := 0, 0, 0
	for _, d := range diffs {
		switch d.Change {
		case "added":
			fmt.Fprintf(os.Stdout, "%s  %s\n", color.Green("+"), d.Path)
			added++
		case "removed":
			fmt.Fprintf(os.Stdout, "%s  %s\n", color.Red("-"), d.Path)
			removed++
		case "modified":
			fmt.Fprintf(os.Stdout, "%s  %s\n", color.Yellow("M"), d.Path)
			modified++
		}
	}

	fmt.Fprintf(os.Stdout, "\n%d differences (%d added, %d removed, %d modified)\n",
		len(diffs), added, removed, modified)

	return nil
}

// buildPathMap walks the snapshot tree recursively and builds a flat map of
// relative path → fs.Node for all file nodes.
func buildPathMap(repo *repository.Repository, treeID backend.ID) (map[string]fs.Node, error) {
	result := make(map[string]fs.Node)

	var walk func(treeID backend.ID, prefix string) error
	walk = func(treeID backend.ID, prefix string) error {
		t, err := loadDiffTree(repo, treeID)
		if err != nil {
			return err
		}

		for _, node := range t.Nodes {
			relPath := node.Name
			if prefix != "" {
				relPath = prefix + "/" + node.Name
			}

			switch node.Type {
			case fs.NodeTypeFile, fs.NodeTypeSymlink:
				result[relPath] = node
			case fs.NodeTypeDir:
				subtreeID := backend.ID(node.Subtree)
				if err := walk(subtreeID, relPath); err != nil {
					return err
				}
			}
		}
		return nil
	}

	return result, walk(treeID, "")
}

// loadDiffTree loads a tree blob from the repository.
func loadDiffTree(repo *repository.Repository, treeID backend.ID) (*diffTree, error) {
	blob, packID := repo.LookupBlob(treeID)
	if blob == nil {
		return nil, fmt.Errorf("tree blob %s not found in index", treeID)
	}

	compressedData, err := repo.LoadBlob(*packID, *blob)
	if err != nil {
		return nil, fmt.Errorf("load tree blob %s: %w", treeID, err)
	}

	jsonData, err := compress.Decompress(compressedData)
	if err != nil {
		return nil, fmt.Errorf("decompress tree blob %s: %w", treeID, err)
	}

	var t diffTree
	if err := json.Unmarshal(jsonData, &t); err != nil {
		return nil, fmt.Errorf("unmarshal tree blob %s: %w", treeID, err)
	}

	return &t, nil
}

// contentEqual compares two Content ID slices for equality.
func contentEqual(a, b []fs.ID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
