# hitstor backup

Back up files to the repository.

## Synopsis

```
hitstor backup [flags] [paths...]
```

## Description

Scans the given paths, splits files into content-defined chunks (FastCDC), deduplicates against existing repository data and a local SQLite cache, compresses (Zstd), encrypts (AES-256-GCM), and writes the data to the repository as pack files. A snapshot is created recording the backed-up paths, hostname, timestamp, and tree structure.

At least one path argument is required.

An exclusive lock is acquired on the repository for the duration of the backup.

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--exclude` | string | | Exclude files matching this glob pattern (repeatable). Matched against the name and the path relative to the backup root; `**` is not supported. See [Exclude Patterns](../../user-guide/backup.md#exclude-patterns) |
| `--tag` | string | | Add a tag to the snapshot (repeatable) |
| `--host` | string | system hostname | Override the hostname stored in the snapshot |
| `--dry-run` | bool | false | Preview what would be backed up without writing data |
| `--bare-metal` | bool | false | Also capture what a bare-metal restore needs: extended attributes (SELinux labels, capabilities, ACLs) and the host's disk and boot layout. See [Bare-metal restore](../../user-guide/bare-metal-restore.md) |

Plus all [global flags](index.md#global-flags), including `--repo-name` for multi-repo setups.

### Exclude Merging

`--exclude` patterns are merged with any `exclude` patterns defined in the config file. Both sources apply during the backup.

## Examples

Back up a single directory:

```bash
hitstor backup --repo /backups/hitstor --password "secret" /home/user/documents

# Using named repo from multi-repo config
hitstor --config /etc/hitstor/config.yaml --repo-name Production backup /data
```

Back up multiple paths with exclusions:

```bash
hitstor backup --repo /backups/hitstor --password "secret" \
  /home/user/documents /home/user/photos \
  --exclude "*.tmp" --exclude ".cache"
```

Tag a backup for identification:

```bash
hitstor backup --repo /backups/hitstor --password "secret" /data \
  --tag "pre-upgrade" --tag "production"
```

## Notes

- Files that change during the backup (detected by re-checking mtime/size after reading) are skipped with a warning.
- Unchanged files are detected via the SQLite files cache and their chunks are not re-read from disk.
- Paths are resolved to absolute paths before being stored in the snapshot.
