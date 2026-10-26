# Backup Jobs

The dashboard **Backups** page (`/backups`) is the central place to define what gets backed up and when. Each backup job has a `job_type` that determines how data is collected.

## Job Types

| Type | Collector | Runs on |
|------|-----------|---------|
| `files` | Agent filesystem scan via built-in plugin (hitstor-scanner-builtin, shipped with agent) or custom scanner plugin | Remote agent |

`files` is the only supported job type. SaaS, database, and cloud-VM job types
were permanently removed in v2.0.0 — see the [Roadmap](roadmap.md#permanently-removed-v200).
Legacy jobs of those types are loaded but disabled on startup.

## Filesystem Jobs

The default job type. The server assigns paths and schedule to an agent; the agent scans, chunks, and uploads new data.

- Configure paths via the job detail page or agent browse UI
- Enable **VSS** on Windows agents for consistent snapshots
- Set **full rescan interval** (days) to guard against silent corruption
- Use **Force full backup** from the dashboard when needed

## Scheduling

Each job supports:

- **Interval** — every N hours
- **Cron** — 5-field cron with IANA timezone
- **Visual schedule builder** — daily / weekly / monthly presets

The agent daemon polls `GetSchedule` every 5 minutes (default). "Backup Now" sends a `WakeUp` RPC for immediate pickup.

## Cancel Running Backups

`POST /api/backups/{id}/cancel` aborts an active session. Stale sessions (no activity for 10+ minutes) show a warning badge on the dashboard.

## API

See [Dashboard HTTP API — Backup Jobs](../reference/dashboard-api.md#backup-jobs).