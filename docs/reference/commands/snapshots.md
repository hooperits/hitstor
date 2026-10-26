# hitstor snapshots

List all snapshots in the repository.

## Synopsis

```
hitstor snapshots [flags]
```

## Description

Lists all snapshots stored in the repository, sorted by time (newest first). By default, output is a formatted table showing each snapshot's short ID, timestamp, hostname, paths, file count, and total size.

Use `--json` for machine-readable JSON output.

Alias: `hitstor ls`

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--host` | string | | Show only snapshots from this hostname |
| `--path` | string | | Show only snapshots that include this backed-up path |

Plus all [global flags](index.md#global-flags), including `--repo-name` to target a specific repository in multi-repo setups.

## Examples

List all snapshots:

```bash
hitstor snapshots --repo /backups/hitstor --password "secret"
```

Filter by hostname:

```bash
hitstor snapshots --repo /backups/hitstor --password "secret" --host webserver-01
```

JSON output:

```bash
hitstor snapshots --repo /backups/hitstor --password "secret" --json
```

## Output Format

### Table (default)

```
ID         Time                 Host         Paths                   Files       Size
a1b2c3d4   2025-01-15 10:30:00  myhost       /home/user/documents      150     45 MiB
b5c6d7e8   2025-01-14 10:30:00  myhost       /home/user/documents      148     44 MiB
```

### JSON (`--json`)

```json
[
  {
    "id": "a1b2c3d4",
    "time": "2025-01-15T10:30:00Z",
    "hostname": "myhost",
    "paths": ["/home/user/documents"],
    "total_files": 150,
    "total_size": 47185920
  }
]
```
