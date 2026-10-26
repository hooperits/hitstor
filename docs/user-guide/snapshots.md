# Snapshots

Every time you run `hitstor backup`, a new **snapshot** is created. A snapshot is a metadata record that points to the deduplicated chunks making up your files at a specific point in time. This page covers how to list, filter, compare, and inspect snapshots.

## List All Snapshots

```console
hitstor snapshots --repo /path/to/repo --password "secret"
ID        Time                 Host        Tags        Paths
----------------------------------------------------------------------------
abc12345  2026-02-20 08:00:12  webserver   daily       /var/www
bcd23456  2026-02-21 08:00:09  webserver   daily       /var/www
cde34567  2026-02-22 08:00:11  webserver   daily       /var/www
def45678  2026-02-22 10:30:00  laptop      manual      /home/user
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
ID        Time                 Host        Tags        Paths
----------------------------------------------------------------------------
abc12345  2026-02-20 08:00:12  webserver   daily       /var/www
bcd23456  2026-02-21 08:00:09  webserver   daily       /var/www
cde34567  2026-02-22 08:00:11  webserver   daily       /var/www
```

### By Path

Show only snapshots that include a specific backup path:

```console
hitstor snapshots --repo /path/to/repo --password "secret" --path /home/user
ID        Time                 Host        Tags        Paths
----------------------------------------------------------------------------
def45678  2026-02-22 10:30:00  laptop      manual      /home/user
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
    "id": "abc12345...",
    "time": "2026-02-20T08:00:12Z",
    "host": "webserver",
    "tags": ["daily"],
    "paths": ["/var/www"]
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

Whenever a command accepts a snapshot ID, you can provide a short prefix instead of the full hex string. The prefix must be long enough to uniquely identify one snapshot in the repository. If the prefix is ambiguous, HitStor prints an error listing the matching snapshots so you can provide a longer prefix.

```console
hitstor snapshots --repo /path/to/repo --password "secret"
ID        ...
--------  ...
abc12345  ...
abc12399  ...

hitstor restore --repo /path/to/repo --password "secret" abc123 /target
Fatal: prefix "abc123" matches multiple snapshots: abc12345, abc12399

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

The output lists files that were added, removed, or modified:

```
+    /var/www/blog/new-post.html
-    /var/www/old-page.html
M    /var/www/index.html
M    /var/www/css/style.css
```

| Prefix | Meaning |
|--------|---------|
| `+` | File added in the second snapshot |
| `-` | File removed in the second snapshot |
| `M` | File modified between the two snapshots |

This is helpful for auditing what changed between backups or verifying that an expected set of changes was captured.

## Disk Usage

The `hitstor du` command reports how much repository space is consumed by one or more snapshots:

```console
hitstor du --repo /path/to/repo --password "secret"
Snapshot    Size
---------------------
abc12345    512.00 MiB
bcd23456      4.20 MiB
cde34567      3.80 MiB
def45678    128.00 MiB
---------------------
Total:      648.00 MiB
```

Because of deduplication, the per-snapshot sizes reflect only the **unique** data each snapshot contributes. The total may be much smaller than the sum of the original file sizes across all snapshots.

!!! note
    `hitstor du` reads the repository index and does not download or decrypt the actual data, so it is fast even for large repositories.
