# hitstor diff

Compare two snapshots.

## Synopsis

```
hitstor diff [flags] <snapshot-id-1> <snapshot-id-2>
```

## Description

Compares two snapshots and displays the differences between them. Shows files that were added, removed, or modified between the two snapshots.

Both snapshot IDs are required and can be specified as full IDs, short prefixes, or `latest`.

Supports `--repo-name` for multi-repo (see global flags in index.md).

## Examples

Compare two snapshots:

```bash
hitstor diff --repo /backups/hitstor --password "secret" a1b2c3d4 b5c6d7e8
```

Compare a specific snapshot with the latest:

```bash
hitstor diff --repo /backups/hitstor --password "secret" a1b2c3d4 latest
```
