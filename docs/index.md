# HitStor

Encrypted, deduplicated backup tool written in Go.

## Overview

HitStor provides secure, efficient backups with client-side encryption, content-defined deduplication, and compression. It supports local and S3-compatible storage backends, remote backup agents over gRPC/TLS, a web dashboard, and an extensible plugin architecture.

## Key Features

- **Client-side encryption** — AES-256-GCM with scrypt key derivation
- **Content-defined deduplication** — FastCDC chunking (256 KiB–4 MiB)
- **Compression** — Zstd compression per chunk before encryption
- **Remote backup server** — gRPC server with TLS and API key authentication
- **Web dashboard** — Backup jobs, agents, storage, restore, DR workflows
- **Bare-metal restore** — ReaR-based recovery media for Linux hosts
- **Plugin architecture** — Storage, notifiers, hooks, filesystem scanners (go-plugin gRPC)
- **Parallel I/O** — Worker pools for faster backup and restore
- **Single static binary** — Zero-dependency deployment with `CGO_ENABLED=0`

## Quick Start

```bash
# Install (Linux deb)
sudo dpkg -i hitstor_0.1.0_amd64.deb

# Initialize a repository
hitstor init --repo /var/lib/hitstor/repo --password "secret"

# Create a backup
hitstor backup --repo /var/lib/hitstor/repo --password "secret" /data

# List snapshots
hitstor snapshots --repo /var/lib/hitstor/repo --password "secret"

# Restore
hitstor restore --repo /var/lib/hitstor/repo --password "secret" latest /restore
```

## Documentation Sections

| Section | Description |
|---------|-------------|
| [Installation](installation/index.md) | Install HitStor via packages, source, Docker, or Windows installers |
| [User Guide](user-guide/index.md) | Learn how to back up, restore, manage snapshots, and configure retention |
| [Administration](admin-guide/index.md) | Servers, backup jobs, agents, discovery, DR, monitoring |
| [Reference](reference/index.md) | Commands, config, gRPC API, dashboard HTTP API, internals |
| [Dev Guide](dev-guide/index.md) | Codebase overview, build, test, and release |
