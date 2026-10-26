# Disaster recovery (guided full-system restore)

HitStor protects **filesystems**, not raw disks. Disaster recovery means rebuilding a host from its file backups onto new or repaired hardware with a **guided dashboard wizard**.

## What this is (and is not)

| Is | Is not |
|----|--------|
| Full restore of all backup jobs for a system | Block-level disk imaging |
| Sequential restore of each job’s latest snapshot | Automatic OS reinstall / bootloader repair |
| Staging root or original-path modes | Hypervisor VM import |

After the OS and HitStor agent are installed on the recovery host, the wizard restores application and data paths from the repository.

For a machine with a **blank disk** — no OS to install an agent onto, no
partition table to restore into — see [Bare-metal restore](bare-metal-restore.md),
which recreates the disk layout and bootloader first and then hands the data
restore back to this wizard.

## Prerequisites

1. Recovery host has a compatible OS (Windows or Linux).
2. HitStor **agent** installed and **registered** with the backup server.
3. Enough free disk for the data (or a large staging volume).
4. Source system’s backup jobs have completed at least once (snapshots present).

Optional but recommended: jobs with **restore verification** green (see Backups page).

## Dashboard wizard

Open **Disaster Recovery** in the sidebar (`/disaster`).

1. **Select system** — protected agent (source) with jobs/snapshots.
2. **Review plan** — each job’s latest snapshot, paths, size, last verify status.
3. **Target agent** — usually the new host; can be the same agent after rebuild.
4. **Mode**
   - **Staging root** (default): restore under e.g. `/recover/hostname/job-name` for review, then cut over.
   - **Original paths**: write to live paths (stop services that lock files first).
5. **Start full-system recovery** — restores run **one job after another** on the target agent.

Progress appears on the same page (and under Restores for individual sessions).

## API (operators / automation)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/disaster/plans` | Summary of systems and readiness |
| GET | `/api/disaster/plans/{agent}` | Full plan + checklist |
| POST | `/api/disaster/recover` | Start recovery |
| GET | `/api/disaster/recoveries` | List orchestrated recoveries |
| GET | `/api/disaster/recoveries/{id}` | One recovery status |

Example:

```bash
curl -sS -X POST "$SERVER/api/disaster/recover" \
  -H 'Content-Type: application/json' \
  -d '{
    "source_agent": "web-01",
    "target_agent": "web-01-new",
    "mode": "staging",
    "staging_root": "/recover/web-01"
  }'
```

## After recovery

1. Validate application data under staging (or live paths).
2. Cut over DNS/services as needed.
3. Re-point or recreate backup jobs on the recovery agent.
4. Optionally run `hitstor verify-restore <snapshot>` for an extra integrity check.

## Related

- [Bare-metal restore](bare-metal-restore.md) — recovering a machine with a blank disk
- [Restore files](restore.md) — single-snapshot granular restore
- [Agents](../admin-guide/agent-deployment.md) — install/register recovery host
- [Benchmarks](../dev-guide/benchmarks.md) — restore performance expectations
