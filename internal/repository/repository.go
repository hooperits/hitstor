package repository

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/crypto"
)

// ChunkerParams holds the chunking parameters for the repository.
type ChunkerParams struct {
	MinSize int `json:"min_size"`
	MaxSize int `json:"max_size"`
	AvgSize int `json:"avg_size"`
}

// Config is the unencrypted repository configuration.
type Config struct {
	Version       int           `json:"version"`
	ID            string        `json:"id"`
	ChunkerParams ChunkerParams `json:"chunker_params"`
}

// DefaultChunkerParams returns the default chunker parameters.
func DefaultChunkerParams() ChunkerParams {
	return ChunkerParams{
		MinSize: 256 * 1024,      // 256 KiB
		MaxSize: 4 * 1024 * 1024, // 4 MiB
		AvgSize: 1024 * 1024,     // 1 MiB
	}
}

// Lock represents a repository lock entry.
type Lock struct {
	Time      time.Time `json:"time"`
	Hostname  string    `json:"hostname"`
	PID       int       `json:"pid"`
	UID       uint32    `json:"uid"`
	Exclusive bool      `json:"exclusive"`
}

type packedBlobRef struct {
	blob   PackedBlob
	packID ID
}

type packCacheEntry struct {
	packID ID
	data   []byte
}

// Repository represents an opened backup repository.
type Repository struct {
	be        backend.Backend
	masterKey *crypto.MasterKey
	config    Config

	// In-memory merged index.
	idx   *Index
	idxMu sync.RWMutex

	// Lookup map for O(1) LookupBlob.
	blobMap map[ID]packedBlobRef

	// Pack cache.
	packCache    []packCacheEntry
	packCacheMax int
	packCacheMu  sync.Mutex

	// Lock tracking.
	lockMu       sync.Mutex
	lockID       *ID
	lockRefCount int
	flockFile    interface{ Close() error }

	minFreeReserve uint64
}

// Init creates a new repository with the given backend and password.
// It generates a random repository ID, writes the config file, creates a
// master key, and saves an encrypted key file.
func Init(be backend.Backend, password string) (*Repository, error) {
	ctx := context.Background()

	exists, err := ConfigExists(be)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("repository: config already exists at this location")
	}

	// Generate random repository ID (32 bytes = 64 hex chars).
	var idBytes [32]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return nil, fmt.Errorf("repository: generate ID: %w", err)
	}

	cfg := Config{
		Version:       1,
		ID:            hex.EncodeToString(idBytes[:]),
		ChunkerParams: DefaultChunkerParams(),
	}

	// Save config (unencrypted).
	cfgData, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("repository: marshal config: %w", err)
	}
	if err := be.Save(ctx, backend.ConfigFile, ID{}, bytes.NewReader(cfgData)); err != nil {
		return nil, fmt.Errorf("repository: save config: %w", err)
	}

	// Generate master key.
	masterKey, err := crypto.NewMasterKey()
	if err != nil {
		return nil, fmt.Errorf("repository: generate master key: %w", err)
	}

	// Create key file.
	keyFile, err := crypto.CreateKeyFile(masterKey, password)
	if err != nil {
		return nil, fmt.Errorf("repository: create key file: %w", err)
	}

	// Serialize and save key file (ID = SHA-256 of JSON content).
	keyData, err := json.Marshal(keyFile)
	if err != nil {
		return nil, fmt.Errorf("repository: marshal key file: %w", err)
	}
	keyID := ID(sha256.Sum256(keyData))
	if err := be.Save(ctx, backend.KeyFile, keyID, bytes.NewReader(keyData)); err != nil {
		return nil, fmt.Errorf("repository: save key file: %w", err)
	}

	return &Repository{
		be:        be,
		masterKey: masterKey,
		config:    cfg,
		idx:       &Index{},
		blobMap:   make(map[ID]packedBlobRef),
	}, nil
}

