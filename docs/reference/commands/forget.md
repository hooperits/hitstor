# hitstor forget

Remove snapshots by ID or retention policy.

## Synopsis

```
hitstor forget [flags] [snapshot-id...]
```

## Description

Removes snapshots from the repository. There are two modes of operation:

- **Explicit IDs** — pass one or more snapshot IDs (full or prefix) as arguments to remove those specific snapshots.
- **Retention policy** — use `--keep-*` flags to automatically determine which snapshots to keep and which to forget.

These two modes cannot be combined. Providing both snapshot IDs and `--keep-*` flags is an error.

Forgetting a snapshot removes only the snapshot metadata. The actual data packs remain on disk until [prune](prune.md) is run. Use `--prune` to automatically run prune after forgetting.

An exclusive lock is acquired on the repository for the duration of the operation.

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--keep-last` | int | 0 | Keep the N most recent snapshots |
| `--keep-hourly` | int | 0 | Keep N hourly snapshots |
| `--keep-daily` | int | 0 | Keep N daily snapshots |
| `--keep-weekly` | int | 0 | Keep N weekly snapshots |
| `--keep-monthly` | int | 0 | Keep N monthly snapshots |
| `--keep-yearly` | int | 0 | Keep N yearly snapshots |
| `--keep-tag` | string | | Keep snapshots with this tag (repeatable) |
| `--host` | string | | Only apply policy to snapshots from this host |
| `--path` | string | | Only apply policy to snapshots with this path |
| `--dry-run` | bool | false | Only show what would be removed |
| `--prune` | bool | false | Run prune after forgetting to reclaim space |

Plus all [global flags](index.md#global-flags), including `--repo-name` for multi-repo setups.

## Retention Semantics

Retention flags use **union semantics**: a snapshot is kept if it matches **any** of the active `--keep-*` rules. For example, with `--keep-last 3 --keep-daily 7`, a snapshot is kept if it is among the 3 most recent **or** among the 7 most recent daily snapshots.

The `--host` and `--path` flags filter which snapshots the policy applies to. Snapshots not matching the filter are left untouched.

## Examples

Forget a specific snapshot:

```bash
hitstor forget --repo /backups/hitstor --password "secret" a1b2c3d4
```

Apply a retention policy:

```bash
hitstor forget --repo /backups/hitstor --password "secret" \
  --keep-last 5 --keep-daily 7 --keep-weekly 4 --keep-monthly 12
```

Preview what would be removed:

```bash
hitstor forget --repo /backups/hitstor --password "secret" \
  --keep-last 3 --dry-run
```

Forget and prune in one step:

```bash
hitstor forget --repo /backups/hitstor --password "secret" \
  --keep-last 5 --prune
```

## Notes

- After forgetting snapshots without `--prune`, HitStor prints a reminder to run `hitstor prune`.
- `--keep-tag` keeps any snapshot that has the specified tag, regardless of other retention rules.
