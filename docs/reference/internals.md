# Internals

This document describes HitStor's internal design: encryption, chunking, deduplication, pack format, and repository layout.

## Encryption

All data stored in the repository is encrypted using **AES-256-GCM** (Galois/Counter Mode), which provides both confidentiality and authentication. The ciphertext format is:

```
nonce (12 bytes) || ciphertext || authentication tag (16 bytes)
```

A random 12-byte nonce is generated for each encryption operation.

### Key Derivation

HitStor uses **scrypt** to derive a 64-byte key from the user's password:

| Parameter | Value |
|---|---|
| N (CPU/memory cost) | 32768 |
| r (block size) | 8 |
| p (parallelism) | 1 |
| Salt | 64 random bytes |
| Output | 64 bytes |

The first 32 bytes of the derived key are used as the AES-256-GCM encryption key for the key file. The remaining 32 bytes are not used directly (the master key provides the actual encryption and ID keys).

### Master Key

Each repository has a **master key** consisting of two 32-byte keys:

- **EncryptKey** -- used for AES-256-GCM encryption of all repository data (packs, indexes, snapshots)
- **IDKey** -- used for HMAC-SHA256 blob identification

The master key is generated from 64 cryptographically random bytes and stored in the `keys/` directory, encrypted with the password-derived key.

### Blob IDs

Blob IDs are computed as **HMAC-SHA256(IDKey, data)**, producing a 32-byte identifier. This is content-addressable (the same content always produces the same ID) but not guessable without the IDKey, providing privacy even if an attacker can observe blob IDs.

## Chunking

HitStor uses **FastCDC** (Fast Content-Defined Chunking) with a Gear rolling hash to split file data into variable-size chunks whose boundaries are determined by the content.

| Parameter | Value |
|---|---|
| Minimum chunk size | 256 KiB |
| Average chunk size | 1 MiB |
| Target maximum chunk size | 4 MiB |

The algorithm uses two-phase boundary detection:

1. **Phase 1** (positions min to avg): uses a more permissive mask (more bits set), making it easier to find a boundary. This biases chunks toward the average size.
2. **Phase 2** (positions avg to max): uses a stricter mask (fewer bits set), allowing chunks to grow past the average when no earlier boundary was found.

If no boundary is found before the maximum size, the chunk is cut at the maximum.

Content-defined chunking means that insertions or deletions in a file only affect nearby chunk boundaries, maximizing deduplication across backups.

## Deduplication

Deduplication happens at the chunk level. Before writing a new blob to the repository, HitStor checks:

1. **In-session set** -- blobs already added during the current backup
2. **Repository index** -- the full index of all blobs stored in the repository

For **local CLI backups** (`hitstor backup`), an additional layer applies:

3. **Files cache** -- SQLite database tracking file inode, size, mtime, and chunk IDs

For **remote agent backups**, unchanged files are detected via **previous snapshot metadata** returned in `BeginSession` (`previous_files` with mtime, size, content IDs). No agent-side SQLite cache is required.

## Compression

Each chunk is compressed with **Zstd** (Zstandard) before encryption. Compression is applied per chunk, not per file, which allows individual chunks to be decompressed independently during restore.

## Pack Files

Blobs are not stored individually. Instead, multiple blobs are grouped into **pack files** to reduce the number of objects in the backend and improve I/O efficiency.

| Parameter | Value |
|---|---|
| Pack size threshold | 128 MiB |

When the accumulated blob data in the current pack exceeds 128 MiB, the pack is finalized and written to the backend. Each pack contains:

- Concatenated encrypted blob data
- An encrypted header at the end of the file containing the blob offsets, sizes, types, and IDs

There are two blob types stored in packs:

- **Data blobs** -- file content chunks
- **Tree blobs** -- directory tree structures

## Index Files

Index files map blob IDs to their locations within pack files. Each index entry records:

- The pack ID containing the blob
- The blob ID, type, offset, and lengths (compressed and uncompressed)

Indexes are written **before** snapshots for crash consistency: if a crash occurs after writing the index but before writing the snapshot, the orphaned index and pack data will be cleaned up by the next prune.

## Snapshots

A snapshot records the metadata for a single backup operation:

- Timestamp
- Hostname and username
- Backed-up paths and exclude patterns
- Tags
- Root tree blob ID (the entry point for the directory tree)
- Summary statistics (files new, files unchanged, bytes added, bytes processed, duration)

Snapshots are stored as encrypted JSON files in the `snapshots/` directory.

## Cache

HitStor maintains a local **SQLite** database for incremental backup performance:

```
~/.cache/hitstor/<repo-id>/cache.db
```

The database uses **WAL** (Write-Ahead Logging) mode and tracks:

- **files** -- maps file paths (by SHA-256 hash) to their inode, size, mtime, and chunk IDs. A cache hit (all three metadata fields match) means the file is unchanged and its chunks can be reused without re-reading the file.
- **known_chunks** -- records chunk IDs that exist in the repository, avoiding redundant index lookups.
- **meta** -- generation counter for cache cleanup.

Each cache open increments a generation counter. Old entries not seen for several generations can be cleaned up.

## Repository Layout

```
<repo>/
├── config                          # Unencrypted JSON: repo ID, chunker params
├── data/
│   └── <first-2-hex>/
│       └── <pack-id>              # Encrypted pack files (blobs + header)
├── index/
│   ├── latest                     # Plain-text pointer to consolidated index ID (optional)
│   └── <index-id>                 # Encrypted JSON: blob → pack mapping
├── keys/
│   └── <key-id>                   # Encrypted JSON: master key + KDF params
├── locks/
│   └── <lock-id>                  # Lock files for concurrency control
└── snapshots/
    └── <snapshot-id>              # Encrypted JSON: snapshot metadata + tree ref
```

All IDs are 32-byte values represented as 64-character hex strings. The `data/` directory uses a two-character fan-out prefix (first two hex chars of the pack ID) to avoid excessive directory entries.

## Repository locking

HitStor uses lock files in `locks/` to coordinate writers. An exclusive lock records hostname, PID, UID, and timestamp. A second writer receives an error unless the existing lock is stale (older than 30 minutes and the owning process is not alive on the same host). Use `--break-lock` on CLI commands to remove stale locks before backup, prune, forget, or check.

The server acquires an exclusive lock per active backup session (not for the entire `serve` lifetime), so local CLI backups can run when no remote session is in progress.

Optional durability: set `sync_writes: true` in config or `HITSTOR_SYNC_WRITES=1` to `fsync` local repository files after atomic rename.

On Unix, an additional `flock` on `locks/.exclusive` guards against concurrent writers on local filesystems. The JSON lock files in `locks/` remain the cross-host coordination mechanism.

`min_free_reserve_bytes` in config (default 1 GiB) controls the minimum free disk space required before writing new pack files on local backends.

When more than 10 index files exist, HitStor consolidates them into one and records its ID in `index/latest` so subsequent opens load the consolidated index first.

## Remote Backup Protocol

When using the remote agent (`hitstor-agent`), the backup pipeline is split between agent and server:

**Agent responsibilities:**

- Filesystem scanning
- Content-defined chunking (FastCDC)
- Blob ID computation (HMAC-SHA256 with server-provided IDKey)
- Zstd compression

**Server responsibilities:**

- Deduplication verification
- AES-256-GCM encryption
- Pack file assembly
- Index management
- Snapshot creation

The IDKey is transmitted to the agent at session start, but the EncryptKey never leaves the server. See the [gRPC API Reference](grpc-api.md) for protocol details.
