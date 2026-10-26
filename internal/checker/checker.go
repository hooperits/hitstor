// Package checker verifies the integrity of a HitStor backup repository.
// It validates index consistency, pack file existence, snapshot tree
// references, and optionally reads every pack to verify blob content hashes.
package checker

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/compress"
	"github.com/hooperits/hitstor/internal/crypto"
	"github.com/hooperits/hitstor/internal/repository"
)

// CheckError describes a single integrity issue found during a check.
type CheckError struct {
	Pack    backend.ID // pack that has the issue (zero if N/A)
	Blob    backend.ID // blob that has the issue (zero if N/A)
	Message string     // human-readable error description
}

// Checker verifies repository integrity.
type Checker struct {
	repo *repository.Repository
}

// New creates a new Checker for the given repository.
func New(repo *repository.Repository) *Checker {
	return &Checker{repo: repo}
}

// Check runs repository integrity verification.
// If readData is true, it also reads every pack and verifies blob contents.
// progress is called with a human-readable step description to report progress.
func (c *Checker) Check(readData bool, progress func(step string)) ([]CheckError, error) {
	var errs []CheckError

	// Step 1: Check indexes for duplicate blob IDs.
	progress("checking indexes...")
	slog.Debug("checking indexes for duplicate blob IDs")
	indexErrs := c.checkIndexes()
	errs = append(errs, indexErrs...)

	// Step 2: Check that all pack files referenced by the index exist.
	progress("checking pack files...")
	slog.Debug("checking pack file existence")
	packErrs := c.checkPacks()
	errs = append(errs, packErrs...)

	// Step 3: Check that snapshot tree blobs resolve.
	progress("checking snapshots...")
	slog.Debug("checking snapshot tree blob references")
	snapErrs, err := c.checkSnapshots()
	if err != nil {
		return errs, fmt.Errorf("checker: check snapshots: %w", err)
	}
	errs = append(errs, snapErrs...)

	// Step 4: Optionally read every pack and verify blob content hashes.
	if readData {
		progress("verifying pack data...")
		slog.Debug("verifying all pack data and blob hashes")
		dataErrs := c.verifyPackData()
		errs = append(errs, dataErrs...)
	}

	slog.Info("check complete", "errors_found", len(errs))

	return errs, nil
}

// checkIndexes iterates over all blobs in the merged index and reports
// duplicate blob IDs.
func (c *Checker) checkIndexes() []CheckError {
	var errs []CheckError

	idx := c.repo.Idx()
	seen := make(map[backend.ID]int)

	for _, pack := range idx.Packs {
		for _, blob := range pack.Blobs {
			seen[blob.ID]++
		}
	}

	for blobID, count := range seen {
		if count > 1 {
			errs = append(errs, CheckError{
				Blob:    blobID,
				Message: fmt.Sprintf("duplicate blob ID %s (appears %d times)", blobID, count),
			})
		}
	}

	return errs
}

// checkPacks verifies that every pack file referenced in the index exists
// in the backend.
func (c *Checker) checkPacks() []CheckError {
	var errs []CheckError
	ctx := context.Background()

	idx := c.repo.Idx()

	// Collect unique pack IDs.
	packIDs := make(map[backend.ID]bool)
	for _, pack := range idx.Packs {
		packIDs[pack.PackID] = true
	}

	for packID := range packIDs {
		if _, err := c.repo.Be().Stat(ctx, backend.DataFile, packID); err != nil {
			errs = append(errs, CheckError{
				Pack:    packID,
				Message: fmt.Sprintf("pack file missing: %s", packID),
			})
		}
	}

	return errs
}

// checkSnapshots loads every snapshot and verifies that its Tree blob ID
// can be found in the merged index.
func (c *Checker) checkSnapshots() ([]CheckError, error) {
	var errs []CheckError

	snapIDs, err := c.repo.ListSnapshots()
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}

	for _, snapID := range snapIDs {
		snap, err := c.repo.LoadSnapshot(snapID)
		if err != nil {
			errs = append(errs, CheckError{
				Message: fmt.Sprintf("failed to load snapshot %s: %v", snapID, err),
			})
			continue
		}

		blob, _ := c.repo.LookupBlob(snap.Tree)
		if blob == nil {
			errs = append(errs, CheckError{
				Blob:    snap.Tree,
				Message: fmt.Sprintf("snapshot tree blob missing: %s", snap.Tree),
			})
		}
	}

	return errs, nil
}

// verifyPackData reads every unique pack file, decrypts each blob, decompresses
// it, and verifies that the content hash matches the blob ID.
func (c *Checker) verifyPackData() []CheckError {
	var errs []CheckError
	ctx := context.Background()

	idx := c.repo.Idx()
	mk := c.repo.Key()

	// Collect unique pack IDs.
	packIDs := make(map[backend.ID]bool)
	for _, pack := range idx.Packs {
		packIDs[pack.PackID] = true
	}

	for packID := range packIDs {
		packErrs := c.verifyOnePack(ctx, packID, mk)
		errs = append(errs, packErrs...)
	}

	return errs
}

// verifyOnePack reads a single pack, parses its header, and verifies every
// blob's content hash.
func (c *Checker) verifyOnePack(ctx context.Context, packID backend.ID, mk *crypto.MasterKey) []CheckError {
	var errs []CheckError

	rc, err := c.repo.Be().Load(ctx, backend.DataFile, packID)
	if err != nil {
		errs = append(errs, CheckError{
			Pack:    packID,
			Message: fmt.Sprintf("failed to load pack %s: %v", packID, err),
		})
		return errs
	}
	defer rc.Close()

	// TODO(perf): LoadRange on Backend to stream blob-at-a-time verification
	// instead of loading the full pack into memory.
	data, err := io.ReadAll(rc)
	if err != nil {
		errs = append(errs, CheckError{
			Pack:    packID,
			Message: fmt.Sprintf("failed to read pack %s: %v", packID, err),
		})
		return errs
	}

	blobs, err := repository.ReadPackHeader(mk.EncryptKey, data)
	if err != nil {
		errs = append(errs, CheckError{
			Pack:    packID,
			Message: fmt.Sprintf("failed to read pack header %s: %v", packID, err),
		})
		return errs
	}

	for _, blob := range blobs {
		blobData, err := repository.ReadPackBlob(mk.EncryptKey, data, blob)
		if err != nil {
			errs = append(errs, CheckError{
				Pack:    packID,
				Blob:    blob.ID,
				Message: fmt.Sprintf("failed to decrypt blob %s in pack %s: %v", blob.ID, packID, err),
			})
			continue
		}

		// Decompress the blob data.
		decompressed, err := compress.Decompress(blobData)
		if err != nil {
			errs = append(errs, CheckError{
				Pack:    packID,
				Blob:    blob.ID,
				Message: fmt.Sprintf("failed to decompress blob %s in pack %s: %v", blob.ID, packID, err),
			})
			continue
		}

		// Verify content hash.
		computedID := crypto.BlobID(mk.IDKey, decompressed)
		if backend.ID(computedID) != blob.ID {
			errs = append(errs, CheckError{
				Pack:    packID,
				Blob:    blob.ID,
				Message: fmt.Sprintf("blob content hash mismatch: expected %s, got %s", blob.ID, backend.ID(computedID)),
			})
		}
	}

	return errs
}
