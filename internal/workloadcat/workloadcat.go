// Package workloadcat reads the workload items stored in snapshots. A
// workload backup stores each item a plugin produced as the file
// <plugin>/<item> in its snapshot, next to <item>.meta.json, the item's
// manifest with the plugin's metadata. Snapshots never change, so the
// manifests of each one are read once and cached; an item's data chunks are
// looked up only when it is restored.
package workloadcat

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"
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
}

// Object returns the object an item belongs to: its directory, e.g.
// "MSSQLSERVER/db" for "MSSQLSERVER/db/full-....bak".
func (it Item) Object() string { return path.Dir(it.Manifest.Item) }

// Catalog caches the items of each snapshot.
type Catalog struct {
	mu    sync.Mutex
	items map[*repository.Repository]map[string][]Item // repo -> snapshot|plugin -> items
}

// Default is the catalog shared by the server and the dashboard.
var Default = &Catalog{}

// SnapshotItems returns the items pluginID stored in snapshot id of repo.
// A snapshot that cannot be read completely is an error and is not cached,
// so a passing read failure never hides items for good.
func (c *Catalog) SnapshotItems(repo *repository.Repository, id repository.ID, snap *repository.Snapshot, pluginID string) ([]Item, error) {
	key := id.String() + "|" + pluginID
	c.mu.Lock()
	cached, ok := c.items[repo][key]
	c.mu.Unlock()
	if ok {
		return cached, nil
	}
	root, err := loadTree(repo, backend.ID(snap.Tree))
	if err != nil {
		return nil, err
	}
	out := []Item{}
	dir, err := findDir(repo, root, pluginID)
	if err != nil {
		return nil, err
	}
	if dir != nil {
		if err := collect(repo, id.String(), dir, &out); err != nil {
			return nil, err
		}
	}
	c.mu.Lock()
	if c.items == nil {
		c.items = make(map[*repository.Repository]map[string][]Item)
	}
	if c.items[repo] == nil {
		c.items[repo] = make(map[string][]Item)
	}
	c.items[repo][key] = out
	c.mu.Unlock()
	return out, nil
}

// Retain drops the cached items of repo's snapshots that are not in ids,
// such as snapshots retention has forgotten.
func (c *Catalog) Retain(repo *repository.Repository, ids []repository.ID) {
	live := make(map[string]bool, len(ids))
	for _, id := range ids {
		live[id.String()] = true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.items[repo] {
		if snap, _, _ := strings.Cut(key, "|"); !live[snap] {
			delete(c.items[repo], key)
		}
	}
}

// RetainRepos drops the cached items of repositories not in repos, such as
// one that was disconnected or reopened.
func (c *Catalog) RetainRepos(repos []*repository.Repository) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for repo := range c.items {
		if !slices.Contains(repos, repo) {
			delete(c.items, repo)
		}
	}
}

// ItemContent returns the data chunks of item in snapshot snap.
func ItemContent(repo *repository.Repository, snap *repository.Snapshot, pluginID, item string) ([]fs.ID, error) {
	root, err := loadTree(repo, backend.ID(snap.Tree))
	if err != nil {
		return nil, err
	}
	dir, err := findDir(repo, root, path.Dir(pluginID+"/"+item))
	if err != nil {
		return nil, err
	}
	if dir != nil {
		for _, n := range dir.Nodes {
			if n.Name == path.Base(item) && n.Type == fs.NodeTypeFile {
				return n.Content, nil
			}
		}
	}
	return nil, fmt.Errorf("item %s not found in snapshot", item)
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

// findDir descends from root along slash-separated dir. It returns nil, nil
// when a component does not exist.
func findDir(repo *repository.Repository, root *tree, dir string) (*tree, error) {
	cur := root
	for _, name := range strings.Split(dir, "/") {
		var next *tree
		for _, n := range cur.Nodes {
			if n.Name == name && n.Type == fs.NodeTypeDir {
				t, err := loadTree(repo, backend.ID(n.Subtree))
				if err != nil {
					return nil, err
				}
				next = t
				break
			}
		}
		if next == nil {
			return nil, nil
		}
		cur = next
	}
	return cur, nil
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
func collect(repo *repository.Repository, snapHex string, t *tree, out *[]Item) error {
	files := make(map[string]fs.Node)
	for _, n := range t.Nodes {
		switch n.Type {
		case fs.NodeTypeFile:
			files[n.Name] = n
		case fs.NodeTypeDir:
			sub, err := loadTree(repo, backend.ID(n.Subtree))
			if err != nil {
				return err
			}
			if err := collect(repo, snapHex, sub, out); err != nil {
				return err
			}
		}
	}
	for name, n := range files {
		itemName, ok := strings.CutSuffix(name, plugin.MetadataSuffix)
		if !ok {
			continue
		}
		if _, ok := files[itemName]; !ok {
			continue
		}
		var raw []byte
		if err := ReadContent(repo, n.Content, func(b []byte) error { raw = append(raw, b...); return nil }); err != nil {
			return fmt.Errorf("snapshot %s: manifest %s: %w", snapHex[:8], name, err)
		}
		var m plugin.ItemManifest
		if err := json.Unmarshal(raw, &m); err != nil {
			// A manifest that is not JSON never will be: skip it for good.
			continue
		}
		*out = append(*out, Item{SnapshotID: snapHex, Manifest: m})
	}
	return nil
}

// ObjectTags returns snap's tags, with an "obj:<object>" tag for each
// object a workload snapshot holds when it lacks them (snapshots taken before
// servers tagged them), read from its contents, so retention keeps each
// object's chain on its own. A snapshot that cannot be read keeps its tags,
// with the error: retention then keeps it as possibly holding any object.
func (c *Catalog) ObjectTags(repo *repository.Repository, id repository.ID, snap *repository.Snapshot) ([]string, error) {
	pluginID := ""
	for _, t := range snap.Tags {
		if strings.HasPrefix(t, "obj:") || t == "objects:none" {
			return snap.Tags, nil
		}
		if v, ok := strings.CutPrefix(t, "plugin:"); ok {
			pluginID = v
		}
	}
	if pluginID == "" {
		return snap.Tags, nil
	}
	items, err := c.SnapshotItems(repo, id, snap, pluginID)
	if err != nil {
		return snap.Tags, err
	}
	tags := append([]string{}, snap.Tags...)
	seen := map[string]bool{}
	for _, it := range items {
		obj := it.Object()
		if obj == "." {
			return snap.Tags, nil // an item outside any object: it may hold anything
		}
		if !seen[obj] {
			seen[obj] = true
			tags = append(tags, "obj:"+obj)
		}
	}
	if len(items) == 0 {
		tags = append(tags, "objects:none")
	}
	return tags, nil
}
