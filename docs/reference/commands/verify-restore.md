# hitstor verify-restore

Test-restore a snapshot and re-hash files (restore verification).

## Synopsis

```
hitstor verify-restore <snapshot-id> [flags]
```

## Description

Restores a snapshot (or a sample of its files) to a temporary directory, re-chunks each restored file with the repository chunker, and compares content blob IDs to the snapshot tree.

This is the primary trust signal that a filesystem backup is actually restorable — it round-trips real data through restore and re-chunking rather than just checking metadata.

Use `latest` as the snapshot ID to verify the most recent snapshot. Tab completion for snapshot IDs is available.

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--max-files` | int | 0 (all) | Limit the number of files verified |
| `--max-bytes` | uint64 | 0 (unlimited) | Limit total bytes verified |
| `--keep-temp` | bool | false | Keep the temporary restore directory instead of cleaning it up |

Plus all [global flags](index.md#global-flags), including `--repo-name` for multi-repo setups.

## Examples

Verify the latest snapshot in full:

```bash
hitstor verify-restore latest --repo /backups/hitstor --password "secret"
```

Sample-verify a large snapshot, capping at 500 files:

```bash
hitstor verify-restore a1b2c3d4 --max-files 500
```

Verify and keep the temp restore for manual inspection:

```bash
hitstor verify-restore latest --keep-temp
```

## Notes

- Every backup job's dashboard entry can also run this automatically after each backup — see [Guided disaster recovery](../../user-guide/disaster-recovery.md).
- A non-zero exit code means either a mismatch was found or the verification itself errored; mismatches are printed to stderr with the mismatched path and reason.
- With `--json`, output is a single JSON object (`ok`, `snapshot_id`, `files_checked`, `files_sampled`, `bytes_checked`, `symlinks_ok`, `sampled`, `duration`, `mismatches`, `error`, `restore_target`) instead of the human-readable summary.
