<div align="center">

<div style="display: inline-block; padding: 2.5rem 3rem; border-radius: 12px; background: linear-gradient(145deg, #071018 0%, #0b1f3a 42%, #132d4f 100%); text-align: center; max-width: 480px;">

<img src="assets/icon.png" alt="" width="72" height="72" style="border-radius: 18px; box-shadow: 0 12px 32px rgba(0,0,0,0.35); margin-bottom: 1.5rem;">

<h1 style="margin: 0 0 0.75rem; font-family: system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif; font-size: 2.75rem; font-weight: 800; letter-spacing: -0.04em; line-height: 1;">
<span style="color: #f4f7fb">hit</span><span style="color: #ff7a45">stor</span>
</h1>

<p style="margin: 0; font-family: system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif; font-size: 1.05rem; line-height: 1.5; color: rgba(244,247,251,0.88);">
Encrypted, deduplicated backups for your infrastructure.
</p>

</div>

<br><br>

**Single static binary, zero runtime dependencies.**

<br>

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white)](go.mod)
[![License](https://img.shields.io/badge/License-AGPL--3.0-0B1F3A?style=flat-square)](LICENSE)
[![Build](https://img.shields.io/badge/Build-CGO__disabled-0B1F3A?style=flat-square)](#build)
[![Dashboard](https://img.shields.io/badge/Dashboard-embedded-E8602C?style=flat-square)](#web-dashboard)
[![Agents](https://img.shields.io/badge/Agents-gRPC_TLS-FF7A45?style=flat-square)](#remote-backup-agent)

<br>

[Quick Start](#quick-start) · [Architecture](#architecture) · [Dashboard](#web-dashboard) · [Agents](#remote-backup-agent) · [Plugins](#plugin-backends) · [Development](#development)

</div>

**English** | [Español](README.es.md)

---

## Table of Contents

| | |
|---|---|
| [Features](#features) | [Quick Start](#quick-start) · [Architecture](#architecture) |
| [Repository Storage](#repository-storage) | [Configuration](#configuration) |
| [Remote Backup Server](#remote-backup-server) | [Remote Backup Agent](#remote-backup-agent) |
| [Web Dashboard](#web-dashboard) | [Plugin Backends](#plugin-backends) |
| [Development](#development) | [License](#license) |

---

## Features

### Security & storage engine

| | |
|---|---|
| **Client-side encryption** | AES-256-GCM with scrypt key derivation — data is encrypted before it leaves the machine |
| **Content-defined deduplication** | FastCDC chunking (256 KiB–4 MiB) identifies identical blocks across backups |
| **Compression** | Zstd applied per chunk before encryption |
| **Multiple backends** | Local filesystem, S3-compatible storage (AWS S3, MinIO, etc.), and plugin backends |
| **Single static binary** | `CGO_ENABLED=0` — zero runtime dependencies |

### Backup & restore

| | |
|---|---|
| **Incremental backups** | Server sends snapshot metadata to agents; mtime/size comparison skips unchanged files — no local state required |
| **Snapshot types** | Full and incremental labels in the dashboard with parent traceability |
| **Periodic full re-scan** | Configurable interval guards against silent corruption; trigger on demand from the dashboard |
| **Parallel I/O** | Worker pools for 2×+ faster backup and restore on multi-core systems |
| **Dashboard restore** | Browse file trees, pick paths, target an agent, monitor progress via SSE |

### Remote operations

| | |
|---|---|
| **Remote backup server** | `hitstor serve` — gRPC over TLS; agents upload pre-chunked data; API key per agent |
| **Remote backup agent** | `hitstor-agent` for Linux amd64/arm64 — systemd timers or server-managed daemon scheduling |
| **Live backup progress** | Dashboard shows running jobs, phase, throughput; stale-session warnings on Overview |
| **Live progress bar** | Terminal UI: `[####----] 42% \| 3,200/7,600 files \| 45.2 MiB/s \| ETA 1m23s` |

### Dashboard & UX

| | |
|---|---|
| **Web dashboard** | Embedded HTML5 UI — agents, snapshots, storage, live progress; optional password auth; htmx + Chart.js in-binary |
| **Colored output** | ANSI terminal colors with [NO_COLOR](https://no-color.org) support |
| **User-friendly errors** | Pattern-matched messages with contextual hints |

### Plugins & integrations

HitStor is a filesystem backup and restore product for Windows and Linux.

HitStor uses a modular plugin architecture. The core `hitstor` and `hitstor-agent` binaries focus on the engine and filesystem backup (via the built-in scanner plugin). Advanced backup sources are delivered as separate optional plugins.

| | |
|---|---|
| **Plugin backends** | HashiCorp go-plugin storage backends (`StorageBackend` proto) |
| **Plugin notifiers** | Slack, PagerDuty, email, etc. via `Notifier` proto |
| **Plugin hooks** | Pre/post-backup lifecycle via `Hook` proto |

---

## Quick Start

> **Fastest path (server + agents):** [docs/installation/quickstart.md](docs/installation/quickstart.md)

```bash
cp .env.example .env   # set HITSTOR_PASSWORD
docker compose -f docker-compose.quickstart.yml up -d
# Dashboard → http://localhost:8421 — then install hitstor-agent on each host
```

### Build

```bash
# Standard build
go build -o hitstor ./cmd/hitstor

# Build with version injection
make build

# Install to your PATH
sudo cp bin/hitstor /usr/local/bin/
```

### Initialize a Repository

```bash
# Local
hitstor init --repo /path/to/repo --password "secret"

# S3
hitstor init --repo s3:bucket-name/prefix \
  --password "secret" \
  --s3-access-key KEY --s3-secret-key SECRET
```

### Backup

```bash
hitstor backup --repo /path/to/repo --password "secret" /data/to/backup

# Control parallelism (default: auto-detect, max 8)
hitstor backup --repo /path/to/repo --password "secret" --parallel 4 /data/to/backup

# Preview what would be backed up (no data written)
hitstor backup --repo /path/to/repo --password "secret" --dry-run /data/to/backup
```

### List Snapshots

```bash
hitstor snapshots --repo /path/to/repo --password "secret"

# Alias
hitstor ls --repo /path/to/repo --password "secret"
```

### Restore

```bash
hitstor restore --repo /path/to/repo --password "secret" SNAPSHOT_ID /target/path
```

### Compare Snapshots

```bash
hitstor diff --repo /path/to/repo --password "secret" SNAPSHOT_ID_1 SNAPSHOT_ID_2
```

### Repository Disk Usage

```bash
hitstor du --repo /path/to/repo --password "secret"
```

### Version

```bash
hitstor version
```

### Verify Repository Integrity

```bash
hitstor check --repo /path/to/repo --password "secret"
hitstor check --repo /path/to/repo --password "secret" --read-data  # full blob verification
```

### Forget Snapshots

```bash
# Forget by explicit ID (alias: hitstor rm)
hitstor forget --repo /path/to/repo --password "secret" SNAPSHOT_ID

# Forget by retention policy
hitstor forget --repo /path/to/repo --password "secret" --keep-last 5 --keep-daily 7 --keep-weekly 4

# Forget + prune in one command
hitstor forget --repo /path/to/repo --password "secret" --keep-last 5 --prune

# Preview what would be forgotten
hitstor forget --repo /path/to/repo --password "secret" --keep-last 3 --dry-run
```

Retention flags: `--keep-last`, `--keep-hourly`, `--keep-daily`,
`--keep-weekly`, `--keep-monthly`, `--keep-yearly`, `--keep-tag`.
Filter flags: `--host`, `--path`.

### Prune (Reclaim Storage)

```bash
hitstor prune --repo /path/to/repo --password "secret"
hitstor prune --repo /path/to/repo --password "secret" --dry-run
hitstor prune --repo /path/to/repo --password "secret" --max-repack-size 500MiB
```

---

## Architecture

```
cmd/hitstor/                    CLI + hitstor serve
cmd/hitstor-agent/              Remote backup agent
cmd/hitstor-scanner-builtin/    Built-in filesystem scanner plugin (ships with core)
api/proto/                      Plugin gRPC definitions (storage, notifier, hook, scanner)
internal/
  archiver/                     Local backup pipeline
  server/                       gRPC BackupService, sessions, multi-repo
  agent/                        Remote backup client + AgentService
  dashboard/                    Embedded web UI + HTTP API
  discovery/                    LAN agent auto-discovery
  sentinel/                     Ransomware telemetry (Gemini)
  repository/                   Content-addressable store
  plugin/                       HashiCorp go-plugin manager
  config/                       YAML config + repositories[]
  ...                           chunker, crypto, retention, metrics, etc.
```

Full map: [docs/dev-guide/codebase-overview.md](docs/dev-guide/codebase-overview.md)

---

## Repository Storage

When you run `hitstor init`, a repository is created with this directory structure:

```
<repo-root>/
├── config                     # Repository metadata (unencrypted JSON)
├── keys/
│   └── <key_id>               # Password-encrypted master key
├── data/
│   ├── ab/                    # Fan-out by first 2 hex chars
│   │   └── ab3f...pack_id     # Encrypted pack files
│   └── ...
├── index/
│   └── <index_id>             # Encrypted blob-to-pack lookup tables
├── snapshots/
│   └── <snapshot_id>          # Encrypted snapshot metadata
└── locks/
    └── <lock_id>              # Exclusive lock files during operations
```

All object IDs are 32-byte SHA-256 hashes displayed as 64 hex characters. Snapshot IDs shown to users are truncated to 8 characters for readability.

### Config File

The only unencrypted file. Contains the repository ID and chunking parameters:

```json
{
  "version": 1,
  "id": "64-hex-char-repository-id",
  "chunker_params": {
    "min_size": 262144,
    "max_size": 4194304,
    "avg_size": 1048576
  }
}
```

### Encryption

All data (packs, indexes, snapshots) is encrypted with **AES-256-GCM** before being written to disk:

1. **Key derivation** — Your password is run through **scrypt** (N=32768, r=8, p=1) with a random 64-byte salt to derive a 64-byte key.
2. **Master key** — The first 32 bytes are the **encryption key** (AES-256-GCM). The last 32 bytes are the **ID key** (HMAC-SHA256 for content addressing).
3. **Per-blob encryption** — Each data blob gets a fresh random 12-byte nonce. Output format: `nonce || ciphertext || GCM-tag`.
4. **Tamper detection** — GCM authentication tags detect any corruption or modification automatically.

The master key is stored in `keys/` encrypted with your password. The password itself is never written to disk.

### How Backups Work

The backup pipeline processes your files through these stages:

```
Files → Scan → Chunk (FastCDC) → Compress (Zstd) → Encrypt (AES-256-GCM) → Pack → Upload
```

1. **Scan** — Walk the filesystem, comparing modification times against the SQLite cache to skip unchanged files.
2. **Chunk** — Split file contents into variable-size blocks (256 KiB–4 MiB) using FastCDC content-defined chunking. Identical content produces identical chunks regardless of file name or location.
3. **Deduplicate** — Each chunk is hashed with HMAC-SHA256. If the hash already exists in the index, the chunk is skipped.
4. **Compress** — New chunks are compressed with Zstd.
5. **Encrypt** — Compressed chunks are encrypted with AES-256-GCM using fresh random nonces.
6. **Pack** — Encrypted blobs are grouped into pack files (~128 MiB each) for efficient storage. Each pack contains a header listing all blobs inside it.
7. **Snapshot** — A snapshot file records the timestamp, hostname, paths, and a pointer to the root directory tree.

### Content-Defined Deduplication

HitStor uses content-defined chunking to identify and eliminate duplicate data across files and across backup runs. This section explains how it works in detail.

#### Chunking Algorithm: FastCDC with Gear Hash

Instead of splitting files at fixed offsets (every 1 MiB), HitStor uses **FastCDC** (Fast Content-Defined Chunking) to find split points based on the file's actual content. This is critical because fixed-size chunks break badly when data is inserted or deleted — every chunk after the change shifts, and none of them match the previous backup.

FastCDC works by sliding a **Gear rolling hash** over the file's bytes. At each position, the hash is updated with a single operation:

```
fingerprint = (fingerprint << 1) + gear_table[byte]
```

The `gear_table` is a fixed lookup table of 256 random 64-bit values, generated from a deterministic seed (`0xFA57CDC`). This ensures that the same file content always produces the same chunk boundaries, regardless of when or where the backup runs.

**Chunk boundary detection** uses two bit masks with different strictness levels:

| File region | Mask used | Effect |
|-------------|-----------|--------|
| Between min (256 KiB) and avg (1 MiB) | maskS (more bits set) | Easier to match — produces shorter chunks |
| Between avg (1 MiB) and max (4 MiB) | maskL (fewer bits set) | Harder to match — lets chunks grow longer |

When the rolling hash ANDed with the active mask equals zero, a chunk boundary is declared. If no boundary is found by the max size (4 MiB), the chunk is cut there. This two-mask approach keeps the average chunk size close to the configured 1 MiB target.

**Why this matters in practice:**

```
Original file:    [AAAA][BBBB][CCCC][DDDD]     ← 4 chunks

Insert "XY" near the start:
  Fixed-size:     [AAXY][AABB][BBCC][CCDD][DD]  ← ALL 5 chunks are different
  FastCDC:        [AAXY][AA][BBBB][CCCC][DDDD]  ← only 2 chunks changed, 2 reused
```

With content-defined chunking, small edits only affect the chunks near the edit point. Chunks further away keep the same boundaries and the same content, so they deduplicate perfectly.

#### Blob Identification

Each chunk receives a unique ID computed as:

```
blob_id = HMAC-SHA256(IDKey, chunk_data)
```

The `IDKey` is the second 32 bytes of the 64-byte master key (the first 32 bytes are the encryption key). Using HMAC instead of plain SHA-256 means that blob IDs are repository-specific — the same file content produces different blob IDs in different repositories, preventing cross-repository information leakage.

**Deterministic property:** the same chunk content within the same repository always produces the same blob ID. This is what makes deduplication possible.

#### Three-Level Deduplication Check

During a backup, every chunk goes through three levels of deduplication before being stored:

```
Chunk computed
    │
    ▼
┌─────────────────────────────┐
│ Level 1: Session map        │  In-memory map of blob IDs added
│ (blobs added this backup)   │  during the current backup run.
└──────────┬──────────────────┘
           │ not found
           ▼
┌─────────────────────────────┐
│ Level 2: Files cache        │  SQLite database tracking file
│ (unchanged files)           │  metadata (inode, size, mtime).
└──────────┬──────────────────┘  If file is unchanged, reuse
           │ not found           cached chunk IDs without
           ▼                     re-reading the file at all.
┌─────────────────────────────┐
│ Level 3: Repository index   │  All blob IDs from all previous
│ (all previous backups)      │  backups, loaded into memory
└──────────┬──────────────────┘  when the repo is opened.
           │ not found
           ▼
      NEW BLOB → compress → encrypt → add to pack
```

If the blob ID is found at **any** level, the chunk is skipped entirely — no compression, no encryption, no storage. Only its ID is recorded as a reference in the snapshot tree.

#### New Blob Processing

When a chunk is genuinely new (not found at any dedup level):

1. **Compress** — Zstd compression reduces the chunk size (typical ratio: 40-60% for text, minimal for already-compressed data).
2. **Encrypt** — AES-256-GCM with a fresh random 12-byte nonce per blob.
3. **Pack** — The encrypted blob is appended to the current pack buffer. When the buffer reaches 128 MiB, the pack is finalized (header encrypted and appended), saved to the `data/` directory, and its blob mappings are recorded in the index.

#### Cross-File Deduplication

Files don't need to be identical for deduplication to work. Any shared content between files produces matching chunks:

```
                    ┌──────────────────────────────────┐
                    │         Repository index          │
                    ├──────────┬──────────┬─────────────┤
                    │ Chunk A  │ Chunk B  │  Chunk C    │
                    │ (stored) │ (stored) │  (stored)   │
                    └────┬─────┴────┬─────┴──────┬──────┘
                         │          │            │
  Backup 1:              │          │            │
    server.log    →  [Chunk A] [Chunk B]         │
    server.log.1  →  [Chunk A] [Chunk C]         │
                     ▲ same content = same ID,   │
                       stored only once           │
                                                  │
  Backup 2 (next day):                           │
    server.log    →  [Chunk A] [Chunk B] [Chunk D]
                     ▲ found    ▲ found   ▲ NEW
                     in index   in index   → stored
```

In this example:
- **Backup 1** stores 3 unique chunks (A, B, C). Chunk A appears in both files but is stored once.
- **Backup 2** only stores 1 new chunk (D). Chunks A and B are found in the repository index from backup 1 and skipped.

#### Incremental Backup Optimization

Beyond chunk-level deduplication, HitStor performs server-side incremental detection. When a backup session begins, the server sends the previous snapshot's file metadata (path, mtime, size, content IDs) to the agent. The agent compares each file's current mtime and size against the previous values — if both match, the file's cached content IDs are reused without reading the file from disk at all.

This approach requires no local state on the agent. It works across reinstalls, on new machines, and when multiple agents back up to the same repository. The first backup is always a full scan; subsequent backups are incremental by default.

This means a typical incremental backup of a 1 TB dataset where only 10 GB changed will:
- **Skip** ~990 GB of unchanged files entirely (metadata match from previous snapshot)
- **Re-chunk** ~10 GB of changed files
- **Store** only the new chunks that don't match any existing blob ID

To guard against silent file corruption, you can configure a periodic full re-scan interval (e.g., every 7 days) that forces the agent to read every file regardless of metadata. You can also trigger an immediate full re-scan from the dashboard.

### Storage Limits

There is no artificial size limit in HitStor. The repository grows until the underlying disk is full.

The main practical constraint is **index memory usage**. When a repository is opened, the entire blob index is loaded into RAM. Each index entry (`PackedBlob`) holds a 32-byte blob ID, blob type, offset, length, and uncompressed length — roughly **120 bytes per entry** in memory.

Since chunks average ~1 MiB (FastCDC `avg_size`), you can estimate index memory from the amount of unique data stored:

| Unique data stored | Chunks (approx.) | Index RAM usage | Notes |
|--------------------|-------------------|-----------------|-------|
| 100 GB | ~100,000 | ~12 MB | Laptop, small server |
| 1 TB | ~1,000,000 | ~120 MB | Typical server |
| 10 TB | ~10,000,000 | ~1.2 GB | Large server, needs adequate RAM |
| 100 TB | ~100,000,000 | ~12 GB | Very large; requires significant RAM |

**"Unique data stored"** means data after deduplication. If you back up 1 TB daily and only 1% changes between runs, after 30 snapshots the repo holds ~1.3 TB of unique data, not 30 TB. Deduplication keeps the index much smaller than the sum of all snapshots.

Pack files are capped at ~128 MiB each. A 1 TB repo contains roughly 8,000 pack files, distributed across 256 fan-out subdirectories (`data/00/` through `data/ff/`). This structure works well on all common Linux filesystems:

| Filesystem | Max files per directory | Max file size | Typical use |
|------------|------------------------|---------------|-------------|
| ext4 | ~10 million | 16 TB | Most Linux systems |
| XFS | Billions | 8 EB | Large storage servers |
| Btrfs | ~2^64 | 16 EB | Copy-on-write systems |

For typical use cases (laptops, workstations, servers with up to a few TB of data), HitStor runs comfortably. The design mirrors tools like restic, which handle multi-TB repositories in production.

### Environment Variables

| Variable | Description |
|----------|-------------|
| `HITSTOR_REPO` | Repository location (same as `--repo`) |
| `HITSTOR_PASSWORD` | Repository password (same as `--password`) |
| `NO_COLOR` | Disable colored output when set to any value |

Environment variables override config file values but are overridden by CLI flags.

---

## Color Output

HitStor uses ANSI colors on interactive terminals:
- **Green** for success messages
- **Yellow** for warnings
- **Red** for errors
- **Cyan** for IDs and metadata

Colors are automatically disabled when output is piped or redirected. To explicitly disable colors, set the `NO_COLOR` environment variable:

```bash
NO_COLOR=1 hitstor snapshots --repo /path/to/repo --password "secret"
```

---

## Shell Completion

```bash
# Bash
hitstor completion bash > /etc/bash_completion.d/hitstor

# Zsh
hitstor completion zsh > "${fpath[1]}/_hitstor"

# Fish
hitstor completion fish > ~/.config/fish/completions/hitstor.fish
```

Snapshot IDs are dynamically completed when Tab is pressed for `restore`, `forget`, and `diff` commands.

---

## Configuration

HitStor reads configuration from a YAML file, environment variables, and CLI flags.
Precedence (highest to lowest):

1. **CLI flags** (`--repo`, `--password`, etc.)
2. **Environment variables** (`HITSTOR_REPO`, `HITSTOR_PASSWORD`)
3. **Config file** (`~/.config/hitstor/config.yaml`)
4. **Defaults**

Use `--config` to specify a custom config file path.

### Configuration Management

```bash
# View all config values and their sources
hitstor config show
hitstor config show --json

# Modern multi-repo management
hitstor repo list
hitstor repo create myrepo /path/to/repo --password "secret"
hitstor --repo-name myrepo snapshots

# Scheduled jobs (dashboard parity)
hitstor jobs list
hitstor jobs create --name daily-backup --repo-name myrepo --agent-name linux-agent --interval-hours 24

# Set a value
hitstor config set repo /backups/hitstor
hitstor config set parallel 4

# Unset a value (revert to default)
hitstor config unset parallel

# Manage list values (e.g., exclude patterns)
hitstor config add exclude "*.tmp"
hitstor config add exclude "*.log"
hitstor config remove exclude "*.tmp"
hitstor config set exclude "*.tmp,*.log,*.cache"  # replace entire list

# Interactive setup wizard
hitstor config init

# Open config file in your editor
hitstor config edit

# Show config file path
hitstor config path
```

All config subcommands support `--json` for machine-readable output.
The `config` command can also be invoked as `cfg` for convenience.

### Interactive Setup Wizard

Run `hitstor config init` for a guided walkthrough. Press Enter at any prompt to accept the default shown in `[brackets]`. Ctrl+C cancels without writing anything.

#### Example: Local Backend

```
$ hitstor config init
 _   _ _ _   ____  _
| | | (_) |_/ ___|| |_ ___  _ __
| |_| | | __\___ \| __/ _ \| '__|
|  _  | | |_ ___) | || (_) | |
|_| |_|_|\__|____/ \__\___/|_|   v0.1.0

Welcome to HitStor configuration setup!

Backend type (local/s3) [local]: local

Repository path: /backups/myserver
Password file (optional): /etc/hitstor/password
Parallel workers (0 = auto) [0]: 4
Exclude patterns (comma-separated, optional): *.tmp,*.cache,*.log

Configuration saved to /home/user/.config/hitstor/config.yaml
```

#### Example: S3 Backend

```
$ hitstor config init
 _   _ _ _   ____  _
| | | (_) |_/ ___|| |_ ___  _ __
| |_| | | __\___ \| __/ _ \| '__|
|  _  | | |_ ___) | || (_) | |
|_| |_|_|\__|____/ \__\___/|_|   v0.1.0

Welcome to HitStor configuration setup!

Backend type (local/s3) [local]: s3

Repository (s3:endpoint/bucket): s3:s3.amazonaws.com/my-backup-bucket
S3 access key: AKIAIOSFODNN7EXAMPLE
S3 secret key: wJalrXUtnFEMI/K7MDENG
S3 region [us-east-1]: eu-west-1
Password file (optional): /etc/hitstor/password
Parallel workers (0 = auto) [0]:
Exclude patterns (comma-separated, optional): *.tmp,*.cache

Configuration saved to /home/user/.config/hitstor/config.yaml
```

#### Re-running on Existing Config

If a config file already exists, the wizard shows current values as defaults and asks before overwriting:

```
$ hitstor config init
...
Config file already exists at /home/user/.config/hitstor/config.yaml
Overwrite? (y/N): y

Backend type (local/s3) [local]: local

Repository path [/backups/myserver]: /backups/newserver
Password file (optional) [/etc/hitstor/password]:
Parallel workers (0 = auto) [4]: 8
Exclude patterns (comma-separated, optional) [*.tmp,*.cache,*.log]:

Configuration saved to /home/user/.config/hitstor/config.yaml
```

#### Verify Your Configuration

After running the wizard, verify with `config show`:

```
$ hitstor config show
KEY               VALUE                     SOURCE
────────────────  ────────────────────────  ───────
repo              /backups/myserver         config
password_file     /etc/hitstor/password     config
s3_access_key     (not set)                 default
s3_secret_key     (not set)                 default
s3_region         us-east-1                 default
exclude           *.tmp, *.cache, *.log     config
parallel          4                         config
```

### Configurable Keys

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `repo` | string | | Repository location (path or s3:endpoint/bucket) |
| `password_file` | string | | Path to password file |
| `s3_access_key` | string | | S3 access key (masked in `show`) |
| `s3_secret_key` | string | | S3 secret key (masked in `show`) |
| `s3_region` | string | us-east-1 | S3 region |
| `exclude` | list | | Exclude patterns |
| `parallel` | int | 0 (auto) | Number of parallel workers |
| `serve_port` | int | 8420 | gRPC server listen port |
| `serve_tls_cert` | string | | Path to TLS certificate file for server |
| `serve_tls_key` | string | | Path to TLS private key file for server |
| `agent_server` | string | | Remote server address (host:port) |
| `agent_api_key` | string | | Agent API key (masked in `show`) |
| `agent_ca_cert` | string | | Path to CA certificate for TLS |
| `agent_hostname` | string | | Override hostname sent to server |
| `agent_paths` | list | | Default backup paths |
| `agent_excludes` | list | | Default exclude patterns for agent |
| `plugin_dir` | string | | Directory to search for plugin binaries |
| `hook_plugins` | list | | Hook plugin names to load for pre/post-backup lifecycle events |
| `fs_scanner_plugin` | string | | Filesystem scanner plugin name (`builtin`, `db`, etc.) |
| `serve_advertise_addr` | string | | Address agents use to reach the server (NAT/public IP) |
| `update_check` | bool | true | Check GitHub for new releases |
| `update_url` | string | | Override release check URL |

### Example Config File

```yaml
repo: /backups/hitstor
password_file: /etc/hitstor/password
parallel: 4
exclude:
  - "*.tmp"
  - "*.cache"

# Remote backup server (optional)
serve_port: 8420
serve_tls_cert: /etc/ssl/hitstor/server.crt
serve_tls_key: /etc/ssl/hitstor/server.key

# Remote backup agent (optional)
agent_server: backup-server.internal:8420
agent_api_key: abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789
agent_ca_cert: /etc/ssl/hitstor/ca.pem
agent_hostname: my-server
agent_excludes:
  - "*.log"
  - "*.cache"
```

---

## Remote Backup Server

HitStor includes a gRPC server that accepts backups from remote agents over TLS. This enables centralized backup of multiple Linux and Windows machines to a single repository.

### Architecture

The system uses a **thick agent** design:

```
Agent (remote machine)                    Server (hitstor serve)
┌─────────────────────┐                  ┌──────────────────────────┐
│ Scan filesystem     │                  │ Verify API key           │
│ Chunk (FastCDC)     │    gRPC/TLS      │ Check dedup (repo index) │
│ Compute blob IDs    │ ──────────────►  │ Encrypt (AES-256-GCM)    │
│ Compress (Zstd)     │                  │ Pack into 128 MiB packs  │
│ Send new chunks     │                  │ Build trees & snapshot   │
└─────────────────────┘                  │ Save index               │
                                         └──────────────────────────┘
```

- The **agent** handles filesystem scanning, chunking, blob ID computation, and compression locally.
- Only **new compressed chunks** are sent over the network — chunks already in the repository are skipped.
- The **server** handles dedup verification, encryption, pack assembly, index management, and snapshot creation.
- The **encryption key never leaves the server** — agents receive only the IDKey for blob identification.

### Starting the Server

```bash
# Start with auto-generated self-signed TLS certificate
hitstor serve --repo /path/to/repo --password "secret"

# Start with custom TLS certificate
hitstor serve --repo /path/to/repo --password "secret" \
  --tls-cert /etc/ssl/server.crt \
  --tls-key /etc/ssl/server.key

# Custom port (default: 8420)
hitstor serve --repo /path/to/repo --password "secret" --port 9000
```

The server acquires an exclusive lock on the repository and listens for agent connections.

### Managing Agents

Each remote machine needs a registered API key to authenticate with the server.

```bash
# Register a new agent (displays the API key once)
hitstor serve agent add myserver --repo /path/to/repo --password "secret"
# Output:
#   Agent "myserver" registered.
#   API Key: a1b2c3d4...64-hex-chars
#   Save this key — it cannot be displayed again.

# List all registered agents
hitstor serve agent list --repo /path/to/repo --password "secret"
# Output:
#   NAME                  CREATED               LAST BACKUP
#   myserver              2026-02-20 10:30:00   never

# Remove an agent (revokes its API key)
hitstor serve agent remove myserver --repo /path/to/repo --password "secret"
```

Agent records are stored encrypted in the repository directory (`agents.json`), protected by the repository's encryption key.

### gRPC Protocol

The server exposes 5 RPCs over TLS:

| RPC | Type | Description |
|-----|------|-------------|
| `Ping` | Unary | Health check — returns server version and repository ID |
| `BeginSession` | Unary | Authenticate with API key, receive IDKey, chunker params, and previous snapshot metadata for incremental detection |
| `CheckBlobs` | Unary | Batch check which blob IDs the server already has (dedup) |
| `UploadChunks` | Client-streaming | Stream compressed chunks to the server |
| `FinalizeBackup` | Unary | Send file tree, server builds snapshot and returns summary |
| `GetSchedule` | Unary | Get the agent's configured backup schedule and next run time |

### Backup Flow

1. **BeginSession** — Agent authenticates with its API key. Server returns the IDKey (for blob ID computation), chunker parameters, and previous snapshot file metadata (paths, mtimes, sizes, content IDs) for incremental detection.
2. **Scan & Chunk** — Agent scans its filesystem, skipping files whose mtime and size match the previous snapshot metadata. Changed and new files are chunked with FastCDC, computing `blob_id = HMAC-SHA256(IDKey, chunk_data)`.
3. **CheckBlobs** — Agent sends batch of blob IDs to check. Server returns only the IDs not in the repository (needed uploads).
4. **UploadChunks** — Agent compresses new chunks with Zstd and streams them to the server. Server encrypts each chunk and packs them.
5. **FinalizeBackup** — Agent sends the complete file/directory tree. Server builds tree blobs, creates a snapshot, saves the index, and returns a summary.

---

## Remote Backup Agent

The `hitstor-agent` binary runs on remote machines and connects to a HitStor server to perform backups.

### Installation

**Using the interactive installer** (recommended for Linux):

```bash
sudo ./hitstor-agent-X.Y.Z-linux-amd64.run
```

The wizard configures server address, API key, backup paths, and starts the service. See [Linux Quick Install](docs/installation/linux-installer.md) for details.

**Build from source:**

```bash
make agent                    # Linux amd64
make agent-arm64              # Linux arm64
make agent-windows            # Windows amd64
make agent-windows-arm64      # Windows arm64
make agent-all                # All platforms

# Install (Linux)
sudo cp bin/hitstor-agent /usr/local/bin/
```

For Windows setup (installation, Windows Service, known limitations), see [docs/installation/windows-agent.md](docs/installation/windows-agent.md).

### Quick Start

```bash
# Test connectivity
hitstor-agent ping --server backup-server:8420 --ca-cert /path/to/server.crt

# Run a backup
hitstor-agent backup /etc /home /var/lib \
  --server backup-server:8420 \
  --api-key <64-char-hex-key> \
  --ca-cert /path/to/server.crt

# With excludes and tags
hitstor-agent backup /home \
  --server backup-server:8420 \
  --api-key <key> \
  --exclude "*.log" --exclude "*.cache" \
  --tag production --tag daily
```

### Configuration

Instead of passing flags every time, configure the agent via the config file or environment variables:

```yaml
# ~/.config/hitstor/config.yaml
agent_server: backup-server.internal:8420
agent_api_key: abcdef0123456789...
agent_ca_cert: /etc/ssl/hitstor/ca.pem
agent_hostname: my-server
agent_excludes:
  - "*.log"
  - "*.cache"
```

Environment variables: `HITSTOR_AGENT_SERVER`, `HITSTOR_AGENT_API_KEY`.

### Systemd Scheduled Backups

Template unit files are provided in `docs/`:

```bash
# Install the service and timer
sudo cp packaging/systemd/hitstor-agent.service /etc/systemd/system/
sudo cp packaging/systemd/hitstor-agent.timer /etc/systemd/system/

# Edit the service to set your server address, API key, and paths
sudo systemctl edit hitstor-agent.service

# Enable the daily timer
sudo systemctl enable --now hitstor-agent.timer

# Check backup status
sudo systemctl status hitstor-agent.service
sudo journalctl -u hitstor-agent.service
```

### Agent Configuration Keys

| Key | Type | Description |
|-----|------|-------------|
| `agent_server` | String | Server address (host:port) |
| `agent_api_key` | String | Agent API key (64-char hex) |
| `agent_ca_cert` | String | Path to CA certificate for TLS |
| `agent_hostname` | String | Override hostname in snapshots |
| `agent_paths` | List | Default backup paths |
| `agent_excludes` | List | Default exclude patterns |

---

## Web Dashboard

The `hitstor serve` command starts an HTTP dashboard alongside the gRPC server. It provides a browser-based UI for monitoring agents, browsing snapshots, and viewing storage usage.

> UI uses the HitStor brand system — navy surfaces, orange accents, padlock isotipo. See [docs/design/branding.md](docs/design/branding.md).

```
hitstor serve
  ├─ gRPC :8420 ── agents (backup traffic)
  └─ HTTP :8421 ── browser (dashboard)
```

### Access

Open `http://server-address:8421` in a browser. The dashboard auto-refreshes via htmx polling (every 30 seconds).

### Pages

| Page | URL | Description |
|------|-----|-------------|
| Overview | `/` | Agent count, snapshot count, last backup time, active backup count |
| Backups | `/backups` | Filesystem backup jobs |
| Agents | `/agents` | Agent list with discovery, status badges (OK / stale / never) |
| Agent Detail | `/agents/{name}` | Schedule, paths, backup history |
| Snapshot Detail | `/snapshots/{id}` | Snapshot metadata, file tree browser, restore with progress |
| Storage | `/storage` | Repository management, snapshots, check, prune, growth charts |
| Restores | `/restores` | Active and completed restore sessions |
| Settings | `/settings` | TLS, network, integrations, notifications |

### JSON API

| Endpoint | Description |
|----------|-------------|
| `GET /api/agents` | Agent list with status, snapshot counts, and schedule info |
| `POST /api/agents/{name}/schedule` | Set agent backup schedule (JSON: enabled, interval_hours) |
| `POST /api/agents/{name}/repo` | Set agent target repository |
| `GET /api/snapshots?host=&limit=` | Snapshots (newest first, default limit 50) |
| `GET /api/stats` | Repository stats (snapshots, packs, indexes, total size) |
| `GET /api/repos` | List configured repositories with stats |
| `POST /api/repos` | Create a new repository (local or S3) |
| `DELETE /api/repos/{name}` | Disconnect a repository |
| `POST /api/repos/{name}/check` | Start integrity check (returns operation ID) |
| `POST /api/repos/{name}/prune` | Start prune operation (returns operation ID) |
| `GET /api/repos/{name}/growth` | Storage growth data for charting |
| `POST /api/snapshots/{id}/restore` | Start restore (JSON: agent_name, target_path, includes) |
| `GET /api/snapshots/{id}/tree?path=` | Browse snapshot file tree (lazy-load directories) |
| `GET /api/restore/{id}/events` | SSE stream for restore progress |
| `GET /api/restores` | List all restore sessions (active and completed) |
| `GET /api/operations/{id}/events` | SSE stream for operation progress |
| `GET /api/active-backups` | Count and details of all active backup sessions |
| `GET /api/backup-progress/events` | SSE stream of backup progress (all sessions, every 2s) |
| `GET /metrics` | Prometheus-compatible metrics (unauthenticated) |

### Prometheus Metrics

The `/metrics` endpoint exposes server metrics in Prometheus text exposition format. It is always unauthenticated, allowing Prometheus to scrape without credentials.

Exposed metrics include:
- **Gauges**: `hitstor_snapshots_total`, `hitstor_packs_total`, `hitstor_indexes_total`, `hitstor_repository_size_bytes`, `hitstor_active_sessions`, `hitstor_registered_agents`, `hitstor_agents_ok`, `hitstor_agents_stale`, `hitstor_agents_never`
- **Counters**: `hitstor_backups_completed_total`, `hitstor_backups_failed_total`, `hitstor_bytes_received_total`, `hitstor_chunks_received_total`, `hitstor_checks_total`, `hitstor_check_errors_total`, `hitstor_prunes_total`, `hitstor_prune_freed_bytes_total`
- **Per-agent**: `hitstor_agent_last_backup_timestamp_seconds{agent="name"}`, `hitstor_agent_schedule_enabled{agent="name"}`, `hitstor_agent_schedule_interval_hours{agent="name"}`

### Alerting

HitStor can send notifications when backups fail or agents go stale. Notifications are delivered via the built-in webhook notifier and/or notifier plugins.

```bash
# Built-in webhook notifier
hitstor config set alert_webhook_url "https://hooks.slack.com/services/..."
hitstor config set alert_stale_threshold 25  # hours (default)

# Notifier plugins (load alongside built-in webhook)
hitstor config add alert_notifier_plugins slack
hitstor config set plugin_dir /usr/local/lib/hitstor/plugins
```

Events are sent as JSON POST requests:

| Event | Trigger |
|-------|---------|
| `backup_failed` | Agent disconnects during upload or finalization fails |
| `agent_stale` | No backup within the configured threshold (checked every 5 min) |

Agents that have never backed up do not trigger stale alerts. Stale alerts include cooldown to avoid repeated notifications — an agent is re-alerted only after the threshold duration passes.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `alert_webhook_url` | String | *(empty)* | Webhook URL for alert notifications (empty = disabled) |
| `alert_stale_threshold` | Int | `25` | Hours before an agent is considered stale |
| `alert_notifier_plugins` | List | *(empty)* | Notifier plugin names to load (comma-separated) |

### Scheduling

HitStor supports server-managed backup scheduling. Configure a schedule per agent via the dashboard, then run the agent in daemon mode to poll the server and execute backups automatically.

#### Enabling a Schedule

1. Open the dashboard and navigate to a backup job's detail page
2. Check "Enable scheduled backups"
3. Choose a schedule mode:
   - **Interval**: Simple "every X hours" repetition
   - **Schedule**: Visual builder with Daily, Weekly, or Monthly presets — select time, days of week, or day of month using dropdowns and toggle buttons (no cron syntax needed)
   - **Advanced**: Raw cron expression input with live validation and next-run preview
4. Click "Save Schedule"

The visual schedule builder automatically detects existing cron expressions and displays them in the appropriate mode. Complex expressions that don't match a visual pattern fall back to Advanced mode.

#### Agent Daemon Mode

Instead of relying on systemd timers, run the agent as a long-lived daemon that polls the server for its schedule:

```bash
# Run in daemon mode (polls server every 5 minutes by default)
hitstor-agent daemon --server backup-server:8420 --api-key <key>

# Custom poll interval
hitstor-agent daemon --server backup-server:8420 --api-key <key> --poll-interval 2m
```

The daemon:
- Polls the server for the configured schedule every 5 minutes (configurable via `--poll-interval`)
- Runs a backup when the scheduled time arrives (`next_run = last_backup + interval_hours`)
- If the agent has never backed up and a schedule is enabled, the first backup runs immediately
- Uses `agent_paths` and `agent_excludes` from the config file for backup paths
- On Windows, the Windows Service automatically runs in daemon mode

Existing agents using systemd timers or Windows Task Scheduler continue to work unchanged. Daemon mode is opt-in.

### Authentication

By default the dashboard is open (no login required). To enable password protection:

```bash
hitstor config set serve_dashboard_password "your-secret"
```

When set, all pages and API endpoints require authentication via session cookie. The login page appears at `/login`.

### Dashboard Configuration Keys

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `serve_dashboard_port` | Int | `8421` | HTTP dashboard listen port |
| `serve_dashboard_password` | String | *(empty)* | Dashboard login password (empty = open access) |

### Flags

```bash
hitstor serve --dashboard-port 9090   # override port
```

---

## Plugin Backends

HitStor supports storage backends as external plugins using the [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin) framework. Plugins are standalone binaries that communicate with hitstor over gRPC.

### Usage

```bash
# Initialize a repository using a plugin backend
hitstor init --repo plugin:azure/myaccount/mycontainer --password "secret"

# Backup using a plugin backend
hitstor backup --repo plugin:azure/myaccount/mycontainer --password "secret" /data
```

The `plugin:<name>[/<uri>]` URI scheme maps to a binary named `hitstor-backend-<name>`.

### Plugin Discovery

1. Search `plugin_dir` config path (if configured)
2. Search `$PATH`

### Built-in Example: Memory Backend

An in-memory plugin is included for testing:

```bash
make plugin-memory   # builds bin/hitstor-backend-memory
```

### Writing Plugins

A plugin binary implements the `StorageBackend` gRPC service defined in [`api/proto/hitstor/plugin/v1/storage.proto`](api/proto/hitstor/plugin/v1/storage.proto). The plugin's `main()` calls `plugin.ServeStorageBackend(impl)` with a `backend.Backend` implementation. See [`cmd/hitstor-backend-memory/`](cmd/hitstor-backend-memory/) for a complete example.

### Notifier Plugins

Notifier plugins deliver alert notifications via custom channels (Slack, PagerDuty, email, etc.). They are standalone binaries named `hitstor-notifier-<name>` implementing the `Notifier` gRPC service defined in [`api/proto/hitstor/plugin/v1/notifier.proto`](api/proto/hitstor/plugin/v1/notifier.proto).

```bash
# Configure notifier plugins
hitstor config add alert_notifier_plugins slack
hitstor config set plugin_dir /usr/local/lib/hitstor/plugins
```

A notifier plugin's `main()` calls `plugin.ServeNotifier(impl)` with a `notifier.Notifier` implementation. See [`cmd/hitstor-notifier-log/`](cmd/hitstor-notifier-log/) for a complete example.

### Hook Plugins

Hook plugins run pre/post-backup lifecycle events. They are standalone binaries named `hitstor-hook-<name>` implementing the `Hook` gRPC service defined in [`api/proto/hitstor/plugin/v1/hook.proto`](api/proto/hitstor/plugin/v1/hook.proto).

```bash
# Configure hook plugins
hitstor config add hook_plugins log
hitstor config set plugin_dir /usr/local/lib/hitstor/plugins
```

Hooks fire at two levels:
- **Server-side** (`hitstor serve`): `PreBackup` fires after agent auth in `BeginSession`; `PostBackup` fires after snapshot saved in `FinalizeBackup`.
- **Local backup** (`hitstor backup`): `PreBackup` fires before the backup starts; `PostBackup` fires after a successful backup.

`PreBackup` errors abort the backup. `PostBackup` errors are logged but do not fail the backup.

A hook plugin's `main()` calls `plugin.ServeHook(impl)` with a `hook.Hook` implementation. See [`cmd/hitstor-hook-log/`](cmd/hitstor-hook-log/) for a complete example.

### FilesystemScanner Plugins

FilesystemScanner plugins replace the built-in directory walker with custom scanning logic. They are standalone binaries named `hitstor-scanner-<name>` implementing the `FilesystemScanner` gRPC service defined in [`api/proto/hitstor/plugin/v1/scanner.proto`](api/proto/hitstor/plugin/v1/scanner.proto).

```bash
# Configure a scanner plugin (only one scanner active at a time)
hitstor config set fs_scanner_plugin builtin
hitstor config set plugin_dir /usr/local/lib/hitstor/plugins
```

Unlike notifier and hook plugins (which support multiple), only **one** scanner plugin can be active at a time. When set, it replaces the default filesystem walker for both local backups (`hitstor backup`) and agent backups (`hitstor-agent backup`).

The `Scan` RPC uses server-side streaming to return file entries one at a time, making it efficient for large directory trees.

A scanner plugin's `main()` calls `plugin.ServeFilesystemScanner(impl)` with a `scanner.Scanner` implementation. See [`cmd/hitstor-scanner-builtin/`](cmd/hitstor-scanner-builtin/) for a complete example.

For detailed plugin development documentation, see [`docs/reference/plugin-development.md`](docs/reference/plugin-development.md).

### Removed: agentless workloads

Microsoft 365, Google Workspace, database, and cloud-VM (including VMware/vSphere)
backups were permanently removed in v2.0.0. HitStor is a filesystem backup and
restore product for Windows and Linux. See
[docs/admin-guide/roadmap.md](docs/admin-guide/roadmap.md).

---

## Performance benchmarks

Reproducible backup/restore timings (optional restic comparison):

```bash
./benchmarks/run.sh --profile small
make bench PROFILE=medium
```

See [benchmarks/README.md](benchmarks/README.md) and [docs/dev-guide/benchmarks.md](docs/dev-guide/benchmarks.md).

## Development

### Prerequisites

- Go 1.26+ (see `go.mod`)
- Docker and Docker Compose (for S3/MinIO integration tests)

### Build and Test

```bash
# Build
go build ./cmd/...

# Run all tests
go test ./...

# Vet
go vet ./...

# Start MinIO for S3 integration tests
docker compose up -d
```

### Project Conventions

- **Test-first** — TDD with table-driven tests.
- **Error wrapping** — `fmt.Errorf("package: action: %w", err)`.
- **Structured logging** — `log/slog` with JSON output in production.
- **No `utils/` or `helpers/`** — Functions live in their domain package.

---

## License

Copyright (C) 2026 HooperITS

HitStor is free software, licensed under the [GNU Affero General Public License v3.0](LICENSE) (AGPL-3.0-only).
If you modify HitStor and make it available to users over a network (for example, the web dashboard),
you must offer them the corresponding source code of your modified version.

---

<div align="center">

<sub>Brand assets: <code>assets/icon.png</code> · <a href="docs/design/branding.md">design system</a></sub>

</div>
