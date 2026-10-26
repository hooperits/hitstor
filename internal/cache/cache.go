// Package cache provides a local SQLite-backed cache for file metadata,
// enabling incremental backups by tracking which files have changed.
package cache

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// FilesCache provides a local SQLite cache for file metadata to enable
// incremental backups.
type FilesCache struct {
	db         *sql.DB
	generation int64

	stmtLookupFile    *sql.Stmt
	stmtUpdateFile    *sql.Stmt
	stmtIsKnownChunk  *sql.Stmt
	stmtAddKnownChunk *sql.Stmt
	stmtUpdateSeen    *sql.Stmt
}

// Open creates or opens a SQLite cache database at the given path.
// Uses WAL mode for concurrent read access. The generation counter is
// incremented each time the cache is opened.
func Open(cachePath string) (*FilesCache, error) {
	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		return nil, fmt.Errorf("cache: create directory: %w", err)
	}

	db, err := sql.Open("sqlite", cachePath)
	if err != nil {
		return nil, fmt.Errorf("cache: open db: %w", err)
	}

	// Enable WAL mode for better concurrent performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("cache: set WAL mode: %w", err)
	}

	// Create tables if they don't exist.
	const schema = `
		CREATE TABLE IF NOT EXISTS meta (
			key   TEXT PRIMARY KEY,
			value INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS files (
			path_hash  BLOB PRIMARY KEY,
			inode      INTEGER NOT NULL,
			size       INTEGER NOT NULL,
			mtime_ns   INTEGER NOT NULL,
			chunk_ids  BLOB NOT NULL,
			last_seen  INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS known_chunks (
			chunk_id BLOB PRIMARY KEY,
			size     INTEGER NOT NULL
		);
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("cache: create tables: %w", err)
	}

	// Increment generation counter.
	gen, err := incrementGeneration(db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("cache: increment generation: %w", err)
	}

	fc := &FilesCache{db: db, generation: gen}

	var prepErr error
	if fc.stmtLookupFile, prepErr = db.Prepare(`SELECT chunk_ids FROM files WHERE path_hash = ? AND inode = ? AND size = ? AND mtime_ns = ?`); prepErr != nil {
		fc.Close()
		return nil, fmt.Errorf("cache: prepare LookupFile statement: %w", prepErr)
	}
	if fc.stmtUpdateFile, prepErr = db.Prepare(`INSERT OR REPLACE INTO files (path_hash, inode, size, mtime_ns, chunk_ids, last_seen) VALUES (?, ?, ?, ?, ?, ?)`); prepErr != nil {
		fc.Close()
		return nil, fmt.Errorf("cache: prepare UpdateFile statement: %w", prepErr)
	}
	if fc.stmtIsKnownChunk, prepErr = db.Prepare(`SELECT 1 FROM known_chunks WHERE chunk_id = ?`); prepErr != nil {
		fc.Close()
		return nil, fmt.Errorf("cache: prepare IsKnownChunk statement: %w", prepErr)
	}
	if fc.stmtAddKnownChunk, prepErr = db.Prepare(`INSERT OR REPLACE INTO known_chunks (chunk_id, size) VALUES (?, ?)`); prepErr != nil {
		fc.Close()
		return nil, fmt.Errorf("cache: prepare AddKnownChunk statement: %w", prepErr)
	}
	if fc.stmtUpdateSeen, prepErr = db.Prepare(`UPDATE files SET last_seen = ? WHERE path_hash = ?`); prepErr != nil {
		fc.Close()
		return nil, fmt.Errorf("cache: prepare UpdateSeen statement: %w", prepErr)
	}

	return fc, nil
}

// incrementGeneration atomically reads, increments, and stores the
// generation counter in the meta table.
func incrementGeneration(db *sql.DB) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var gen int64
	err = tx.QueryRow("SELECT value FROM meta WHERE key = 'generation'").Scan(&gen)
	if err == sql.ErrNoRows {
		gen = 0
	} else if err != nil {
		return 0, err
	}

	gen++
	_, err = tx.Exec(
		"INSERT OR REPLACE INTO meta (key, value) VALUES ('generation', ?)", gen,
	)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return gen, nil
}

// Close closes the database.
func (c *FilesCache) Close() error {
	if c.db == nil {
		return nil
	}

	if c.stmtLookupFile != nil {
		c.stmtLookupFile.Close()
	}
	if c.stmtUpdateFile != nil {
		c.stmtUpdateFile.Close()
	}
	if c.stmtIsKnownChunk != nil {
		c.stmtIsKnownChunk.Close()
	}
	if c.stmtAddKnownChunk != nil {
		c.stmtAddKnownChunk.Close()
	}
	if c.stmtUpdateSeen != nil {
		c.stmtUpdateSeen.Close()
	}

	return c.db.Close()
}

// Generation returns the current generation counter.
func (c *FilesCache) Generation() int64 {
	return c.generation
}

// LookupFile checks if a file with the given path hash has unchanged
// metadata (inode, size, mtime). Returns the cached chunk IDs if found.
// pathHash is SHA-256 of the absolute file path.
//
// All three fields (inode, size, mtimeNs) must match for a cache hit.
// On a hit, last_seen is updated to the current generation.
func (c *FilesCache) LookupFile(pathHash [32]byte, inode uint64, size uint64, mtimeNs int64) ([][32]byte, bool) {
	var chunkBlob []byte
	err := c.stmtLookupFile.QueryRow(
		pathHash[:], int64(inode), int64(size), mtimeNs,
	).Scan(&chunkBlob)

	if err != nil {
		// Either not found or metadata mismatch.
		return nil, false
	}

	// Update last_seen to the current generation.
	_, _ = c.stmtUpdateSeen.Exec(
		c.generation, pathHash[:],
	)

	return unpackChunkIDs(chunkBlob), true
}

// UpdateFile stores or updates the file entry with its chunk IDs.
func (c *FilesCache) UpdateFile(pathHash [32]byte, inode uint64, size uint64, mtimeNs int64, chunkIDs [][32]byte) {
	blob := packChunkIDs(chunkIDs)
	_, _ = c.stmtUpdateFile.Exec(
		pathHash[:], int64(inode), int64(size), mtimeNs, blob, c.generation,
	)
}

// IsKnownChunk checks if a chunk ID exists in the known_chunks table.
func (c *FilesCache) IsKnownChunk(id [32]byte) bool {
	var n int
	err := c.stmtIsKnownChunk.QueryRow(id[:]).Scan(&n)
	return err == nil
}

// AddKnownChunk records a chunk in the known_chunks table.
func (c *FilesCache) AddKnownChunk(id [32]byte, size int) {
	_, _ = c.stmtAddKnownChunk.Exec(id[:], size)
}

// Cleanup removes file entries not seen for the given number of generations.
// It deletes entries where (current_generation - last_seen) >= maxGenerations.
func (c *FilesCache) Cleanup(maxGenerations int64) error {
	cutoff := c.generation - maxGenerations
	_, err := c.db.Exec("DELETE FROM files WHERE last_seen <= ?", cutoff)
	return err
}

// packChunkIDs converts a slice of [32]byte IDs into a single contiguous
// byte slice (each ID is 32 bytes, total = N*32).
func packChunkIDs(ids [][32]byte) []byte {
	buf := make([]byte, 0, len(ids)*32)
	for _, id := range ids {
		buf = append(buf, id[:]...)
	}
	return buf
}

// unpackChunkIDs converts a contiguous byte slice back into a slice of
// [32]byte IDs.
func unpackChunkIDs(blob []byte) [][32]byte {
	n := len(blob) / 32
	ids := make([][32]byte, n)
	for i := 0; i < n; i++ {
		copy(ids[i][:], blob[i*32:(i+1)*32])
	}
	return ids
}
