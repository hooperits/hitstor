// Package workloadcat reads the workload items stored in snapshots. A
// workload backup stores each item a plugin produced as the file
// <plugin>/<item> in its snapshot, next to <item>.meta.json, the item's
// manifest with the plugin's metadata. Snapshots never change, so the items
// of each one are read once and cached.
package workloadcat

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"sync"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/compress"
	"github.com/hooperits/hitstor/internal/fs"
	"github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/repository"
)

// Item is one item a workload plugin produced, found in a snapshot.
type Item struct {
	SnapshotID string
	Manifest   plugin.ItemManifest
	// Content lists the item's data chunks.
	Content []fs.ID
}

// Object returns the object an item belongs to: its directory, e.g.
// "MSSQLSERVER/db" for "MSSQLSERVER/db/full-....bak".
func (it Item) Object() string { return path.Dir(it.Manifest.Item) }

// Catalog caches the items of each snapshot.
type Catalog struct {
	mu    sync.Mutex
	items map[string][]Item // repository|snapshot|plugin -> items
}

// Default is the catalog shared by the server and the dashboard.
var Default = &Catalog{}

// SnapshotItems returns the items pluginID stored in snapshot id of repo.
func (c *Catalog) SnapshotItems(repo *repository.Repository, id repository.ID, snap *repository.Snapshot, pluginID string) ([]Item, error) {
	key := fmt.Sprintf("%p|%s|%s", repo, id, pluginID)
	c.mu.Lock()
	cached, ok := c.items[key]
	c.mu.Unlock()
	if ok {
		return cached, nil
	}
	root, err := loadTree(repo, backend.ID(snap.Tree))
	if err != nil {
		return nil, err
	}
	var out []Item
	if dir := findDir(repo, root, pluginID); dir != nil {
		collect(repo, id.String(), dir, &out)
	}
	c.mu.Lock()
	if c.items == nil {
		c.items = make(map[string][]Item)
	}
	c.items[key] = out
	c.mu.Unlock()
	return out, nil
}

type tree struct {
	Nodes []fs.Node `json:"nodes"`
}

func loadTree(repo *repository.Repository, id backend.ID) (*tree, error) {
	blob, packID := repo.LookupBlob(repository.ID(id))
	if blob == nil {
		return nil, fmt.Errorf("tree blob %s not found in index", id)
	}
	data, err := repo.LoadBlob(*packID, *blob)
	if err != nil {
		return nil, fmt.Errorf("load tree blob %s: %w", id, err)
	}
	if data, err = compress.Decompress(data); err != nil {
		return nil, fmt.Errorf("decompress tree blob %s: %w", id, err)
	}
	var t tree
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("unmarshal tree blob %s: %w", id, err)
	}
	return &t, nil
}

// findDir descends from root along slash-separated dir, returning nil when
// any component is missing.
func findDir(repo *repository.Repository, root *tree, dir string) *tree {
	cur := root
	for _, name := range strings.Split(dir, "/") {
		var next *tree
		for _, n := range cur.Nodes {
			if n.Name == name && n.Type == fs.NodeTypeDir {
				if t, err := loadTree(repo, backend.ID(n.Subtree)); err == nil {
					next = t
				}
				break
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

// ReadContent calls fn with each decompressed chunk of a file's content.
func ReadContent(repo *repository.Repository, content []fs.ID, fn func([]byte) error) error {
	for _, id := range content {
		blob, packID := repo.LookupBlob(repository.ID(id))
		if blob == nil {
			return fmt.Errorf("blob %s not found in index", backend.ID(id))
		}
		data, err := repo.LoadBlob(*packID, *blob)
		if err != nil {
			return fmt.Errorf("load blob %s: %w", backend.ID(id), err)
		}
		if data, err = compress.Decompress(data); err != nil {
			return fmt.Errorf("decompress blob %s: %w", backend.ID(id), err)
		}
		if err := fn(data); err != nil {
			return err
		}
	}
	return nil
}

// collect walks t, a directory inside the plugin's snapshot directory, and
// appends every item that has a manifest.
func collect(repo *repository.Repository, snapHex string, t *tree, out *[]Item) {
	files := make(map[string]fs.Node)
	for _, n := range t.Nodes {
		switch n.Type {
		case fs.NodeTypeFile:
			files[n.Name] = n
		case fs.NodeTypeDir:
			if sub, err := loadTree(repo, backend.ID(n.Subtree)); err == nil {
				collect(repo, snapHex, sub, out)
			}
		}
	}
	for name, n := range files {
		itemName, ok := strings.CutSuffix(name, plugin.MetadataSuffix)
		if !ok {
			continue
		}
		item, ok := files[itemName]
		if !ok {
			continue
		}
		var raw []byte
		if err := ReadContent(repo, n.Content, func(b []byte) error { raw = append(raw, b...); return nil }); err != nil {
			slog.Warn("workload catalog: unreadable item manifest", "snapshot", snapHex, "item", name, "error", err)
			continue
		}
		var m plugin.ItemManifest
		if err := json.Unmarshal(raw, &m); err != nil {
			slog.Warn("workload catalog: invalid item manifest", "snapshot", snapHex, "item", name, "error", err)
			continue
		}
		*out = append(*out, Item{SnapshotID: snapHex, Manifest: m, Content: item.Content})
	}
}
