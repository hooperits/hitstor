# hitstor check

Verify repository integrity.

## Synopsis

```
hitstor check [flags]
```

## Description

Checks the structural integrity of the repository. By default, it verifies that all snapshots, indexes, and pack metadata are consistent and readable.

With `--read-data`, it additionally reads and decrypts every blob in every pack file, verifying that no data is corrupted. This is slower but provides a thorough end-to-end verification.

Errors are printed to stderr. The exit code is non-zero if any errors are found.

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--read-data` | bool | false | Read and verify all pack data (slower but thorough) |

Plus all [global flags](index.md#global-flags), including `--repo-name` for multi-repo setups.

## Examples

Quick structural check:

```bash
hitstor check --repo /backups/hitstor --password "secret"
```

Full verification including all blob data:

```bash
hitstor check --repo /backups/hitstor --password "secret" --read-data
```

## Notes

- Without `--read-data`, the check verifies metadata consistency (snapshots reference valid trees, indexes are well-formed, etc.) but does not read pack file contents.
- With `--read-data`, every blob is decrypted and its ID is re-verified. This catches bit-rot, truncated files, and other storage-level corruption.
