# `hitstor jobs`

Manage scheduled backup jobs (multi-repo aware).

> **Note:** Full job CRUD, scheduling, and execution is primarily performed via the dashboard. The CLI currently provides basic support and will be expanded.

## Usage

```bash
hitstor jobs [command]
```

## Subcommands

| Command | Description |
|---------|-------------|
| `list` | List configured backup jobs |
| `create` | Define a new backup job (currently prints definition; full persistence coming) |

## Flags for `create`

- `--name` (required)
- `--repo-name` (required)
- `--job-type` (only `files` is supported)
- `--path` (repeatable)
- `--interval-hours`, `--cron`, `--timezone`, `--agent-name`

## Examples

```bash
hitstor --config /etc/hitstor/config.yaml jobs list

hitstor --config /etc/hitstor/config.yaml jobs create \
  --name "Daily /home" \
  --repo-name Production \
  --job-type files \
  --path /home \
  --interval-hours 24
```

For production job management use the dashboard Storage + Jobs pages.
