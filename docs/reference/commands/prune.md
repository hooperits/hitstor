# hitstor prune

Remove unreferenced data from the repository.

## Synopsis

```
hitstor prune [flags]
```

## Description

Removes pack files, index entries, and blobs that are no longer referenced by any snapshot. Run this after `forget` to reclaim storage space.

Prune performs a mark-and-sweep garbage collection:

1. Scans all snapshots to determine which blobs are still referenced.
2. Identifies pack files containing only unreferenced blobs (deleted entirely) or a mix of referenced and unreferenced blobs (repacked).
3. Writes new pack files containing only the referenced blobs from mixed packs.
4. Deletes obsolete pack files and updates indexes.

An exclusive lock is acquired on the repository for the duration of the operation.

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Only show what would be removed |
| `--max-repack-size` | string | | Limit total bytes repacked (e.g., `500MiB`, `2GiB`) |

Plus all [global flags](index.md#global-flags), including `--repo-name` for multi-repo setups.

### Size Suffixes

| Suffix | Meaning |
|--------|---------|
| `B` | Bytes |
| `KiB`, `MiB`, `GiB`, `TiB` | Binary units (1024-based) |
| `KB`, `MB`, `GB`, `TB` | Decimal units (1000-based) |

If no suffix is given, the value is treated as bytes.

## Examples

Prune unreferenced data:

```bash
hitstor prune --repo /backups/hitstor --password "secret"
```

Preview what would be removed:

```bash
hitstor prune --repo /backups/hitstor --password "secret" --dry-run
```

Limit repacking to 500 MiB:

```bash
hitstor prune --repo /backups/hitstor --password "secret" --max-repack-size 500MiB
```

## Notes

- Prune is also available as part of `forget --prune`, which combines both operations.
- The operation reports the number of packs deleted, packs repacked, bytes freed, and indexes updated.
