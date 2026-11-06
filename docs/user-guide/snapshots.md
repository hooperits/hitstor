# Snapshots

Every time you run `hitstor backup`, a new **snapshot** is created. A snapshot is a metadata record that points to the deduplicated chunks making up your files at a specific point in time. This page covers how to list, filter, compare, and inspect snapshots.

## List All Snapshots

```console
hitstor snapshots --repo /path/to/repo --password "secret"
ID        Time                 Host       Tags    Files  Size
────────  ───────────────────  ─────────  ──────  ─────  ─────────
def45678  2026-02-22 10:30:00  laptop     manual  8,312  2.1 GiB
cde34567  2026-02-22 08:00:11  webserver  daily   1,204  96.4 MiB
bcd23456  2026-02-21 08:00:09  webserver  daily   1,201  96.1 MiB
abc12345  2026-02-20 08:00:12  webserver  daily   1,198  95.8 MiB
```

The `hitstor ls` alias works identically:

```console
hitstor ls --repo /path/to/repo --password "secret"
```

## Filtering Snapshots

### By Host

Show only snapshots recorded from a specific host:

```console
hitstor snapshots --repo /path/to/repo --password "secret" --host webserver
ID        Time                 Host       Tags    Files  Size
────────  ───────────────────  ─────────  ──────  ─────  ─────────
cde34567  2026-02-22 08:00:11  webserver  daily   1,204  96.4 MiB
bcd23456  2026-02-21 08:00:09  webserver  daily   1,201  96.1 MiB
abc12345  2026-02-20 08:00:12  webserver  daily   1,198  95.8 MiB
```

### By Path

Show only snapshots that include a specific backup path:

```console
hitstor snapshots --repo /path/to/repo --password "secret" --path /home/user
ID        Time                 Host       Tags    Files  Size
────────  ───────────────────  ─────────  ──────  ─────  ─────────
def45678  2026-02-22 10:30:00  laptop     manual  8,312  2.1 GiB
```

### Combining Filters

`--host` and `--path` can be used together. A snapshot must match **both** filters to appear.

## JSON Output

For scripting and integration with other tools, use `--json` to produce machine-readable output:

```console
hitstor snapshots --repo /path/to/repo --password "secret" --json
```

The output is a JSON array of snapshot objects:

```json
[
  {
    "id": "abc12345",
    "time": "2026-02-20T08:00:12Z",
    "hostname": "webserver",
    "tags": ["daily"],
    "paths": ["/var/www"],
    "total_files": 1198,
    "total_size": 100453171
  },
  ...
]
```

This is useful for piping into `jq` or other JSON processors:

```console
hitstor snapshots --repo /path/to/repo --password "secret" --json \
    | jq '.[].id'
```

## Understanding Snapshot IDs

Each snapshot is identified by a hexadecimal string derived from a cryptographic hash of its contents. In the default table output, IDs are shown as a short prefix (typically 8 characters).

### Short Prefix Matching

Whenever a command accepts a snapshot ID, you can provide a short prefix instead of the full hex string. The prefix must be long enough to uniquely identify one snapshot in the repository. If the prefix is ambiguous, HitStor reports how many snapshots match so you can provide a longer prefix.

```console
hitstor snapshots --repo /path/to/repo --password "secret"
ID        ...
────────  ...
abc12345  ...
abc12399  ...

hitstor restore --repo /path/to/repo --password "secret" abc123 /target
Error: snapshot prefix "abc123" is ambiguous (2 matches)

hitstor restore --repo /path/to/repo --password "secret" abc12345 /target
restoring snapshot abc12345 ...
```

### The `latest` Keyword

The keyword `latest` can be used in place of a snapshot ID. It resolves to the most recent snapshot in the repository (by timestamp).

## Comparing Snapshots

The `hitstor diff` command shows what changed between two snapshots:

```console
hitstor diff --repo /path/to/repo --password "secret" abc12345 bcd23456
```

The output lists files that were added, removed, or modified, with paths relative to the snapshot root (a backup of `/var/www` stores its files under `www/`), followed by a summary:

```
comparing snapshots abc12345 ↔ bcd23456

+  www/blog/new-post.html
-  www/old-page.html
M  www/index.html
M  www/css/style.css

4 differences (1 added, 1 removed, 2 modified)
```

| Prefix | Meaning |
|--------|---------|
| `+` | File added in the second snapshot |
| `-` | File removed in the second snapshot |
| `M` | File modified between the two snapshots |

This is helpful for auditing what changed between backups or verifying that an expected set of changes was captured.

## Disk Usage

The `hitstor du` command reports the number of snapshots, pack files, and index files in the repository and the total space they use:

```console
hitstor du --repo /path/to/repo --password "secret"
Snapshots:     4
Packs:        38
Indexes:       4
Total size: 2.2 GiB
```

Because of deduplication, the total may be much smaller than the sum of the `Size` column of `hitstor snapshots`, which shows each snapshot's original file sizes. Use `--json` for machine-readable output.

!!! note
    `hitstor du` lists the repository's files and adds up their sizes; it does not download or decrypt the actual data, so it is fast even for large repositories.
