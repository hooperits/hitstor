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
| [version](version.md) | Display version information |

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--repo` | string | | Repository location (path or `s3:endpoint/bucket`) — legacy single-repo |
| `--repo-name` | string | | Select repository by name from multi-repo config (preferred) |
| `--config` | string | `~/.config/hitstor/config.yaml` | Path to config file (on servers often `/etc/hitstor/config.yaml`) |
| `--password` | string | | Repository password |
| `--password-file` | string | | Path to file containing the repository password |
| `--parallel` | int | 0 | Number of parallel workers (0 = auto-detect, max 8) |
| `--s3-access-key` | string | | S3 access key |
| `--s3-secret-key` | string | | S3 secret key |
| `--s3-region` | string | us-east-1 | S3 region |
| `--json` | bool | false | Output in JSON format (where supported) |
| `--no-color` | bool | false | Disable colored output |
