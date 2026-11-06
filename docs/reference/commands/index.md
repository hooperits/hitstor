# Server Commands

Reference for all `hitstor` CLI commands.

## Commands

| Command | Description |
|---------|-------------|
| [init](init.md) | Initialize a new backup repository |
| [backup](backup.md) | Back up files to the repository |
| [restore](restore.md) | Restore files from a snapshot |
| [snapshots](snapshots.md) | List all snapshots (alias: `ls`) |
| [forget](forget.md) | Remove snapshots by ID or retention policy |
| [prune](prune.md) | Remove unreferenced data from the repository |
| [check](check.md) | Verify repository integrity |
| [serve](serve.md) | Start the gRPC backup server |
| [config](config.md) | Manage configuration |
| [diff](diff.md) | Compare two snapshots |
| [du](du.md) | Show repository disk usage |
| [repo](repo.md) | Repository operations (multi-repo aware) |
| [jobs](jobs.md) | Manage scheduled backup jobs |
| [agents](agents.md) | Manage registered backup agents |
| [verify-restore](verify-restore.md) | Test-restore a snapshot and re-hash its files |
| [bmr](bmr.md) | Build and inspect bare-metal recovery media |
| [version](version.md) | Display version information |

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--repo` | string | | Repository location (path or `s3:endpoint/bucket`) — legacy single-repo |
| `--repo-name` | string | | Select repository by name from multi-repo config (preferred) |
| `--config` | string | `/etc/hitstor/config.yaml` if it exists and is readable, else `~/.config/hitstor/config.yaml` | Path to config file |
| `--password` | string | | Repository password |
| `--password-file` | string | | Path to file containing the repository password |
| `--parallel` | int | 0 | Number of parallel workers (0 = auto: the number of CPUs, at most 8) |
| `--s3-access-key` | string | | S3 access key |
| `--s3-secret-key` | string | | S3 secret key |
| `--s3-region` | string | us-east-1 | S3 region |
| `--json` | bool | false | Output in JSON format (where supported) |
| `--break-lock` | bool | false | Remove stale repository locks before the operation |
| `-q`, `--quiet` | bool | false | Suppress non-error output |
| `-v`, `--verbose` | count | | Increase log verbosity (repeat for more: `-vv`) |

Colored output is turned off when output is not a terminal or the `NO_COLOR` environment variable is set.
