// Package pruner implements mark-and-sweep garbage collection for HitStor
// repositories. It removes unreferenced packs and repacks partially-referenced
// packs after snapshots have been forgotten.
package pruner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/compress"
	"github.com/hooperits/hitstor/internal/fs"
	"github.com/hooperits/hitstor/internal/repository"
)

// tree is a local copy of the tree structure used by the archiver and restorer.
type tree struct {
	Nodes []fs.Node `json:"nodes"`
}

// Stats contains statistics about a prune operation.
type Stats struct {
	SnapshotsTotal    int
	BlobsReferenced   int
	BlobsUnreferenced int
	PacksKept         int
	PacksDeleted      int
	PacksRepacked     int
	BytesFreed        uint64
	BytesRepacked     uint64
	IndexesDeleted    int
	IndexesWritten    int
}

// Options configures the prune operation.
type Options struct {
	DryRun        bool
	MaxRepackSize uint64 // 0 = unlimited
}

// Pruner performs garbage collection on a repository.
type Pruner struct {
	repo *repository.Repository
}

// New creates a new Pruner for the given repository.
func New(repo *repository.Repository) *Pruner {
	return &Pruner{repo: repo}
}

// Prune performs mark-and-sweep garbage collection.
func (p *Pruner) Prune(opts Options, progress func(string)) (Stats, error) {
	var stats Stats

	// Phase 1: Mark — collect all referenced blob IDs.
	progress("collecting referenced blobs...")
	slog.Debug("pruner: collecting referenced blobs from all snapshots")

	referenced, snapCount, err := p.collectReferencedBlobs()
	if err != nil {
		return stats, fmt.Errorf("pruner: collect referenced blobs: %w", err)
	}
	stats.SnapshotsTotal = snapCount
	stats.BlobsReferenced = len(referenced)

	// Phase 2: Sweep — classify packs.
	progress("classifying packs...")
	slog.Debug("pruner: classifying packs", "referenced_blobs", len(referenced))

	fullUnref, partialRef, fullRef := p.classifyPacks(referenced)
	stats.PacksKept = len(fullRef)
	stats.PacksDeleted = len(fullUnref)
	stats.PacksRepacked = len(partialRef)

	// Count unreferenced blobs and bytes to free.
	for _, ip := range fullUnref {
		stats.BlobsUnreferenced += len(ip.Blobs)
		for _, b := range ip.Blobs {
			stats.BytesFreed += uint64(b.Length)
		}
	}
	for _, ip := range partialRef {
		for _, b := range ip.Blobs {
			if !referenced[b.ID] {
				stats.BlobsUnreferenced++
				stats.BytesFreed += uint64(b.Length)
			}
		}
	}

	if stats.PacksDeleted == 0 && stats.PacksRepacked == 0 {
		progress("nothing to prune")
		slog.Info("prune complete", "action", "nothing to prune")
		return stats, nil
	}

	if opts.DryRun {
		progress("dry run — no changes made")
		slog.Info("prune complete (dry run)",
			"packs_deleted", stats.PacksDeleted,
			"packs_repacked", stats.PacksRepacked,
			"bytes_freed", stats.BytesFreed,
		)
		return stats, nil
	}

	// Phase 3 & 4: Repack partially-referenced packs, then delete fully-unreferenced.
	progress("repacking partially-referenced packs...")

	newPackEntries, err := p.repackPacks(partialRef, referenced, opts.MaxRepackSize)
	if err != nil {
		return stats, fmt.Errorf("pruner: repack: %w", err)
	}
	stats.BytesRepacked = 0
	for _, ip := range partialRef {
		for _, b := range ip.Blobs {
			stats.BytesRepacked += uint64(b.Length)
		}
	}

	// Phase 5: Rewrite indexes (list old IDs first, write new, delete old).
	progress("rewriting indexes...")

	var newIndex repository.Index
	for _, ip := range fullRef {
		newIndex.Packs = append(newIndex.Packs, ip)
	}
	newIndex.Packs = append(newIndex.Packs, newPackEntries...)

	oldIndexIDs, err := p.repo.ListIndexes()
	if err != nil {
		return stats, fmt.Errorf("pruner: list indexes: %w", err)
	}

	p.repo.ReplaceIndex(&newIndex)
	newIndexID, err := p.repo.SaveIndex(&newIndex)
	if err != nil {
		return stats, fmt.Errorf("pruner: save new index: %w", err)
	}
	stats.IndexesWritten = 1
	p.repo.ReplaceIndex(&newIndex)

	// Delete old packs.
	progress("deleting unreferenced packs...")
	ctx := context.Background()

	for _, ip := range fullUnref {
		if err := p.repo.Be().Remove(ctx, backend.DataFile, ip.PackID); err != nil {
			return stats, fmt.Errorf("pruner: remove pack %s: %w", ip.PackID, err)
		}
	}
	for _, ip := range partialRef {
		if err := p.repo.Be().Remove(ctx, backend.DataFile, ip.PackID); err != nil {
			return stats, fmt.Errorf("pruner: remove old pack %s: %w", ip.PackID, err)
		}
	}

	for _, indexID := range oldIndexIDs {
		if indexID == newIndexID {
			continue
		}
		if err := p.repo.RemoveIndex(indexID); err != nil {
			slog.Warn("pruner: failed to remove old index", "index", indexID, "error", err)
		}
		stats.IndexesDeleted++
	}
	if err := repository.SetLatestIndexID(p.repo.Be(), newIndexID); err != nil {
		return stats, fmt.Errorf("pruner: set index latest: %w", err)
	}

	progress("prune complete")
	slog.Info("prune complete",
		"packs_deleted", stats.PacksDeleted,
		"packs_repacked", stats.PacksRepacked,
		"bytes_freed", stats.BytesFreed,
	)

	return stats, nil
}

