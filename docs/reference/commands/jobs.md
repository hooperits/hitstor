# `hitstor jobs`

Manage scheduled backup jobs (multi-repo aware).

> **Note:** Full job management (editing, workload jobs with backup plans, running and cancelling) is done in the dashboard. The CLI covers listing, creating file jobs, and repairing orphaned jobs.

## Usage

```bash
hitstor jobs [command]
```

## Subcommands

| Command | Description |
|---------|-------------|
| `list` | List configured backup jobs (`--agent <name>` filters by agent; `--orphans` shows only orphaned jobs) |
| `create` | Create a file backup job in the agent registry of the repository given by `--repo-name` |
| `orphans` | List jobs whose agent is no longer registered |
| `reattach` | Re-attach jobs to an agent: `--agent <name>` (required) with `--all-orphans` or `--job-id <id>` (repeatable) |

## Flags for `create`

- `--name` (required)
- `--repo-name` (required)
- `--job-type` (only `files` is supported; workload jobs are created in the dashboard)
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

`create` stores the job in the agent registry of the `--repo-name` repository, which is also where the job's backups go. A running server reads jobs from the registry of its first repository (by name), so create jobs that back up to another repository from the dashboard. For production job management use the dashboard **Backups** page.
