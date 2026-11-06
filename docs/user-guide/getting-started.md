# Getting Started

This tutorial walks you through the core HitStor workflow from start to finish. By the end you will have initialized a repository, created backups, listed snapshots, restored files, verified integrity, and applied a retention policy.

## Initialize a Repository

A **repository** is the directory where HitStor stores encrypted, deduplicated backup data. Before you can create any backups you must initialize one.

```console
hitstor init --repo /path/to/repo --password "secret"
created new repository 3d3f89b7 at /path/to/repo
```

The `--repo` flag points to the target directory (it will be created if it does not exist). The `--password` flag supplies the encryption passphrase that protects all data in the repository.

!!! warning
    Remember your password. Without it, the data in the repository is unrecoverable. Consider storing the password in a password manager or using `--password-file` to read it from a file.

## Create Your First Backup

Back up a directory by passing its path as the final argument:

```console
hitstor backup --repo /path/to/repo --password "secret" /data
scanning files...
scanned 1,247 files in 38 directories
✓ snapshot abc12345 saved
  Files:       1,247 new,        0 unchanged
  Dirs:           38 new
  Added:    498.2 MiB (512.0 MiB processed)
  Duration:      8.2s
```

HitStor splits every file into variable-length chunks, deduplicates them, compresses them, and encrypts the result before writing it to the repository.

## List Snapshots

Each backup produces a **snapshot** -- a point-in-time record of the files that were backed up. List all snapshots with:

```console
hitstor snapshots --repo /path/to/repo --password "secret"
ID        Time                 Host    Tags  Files  Size
────────  ───────────────────  ──────  ────  ─────  ─────────
abc12345  2026-02-22 10:00:05  myhost        1,247  512.0 MiB
```

## Create a Second Backup (Incremental Behavior)

Run the same backup command again after making a few changes to `/data`:

```console
hitstor backup --repo /path/to/repo --password "secret" /data
scanning files...
scanned 1,247 files in 38 directories
✓ snapshot def67890 saved
  Files:           8 new,    1,239 unchanged
  Dirs:            2 new
  Added:     4.2 MiB (6.1 MiB processed)
  Duration:      1.1s
```

Notice how much faster the second backup is. HitStor only stores the chunks that are new or modified; unchanged data is deduplicated automatically.

```console
hitstor snapshots --repo /path/to/repo --password "secret"
ID        Time                 Host    Tags  Files  Size
────────  ───────────────────  ──────  ────  ─────  ─────────
def67890  2026-02-22 10:15:32  myhost        1,247  512.3 MiB
abc12345  2026-02-22 10:00:05  myhost        1,247  512.0 MiB
```

## Restore from a Snapshot

To restore the contents of a snapshot to a target directory, pass the snapshot ID and the destination path:

```console
hitstor restore --repo /path/to/repo --password "secret" abc12345 /restore/path
restoring snapshot abc12345 to /restore/path
✓ restored 1,247 files (512.0 MiB) in 6.3s
```

You can use the full snapshot ID or any unique prefix of it, such as the 8-character ID that `hitstor snapshots` shows. You can also use the keyword `latest` to restore the most recent snapshot:

```console
hitstor restore --repo /path/to/repo --password "secret" latest /restore/path
```

## Verify the Repository

The `check` command verifies the structural integrity of the repository: that its indexes, pack files, and snapshots are consistent and that all referenced data is present. Add `--read-data` to also read and decrypt every blob.

```console
hitstor check --repo /path/to/repo --password "secret"
checking indexes...
checking pack files...
checking snapshots...
no errors found
```

Run `check` periodically -- especially before and after migrating repository data -- to catch corruption early.

## Apply a Retention Policy

Over time the number of snapshots grows. The `forget` command removes snapshots that fall outside a retention policy, and the `prune` command reclaims the disk space used by data that is no longer referenced.

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --keep-last 7 \
    --keep-daily 30
snapshots: 2 kept, 0 forgotten
```

In this example both snapshots are recent enough to be retained. As your snapshot history grows, older snapshots that do not satisfy any policy rule will be removed.

## Reclaim Disk Space

After `forget` marks snapshots for removal, the underlying data chunks may still be present. Run `prune` to delete unreferenced data and repack partially-used pack files:

```console
hitstor prune --repo /path/to/repo --password "secret"
collecting referenced blobs...
classifying packs...
nothing to prune
prune complete
Packs:      0 deleted,    0 repacked
Freed:   0 B (repacked 0 B)
Indexes:    0 deleted,    0 written
```

When there is unreferenced data to clean up, `prune` reports exactly how much space was reclaimed.

## Next Steps

Now that you understand the basic workflow you can explore the rest of the User Guide:

- [Backup](backup.md) -- advanced backup options, exclude patterns, and parallel workers.
- [Restore](restore.md) -- selective restores, permissions, and verification.
- [Snapshots](snapshots.md) -- filtering, comparing, and inspecting snapshots.
- [Retention](retention.md) -- fine-grained retention strategies and automated pruning.
- [Configuration](configuration.md) -- config file, environment variables, and the setup wizard.
- [Plugins](plugins.md) -- extend HitStor with storage backends, notifiers, hooks, and scanners.