// Open opens an existing repository by loading its config, decrypting a key
// file with the given password, and loading all index files into memory.
func Open(be backend.Backend, password string) (*Repository, error) {
	ctx := context.Background()

	// Load config.
	cfgReader, err := be.Load(ctx, backend.ConfigFile, ID{})
	if err != nil {
		return nil, fmt.Errorf("repository: load config: %w", err)
	}
	defer cfgReader.Close()

	cfgData, err := io.ReadAll(cfgReader)
	if err != nil {
		return nil, fmt.Errorf("repository: read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		return nil, fmt.Errorf("repository: unmarshal config: %w", err)
	}

	// Load key files and try to open with password.
	keyIDs, err := be.List(ctx, backend.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("repository: list key files: %w", err)
	}
	if len(keyIDs) == 0 {
		return nil, fmt.Errorf("repository: no key files found")
	}

	var masterKey *crypto.MasterKey
	for _, keyID := range keyIDs {
		rd, err := be.Load(ctx, backend.KeyFile, keyID)
		if err != nil {
			continue
		}

		data, err := io.ReadAll(rd)
		rd.Close()
		if err != nil {
			continue
		}

		var kf crypto.KeyFile
		if err := json.Unmarshal(data, &kf); err != nil {
			continue
		}

		mk, err := crypto.OpenKeyFile(&kf, password)
		if err != nil {
			continue
		}

		masterKey = mk
		break
	}

	if masterKey == nil {
		return nil, fmt.Errorf("repository: wrong password or corrupted key file")
	}

	repo := &Repository{
		be:        be,
		masterKey: masterKey,
		config:    cfg,
		idx:       &Index{},
		blobMap:   make(map[ID]packedBlobRef),
	}

	if err := repo.loadIndexes(ctx, masterKey.EncryptKey); err != nil {
		return nil, err
	}

	return repo, nil
}

func (r *Repository) loadIndexes(ctx context.Context, encryptKey [32]byte) error {
	indexIDs, err := r.be.List(ctx, backend.IndexFile)
	if err != nil {
		return fmt.Errorf("repository: list index files: %w", err)
	}

	latestID, hasLatest, err := ReadLatestIndexID(r.be)
	if err != nil {
		return fmt.Errorf("repository: read index latest: %w", err)
	}

	loadOrder := indexIDs
	if hasLatest {
		loadOrder = make([]ID, 0, len(indexIDs))
		loadOrder = append(loadOrder, latestID)
		for _, indexID := range indexIDs {
			if indexID != latestID {
				loadOrder = append(loadOrder, indexID)
			}
		}
	}

	seen := make(map[ID]bool, len(loadOrder))
	for _, indexID := range loadOrder {
		if seen[indexID] {
			continue
		}
		seen[indexID] = true

		rd, err := r.be.Load(ctx, backend.IndexFile, indexID)
		if err != nil {
			return fmt.Errorf("repository: load index %s: %w", indexID, err)
		}

		data, err := io.ReadAll(rd)
		rd.Close()
		if err != nil {
			return fmt.Errorf("repository: read index %s: %w", indexID, err)
		}

		idx, err := DecodeIndex(encryptKey, data)
		if err != nil {
			return fmt.Errorf("repository: decode index %s: %w", indexID, err)
		}

		r.idx.Packs = append(r.idx.Packs, idx.Packs...)
	}

	r.rebuildBlobMap()
	return nil
}

// rebuildBlobMap scans idx.Packs and rebuilds the O(1) lookup map.
// Note: This helper must be called under a write lock if idx is being updated concurrently.
func (r *Repository) rebuildBlobMap() {
	r.blobMap = make(map[ID]packedBlobRef)
	for i := range r.idx.Packs {
		for j := range r.idx.Packs[i].Blobs {
			blob := r.idx.Packs[i].Blobs[j]
			packID := r.idx.Packs[i].PackID
			r.blobMap[blob.ID] = packedBlobRef{
				blob:   blob,
				packID: packID,
			}
		}
	}
}

// SetPackCacheMax sets the maximum number of pack files to keep in the in-memory cache.
// A size of 0 triggers the default size (4 packs). A negative size disables the cache.
func (r *Repository) SetPackCacheMax(n int) {
	r.packCacheMu.Lock()
	defer r.packCacheMu.Unlock()
	r.packCacheMax = n
	if n < 0 {
		r.packCache = nil
	} else if n > 0 && len(r.packCache) > n {
		r.packCache = r.packCache[:n]
	}
}

// LoadPackCached retrieves the pack data from the in-memory cache, or loads it from the backend and caches it.
func (r *Repository) LoadPackCached(packID ID) ([]byte, error) {
	r.packCacheMu.Lock()

	// Default to 4 packs (512 MiB total for 128 MiB packs) if not configured.
	max := r.packCacheMax
	if max == 0 {
		max = 4
	}

	// Check if the cache is active.
	if max > 0 {
		// Check if in cache.
		for i, entry := range r.packCache {
			if entry.packID == packID {
				// Move to the front (LRU promotion).
				if i > 0 {
					r.packCache[0], r.packCache[i] = r.packCache[i], r.packCache[0]
				}
				data := entry.data
				r.packCacheMu.Unlock()
				return data, nil
			}
		}
	}
	r.packCacheMu.Unlock()

	// Cache miss — load from backend.
	data, err := r.LoadPack(packID)
	if err != nil {
		return nil, err
	}

	// Insert into cache.
	r.packCacheMu.Lock()
	defer r.packCacheMu.Unlock()

	// Recheck max under lock.
	max = r.packCacheMax
	if max == 0 {
		max = 4
	}

	if max <= 0 {
		return data, nil
	}

	// Check again in case another goroutine loaded it.
	for i, entry := range r.packCache {
		if entry.packID == packID {
			if i > 0 {
				r.packCache[0], r.packCache[i] = r.packCache[i], r.packCache[0]
			}
			return entry.data, nil
		}
	}

	// Insert at the front.
	newEntry := packCacheEntry{packID: packID, data: data}
	if len(r.packCache) < max {
		// Grow the slice by prepending.
		r.packCache = append([]packCacheEntry{newEntry}, r.packCache...)
	} else {
		// Evict the last element (LRU eviction).
		copy(r.packCache[1:], r.packCache[:max-1])
		r.packCache[0] = newEntry
	}

	return data, nil
}

// Unlock removes the repository lock.
func (r *Repository) Unlock() error {
	r.lockMu.Lock()
	defer r.lockMu.Unlock()

	if r.lockID == nil {
		return nil
	}
	if r.lockRefCount > 1 {
		r.lockRefCount--
		return nil
	}

	if r.flockFile != nil {
		_ = r.flockFile.Close()
		r.flockFile = nil
	}

	ctx := context.Background()
	if err := r.be.Remove(ctx, backend.LockFile, *r.lockID); err != nil {
		return fmt.Errorf("repository: remove lock: %w", err)
	}

	r.lockID = nil
	r.lockRefCount = 0
	return nil
}

// SavePack saves raw pack data to the backend. Returns the pack ID (SHA-256
// of the data).
func (r *Repository) SavePack(data []byte) (ID, error) {
	if err := r.CheckFreeSpace(r.minFreeRequired()); err != nil {
		return ID{}, err
	}

	ctx := context.Background()

	packID := ID(sha256.Sum256(data))
	if err := r.be.Save(ctx, backend.DataFile, packID, bytes.NewReader(data)); err != nil {
		return ID{}, fmt.Errorf("repository: save pack: %w", err)
	}

	return packID, nil
}

// LoadBlob loads a single blob from a pack file, decrypting it with the
// master key.
func (r *Repository) LoadBlob(packID ID, blob PackedBlob) ([]byte, error) {
	type rangedLoader interface {
		LoadRange(ctx context.Context, ft backend.FileType, id ID, offset, length int64) (io.ReadCloser, error)
	}
	if lr, ok := r.be.(rangedLoader); ok {
		rd, err := lr.LoadRange(context.Background(), backend.DataFile, packID, int64(blob.Offset), int64(blob.Length))
		if err == nil {
			slice, readErr := io.ReadAll(rd)
			rd.Close()
			if readErr == nil {
				plaintext, decErr := crypto.Decrypt(r.masterKey.EncryptKey, slice)
				if decErr == nil {
					return plaintext, nil
				}
			}
		}
	}

	packData, err := r.LoadPackCached(packID)
	if err != nil {
		return nil, err
	}
	return ReadPackBlob(r.masterKey.EncryptKey, packData, blob)
}

// SaveIndex encodes and saves an index to the backend. The index is encrypted
// with the master key. Returns the index ID (SHA-256 of encrypted content).
// The new index packs are also merged into the in-memory index.
func (r *Repository) SaveIndex(idx *Index) (ID, error) {
	ctx := context.Background()

	data, err := EncodeIndex(r.masterKey.EncryptKey, idx)
	if err != nil {
		return ID{}, fmt.Errorf("repository: encode index: %w", err)
	}

	indexID := ID(sha256.Sum256(data))
	if err := r.be.Save(ctx, backend.IndexFile, indexID, bytes.NewReader(data)); err != nil {
		return ID{}, fmt.Errorf("repository: save index: %w", err)
	}

	// Merge into in-memory index.
	r.idxMu.Lock()
	r.idx.Packs = append(r.idx.Packs, idx.Packs...)
	for i := range idx.Packs {
		for j := range idx.Packs[i].Blobs {
			blob := idx.Packs[i].Blobs[j]
			packID := idx.Packs[i].PackID
			r.blobMap[blob.ID] = packedBlobRef{
				blob:   blob,
				packID: packID,
			}
		}
	}
	r.idxMu.Unlock()

	return indexID, nil
}

// LoadIndex loads and decodes a single index from the backend.
func (r *Repository) LoadIndex(indexID ID) (*Index, error) {
	ctx := context.Background()

	rd, err := r.be.Load(ctx, backend.IndexFile, indexID)
	if err != nil {
		return nil, fmt.Errorf("repository: load index %s: %w", indexID, err)
	}
	defer rd.Close()

	data, err := io.ReadAll(rd)
	if err != nil {
		return nil, fmt.Errorf("repository: read index %s: %w", indexID, err)
	}

	return DecodeIndex(r.masterKey.EncryptKey, data)
}

// SaveSnapshot encodes and saves a snapshot to the backend. Returns the
// snapshot ID (SHA-256 of encrypted content).
func (r *Repository) SaveSnapshot(snap *Snapshot) (ID, error) {
	ctx := context.Background()

	data, err := EncodeSnapshot(r.masterKey.EncryptKey, snap)
	if err != nil {
		return ID{}, fmt.Errorf("repository: encode snapshot: %w", err)
	}

	snapID := ID(sha256.Sum256(data))
	if err := r.be.Save(ctx, backend.SnapshotFile, snapID, bytes.NewReader(data)); err != nil {
		return ID{}, fmt.Errorf("repository: save snapshot: %w", err)
	}

	return snapID, nil
}

// LoadSnapshot loads and decodes a snapshot from the backend.
func (r *Repository) LoadSnapshot(snapID ID) (*Snapshot, error) {
	ctx := context.Background()

	rd, err := r.be.Load(ctx, backend.SnapshotFile, snapID)
	if err != nil {
		return nil, fmt.Errorf("repository: load snapshot %s: %w", snapID, err)
	}
	defer rd.Close()

	data, err := io.ReadAll(rd)
	if err != nil {
		return nil, fmt.Errorf("repository: read snapshot %s: %w", snapID, err)
	}

	return DecodeSnapshot(r.masterKey.EncryptKey, data)
}

// ListSnapshots lists all snapshot IDs in the repository.
func (r *Repository) ListSnapshots() ([]ID, error) {
	return r.be.List(context.Background(), backend.SnapshotFile)
}

// RemoveSnapshot deletes a snapshot from the backend.
func (r *Repository) RemoveSnapshot(snapID ID) error {
	return r.be.Remove(context.Background(), backend.SnapshotFile, snapID)
}

// LookupBlob searches the in-memory index for a blob by ID. Returns the
// PackedBlob metadata and the pack ID, or nil if not found.
func (r *Repository) LookupBlob(blobID ID) (*PackedBlob, *ID) {
	r.idxMu.RLock()
	defer r.idxMu.RUnlock()

	if ref, ok := r.blobMap[blobID]; ok {
		blob := ref.blob
		packID := ref.packID
		return &blob, &packID
	}
	return nil, nil
}

// Cfg returns the repository configuration.
func (r *Repository) Cfg() Config {
	return r.config
}

// Key returns the master key.
func (r *Repository) Key() *crypto.MasterKey {
	return r.masterKey
}

// Idx returns the merged in-memory index.
func (r *Repository) Idx() *Index {
	r.idxMu.RLock()
	defer r.idxMu.RUnlock()
	return r.idx
}

// Be returns the backend.
func (r *Repository) Be() backend.Backend {
	return r.be
}

// RemovePack deletes a pack file from the backend.
func (r *Repository) RemovePack(packID ID) error {
	if err := r.be.Remove(context.Background(), backend.DataFile, packID); err != nil {
		return fmt.Errorf("repository: remove pack %s: %w", packID, err)
	}
	return nil
}

// RemoveIndex deletes an index file from the backend.
func (r *Repository) RemoveIndex(indexID ID) error {
	if err := r.be.Remove(context.Background(), backend.IndexFile, indexID); err != nil {
		return fmt.Errorf("repository: remove index %s: %w", indexID, err)
	}
	return nil
}

// ListIndexes returns all index file IDs from the backend.
func (r *Repository) ListIndexes() ([]ID, error) {
	return r.be.List(context.Background(), backend.IndexFile)
}

// ReplaceIndex atomically replaces the in-memory merged index.
func (r *Repository) ReplaceIndex(idx *Index) {
	r.idxMu.Lock()
	defer r.idxMu.Unlock()
	r.idx = idx
	r.rebuildBlobMap()
}

// IndexConsolidateThreshold is the number of on-disk index files that triggers consolidation.
const IndexConsolidateThreshold = 10

// MaybeConsolidateIndexes rewrites many index files into one consolidated index.
func (r *Repository) MaybeConsolidateIndexes() error {
	oldIDs, err := r.ListIndexes()
	if err != nil {
		return err
	}
	if len(oldIDs) <= IndexConsolidateThreshold {
		return nil
	}

	r.idxMu.RLock()
	consolidated := &Index{Packs: append([]IndexPack(nil), r.idx.Packs...)}
	r.idxMu.RUnlock()

	r.ReplaceIndex(consolidated)
	newID, err := r.SaveIndex(consolidated)
	if err != nil {
		return fmt.Errorf("repository: consolidate indexes: %w", err)
	}
	r.ReplaceIndex(consolidated)

	for _, id := range oldIDs {
		if id == newID {
			continue
		}
		if err := r.RemoveIndex(id); err != nil {
			return fmt.Errorf("repository: remove old index %s: %w", id, err)
		}
	}
	return SetLatestIndexID(r.be, newID)
}

// LoadPack loads the raw pack file data from the backend.
func (r *Repository) LoadPack(packID ID) ([]byte, error) {
	ctx := context.Background()

	rd, err := r.be.Load(ctx, backend.DataFile, packID)
	if err != nil {
		return nil, fmt.Errorf("repository: load pack %s: %w", packID, err)
	}
	defer rd.Close()

	data, err := io.ReadAll(rd)
	if err != nil {
		return nil, fmt.Errorf("repository: read pack %s: %w", packID, err)
	}

	return data, nil
}