// collectReferencedBlobs walks all snapshots and their trees to find all
// blob IDs that are still referenced.
func (p *Pruner) collectReferencedBlobs() (map[backend.ID]bool, int, error) {
	referenced := make(map[backend.ID]bool)

	snapIDs, err := p.repo.ListSnapshots()
	if err != nil {
		return nil, 0, fmt.Errorf("list snapshots: %w", err)
	}

	for _, snapID := range snapIDs {
		snap, err := p.repo.LoadSnapshot(snapID)
		if err != nil {
			return nil, 0, fmt.Errorf("load snapshot %s: %w", snapID, err)
		}

		if err := p.walkTree(snap.Tree, referenced); err != nil {
			return nil, 0, fmt.Errorf("walk snapshot %s tree: %w", snapID, err)
		}
	}

	return referenced, len(snapIDs), nil
}

// walkTree recursively walks a tree blob, adding all referenced blob IDs
// (both tree and data blobs) to the referenced set.
func (p *Pruner) walkTree(treeID backend.ID, referenced map[backend.ID]bool) error {
	if referenced[treeID] {
		return nil // already visited
	}
	referenced[treeID] = true

	t, err := p.loadTree(treeID)
	if err != nil {
		return err
	}

	for _, node := range t.Nodes {
		switch node.Type {
		case fs.NodeTypeFile:
			for _, contentID := range node.Content {
				referenced[backend.ID(contentID)] = true
			}
		case fs.NodeTypeDir:
			if !node.Subtree.IsZero() {
				if err := p.walkTree(backend.ID(node.Subtree), referenced); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// loadTree loads and decodes a tree blob from the repository.
func (p *Pruner) loadTree(treeID backend.ID) (*tree, error) {
	blob, packID := p.repo.LookupBlob(treeID)
	if blob == nil {
		return nil, fmt.Errorf("tree blob %s not found in index", treeID)
	}

	compressedData, err := p.repo.LoadBlob(*packID, *blob)
	if err != nil {
		return nil, fmt.Errorf("load tree blob %s: %w", treeID, err)
	}

	jsonData, err := compress.Decompress(compressedData)
	if err != nil {
		return nil, fmt.Errorf("decompress tree blob %s: %w", treeID, err)
	}

	var t tree
	if err := json.Unmarshal(jsonData, &t); err != nil {
		return nil, fmt.Errorf("unmarshal tree blob %s: %w", treeID, err)
	}

	return &t, nil
}

// classifyPacks separates index packs into three categories:
// fully unreferenced, partially referenced, and fully referenced.
func (p *Pruner) classifyPacks(referenced map[backend.ID]bool) (fullUnref, partialRef, fullRef []repository.IndexPack) {
	idx := p.repo.Idx()

	for _, ip := range idx.Packs {
		refCount := 0
		for _, b := range ip.Blobs {
			if referenced[b.ID] {
				refCount++
			}
		}

		switch {
		case refCount == 0:
			fullUnref = append(fullUnref, ip)
		case refCount == len(ip.Blobs):
			fullRef = append(fullRef, ip)
		default:
			partialRef = append(partialRef, ip)
		}
	}

	return
}

// repackPacks copies referenced blobs from partially-referenced packs into
// new packs. Returns the new IndexPack entries for the consolidated index.
func (p *Pruner) repackPacks(partialRef []repository.IndexPack, referenced map[backend.ID]bool, maxRepackSize uint64) ([]repository.IndexPack, error) {
	var newPacks []repository.IndexPack
	var totalRepacked uint64

	encKey := p.repo.Key().EncryptKey
	pw := repository.NewPackWriter(encKey)

	for _, ip := range partialRef {
		if maxRepackSize > 0 && totalRepacked >= maxRepackSize {
			break
		}

		packData, err := p.repo.LoadPack(ip.PackID)
		if err != nil {
			return nil, fmt.Errorf("load pack %s for repack: %w", ip.PackID, err)
		}

		for _, b := range ip.Blobs {
			if !referenced[b.ID] {
				continue
			}

			// Read the blob (decrypt from old pack).
			plaintext, err := repository.ReadPackBlob(encKey, packData, b)
			if err != nil {
				return nil, fmt.Errorf("read blob %s from pack %s: %w", b.ID, ip.PackID, err)
			}

			// Add to new pack (re-encrypt).
			if err := pw.AddBlob(b.ID, b.Type, plaintext, b.UncompressedLength); err != nil {
				return nil, fmt.Errorf("add blob %s to new pack: %w", b.ID, err)
			}
		}

		totalRepacked += uint64(len(packData))

		// Flush if pack is large enough.
		if pw.Size() >= 128*1024*1024 {
			ip, err := p.flushPackWriter(pw)
			if err != nil {
				return nil, err
			}
			newPacks = append(newPacks, ip)
			pw = repository.NewPackWriter(encKey)
		}
	}

	// Flush remaining blobs.
	if pw.Count() > 0 {
		ip, err := p.flushPackWriter(pw)
		if err != nil {
			return nil, err
		}
		newPacks = append(newPacks, ip)
	}

	return newPacks, nil
}

// flushPackWriter finalizes and saves a pack, returning the IndexPack entry.
func (p *Pruner) flushPackWriter(pw *repository.PackWriter) (repository.IndexPack, error) {
	packID, packData, err := pw.Finalize()
	if err != nil {
		return repository.IndexPack{}, fmt.Errorf("finalize repack: %w", err)
	}

	savedID, err := p.repo.SavePack(packData)
	if err != nil {
		return repository.IndexPack{}, fmt.Errorf("save repacked pack: %w", err)
	}

	// Read the header to get blob metadata with correct offsets.
	blobs, err := repository.ReadPackHeader(p.repo.Key().EncryptKey, packData)
	if err != nil {
		return repository.IndexPack{}, fmt.Errorf("read repacked header: %w", err)
	}

	_ = packID // savedID is computed from the data, same as packID

	return repository.IndexPack{
		PackID: savedID,
		Blobs:  blobs,
	}, nil
}
