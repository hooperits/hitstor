# hitstor-agent backup

Send a backup to the remote server.

## Synopsis

```
hitstor-agent backup [flags] [paths...]
```

## Description

Scans the given paths, splits files into content-defined chunks (FastCDC), compresses each chunk with Zstd, and uploads new chunks to the remote HitStor server over gRPC/TLS. The server handles deduplication, encryption, and storage.

At least one path argument is required (or configured via `agent_paths`).

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--exclude` | string | | Exclude files matching this glob pattern (repeatable) |

Plus all agent [global flags](index.md#global-flags).

## Examples

Back up specific directories:

```bash
hitstor-agent backup --server backup.internal:8420 \
  --api-key YOUR_KEY /etc /home /var/lib
```

Using environment variables:

```bash
export HITSTOR_AGENT_SERVER=backup.internal:8420
export HITSTOR_AGENT_API_KEY=YOUR_KEY
hitstor-agent backup /etc /home
```

Using config file paths:

```bash
hitstor-agent backup
```
