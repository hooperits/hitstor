# hitstor restore

Restore files from a snapshot.

## Synopsis

```
hitstor restore [flags] <snapshot-id> <target-path>
```

## Description

Restores all files from the specified snapshot to the target directory. The snapshot can be identified by its full 64-character hex ID, a unique short prefix, or the keyword `latest` (which resolves to the most recent snapshot by timestamp).

Both arguments are required: the snapshot ID and the target path where files will be written.

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--include` | string | | Restore only files matching this path (repeatable) |

Plus all [global flags](index.md#global-flags), including `--repo-name` for multi-repo setups.

## Snapshot ID Resolution

HitStor supports three ways to identify a snapshot:

- **Full ID** — the complete 64-character hex string
- **Short prefix** — a unique prefix of the snapshot ID (e.g., `a1b2c3d4`). If the prefix is ambiguous (matches multiple snapshots), an error is returned.
- **`latest`** — resolves to the snapshot with the most recent timestamp

## Examples

Restore the latest snapshot:

```bash
hitstor restore --repo /backups/hitstor --password "secret" latest /tmp/restore
```

Restore a specific snapshot by short ID:

```bash
hitstor restore --repo /backups/hitstor --password "secret" a1b2c3d4 /tmp/restore
```

Restore only specific files:

```bash
hitstor restore --repo /backups/hitstor --password "secret" latest /tmp/restore \
  --include "/home/user/documents/important.pdf"
```
