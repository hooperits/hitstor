# Backup

The `hitstor backup` command creates a new snapshot of one or more directories. Each snapshot captures the full state of the specified paths at the time the backup runs, but thanks to content-defined chunking and deduplication, only new or changed data is actually written to the repository.

## Basic Usage

```console
hitstor backup --repo /path/to/repo --password "secret" /path/to/backup
```

This creates a snapshot containing every file under `/path/to/backup`.

## Backing Up Multiple Paths

You can specify more than one source path in a single backup:

```console
hitstor backup --repo /path/to/repo --password "secret" \
    /home/user/documents \
    /home/user/photos \
    /etc
```

All listed paths are included in the same snapshot.

## Exclude Patterns

### Command-Line Excludes

Use `--exclude` to skip files or directories that match a pattern:

```console
hitstor backup --repo /path/to/repo --password "secret" \
    --exclude "*.tmp" \
    --exclude "node_modules" \
    /home/user/projects
```

Patterns follow standard glob syntax:

| Pattern | Matches |
|---------|---------|
| `*.log` | All files ending in `.log` |
| `cache/` | Any directory named `cache` |
| `/data/temp` | The exact path `/data/temp` relative to the backup root |
| `**/build` | A directory named `build` at any depth |

### Config File Excludes

You can also define excludes in the configuration file (`~/.config/hitstor/config.yaml`):

```yaml
backup:
  exclude:
    - "*.tmp"
    - ".cache"
    - "node_modules"
```

### Merging Behavior

Excludes from both sources are merged at runtime. A file is excluded if it matches **any** pattern from either the command line or the config file. Command-line patterns do not replace config-file patterns; they add to them.

## Tags

Tags let you label snapshots for easier filtering later. Use `--tag` (repeatable) to attach one or more tags:

```console
hitstor backup --repo /path/to/repo --password "secret" \
    --tag project-alpha \
    --tag weekly \
    /data/project-alpha
```

Tags appear in the `hitstor snapshots` output and can be used with `--keep-tag` in retention policies. See [Retention](retention.md) for details.

## Host Override

By default HitStor records the system hostname in each snapshot. If you back up the same data from multiple machines, or if your hostname changes, you can override it:

```console
hitstor backup --repo /path/to/repo --password "secret" \
    --host webserver-01 \
    /var/www
```

The host value is used by `forget` when applying per-host retention policies.

## Dry Run Mode

To preview what a backup would do without writing any data, pass `--dry-run`:

```console
hitstor backup --repo /path/to/repo --password "secret" \
    --dry-run \
    /data
```

The command scans the source paths, reports file counts and estimated data sizes, but does not create a snapshot or write any packs.

## Parallel Workers

HitStor processes files concurrently. The `--parallel` flag controls the number of worker goroutines:

```console
hitstor backup --repo /path/to/repo --password "secret" \
    --parallel 4 \
    /data
```

The default behavior auto-detects the number of CPUs and caps at **8** workers. On I/O-bound workloads (network storage, spinning disks) you may get better throughput with fewer workers; on fast SSDs you can try increasing the count.

| Situation | Recommended `--parallel` |
|-----------|--------------------------|
| NVMe SSD, many small files | 6--8 |
| Spinning disk | 1--2 |
| Network mount (NFS, SMB) | 2--4 |
| S3 backend | 4--8 |

## Progress Display

While the backup runs, HitStor prints a live progress line showing:

- Number of files scanned
- Amount of data processed
- Amount of new data added to the repository
- Elapsed time and estimated time remaining

The progress display updates in place on terminals that support carriage returns. When the output is redirected to a file or pipe, HitStor disables the live updates and prints a final summary instead.

## How Deduplication Works

HitStor uses a three-layer deduplication strategy to minimize storage consumption:

### 1. Content-Defined Chunking (FastCDC)

Files are split into variable-length chunks using the FastCDC algorithm. Unlike fixed-size splitting, FastCDC is resilient to insertions and deletions: if you add a paragraph to the beginning of a large file, only the first few chunks change while the rest remain identical.

### 2. Chunk-Level Deduplication

Every chunk is identified by its cryptographic hash. Before writing a chunk to the repository, HitStor checks whether a chunk with the same hash already exists. If it does, the chunk is simply referenced rather than stored again.

### 3. Pack-Level Aggregation

Individual chunks are grouped into **pack files** to reduce the number of objects in the repository and improve I/O efficiency. Each pack file contains many chunks and is accompanied by an index that maps chunk hashes to offsets within the pack.

Together these layers mean that:

- Identical files across different snapshots are stored only once.
- Files that differ by a small amount share most of their chunks.
- Backups of similar machines against the same repository benefit from cross-machine deduplication.

## S3 Backend Usage

HitStor supports Amazon S3 (and S3-compatible services) as a backend. To back up to an S3 bucket:

```console
hitstor init --repo s3:s3.amazonaws.com/my-bucket/hitstor-repo --password "secret"
```

Then run backups against the same repo URI:

```console
hitstor backup --repo s3:s3.amazonaws.com/my-bucket/hitstor-repo \
    --password "secret" \
    /data
```

Configure credentials through standard AWS environment variables or the shared credentials file:

| Variable | Purpose |
|----------|---------|
| `AWS_ACCESS_KEY_ID` | Access key |
| `AWS_SECRET_ACCESS_KEY` | Secret key |
| `AWS_DEFAULT_REGION` | Bucket region |

For S3-compatible services (MinIO, Backblaze B2, Wasabi), point the URI to the appropriate endpoint:

```console
hitstor init --repo s3:https://s3.us-west-001.backblazeb2.com/my-bucket/hitstor-repo \
    --password "secret"
```

!!! tip
    Store your repository path and credentials in the config file or environment variables so you do not have to type them on every command. See [Configuration](configuration.md).
