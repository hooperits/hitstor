# Roadmap

HitStor is a **backup and restore product for Windows and Linux**: filesystems
in the core, and applications through installable
[workload plugins](../dev-guide/plugin-architecture.md). The roadmap exists to
make it excellent at that. Work that does not improve backing up and restoring
data fast, safely, and verifiably is out of scope by default.

## Shipped

| Item | Notes |
|------|-------|
| Deduplication & encryption engine | FastCDC chunking, Zstd, AES-256-GCM, scrypt KDF |
| Server / agent architecture | gRPC over TLS, thick agent chunks and compresses locally |
| Web dashboard | Jobs, agents, storage, restores, progress via SSE |
| Users, roles and two-step verification | Personal accounts with roles scoped to agents, mandatory TOTP, hash-chained audit log |
| Email | Invitations and password-reset links over SMTP or Microsoft 365 (OAuth 2.0) |
| Scheduled PDF reports | Protection summary and daily backup status, emailed on a schedule or downloaded |
| Incremental backups & synthetic full | Parent-snapshot reuse, periodic full rescan |
| Advanced scheduling | 5-field cron with `L`, `D#N` and `DL`, schedule builder, timezone-aware with DST handling |
| Agent auto-discovery | Network scan for unregistered agents |
| Snapshot restore | File-tree browse, granular and whole-path restore |
| Restore verification | Automated post-backup restore checks per job |
| Consistent snapshots | VSS (Windows), LVM and btrfs (Linux) |
| Guided disaster recovery | Multi-job full-system restore onto a rebuilt host |
| [Bare-metal restore (Linux)](../user-guide/bare-metal-restore.md) | ReaR rescue media built per host, extended attributes and system profile captured with `--bare-metal` |
| Benchmarks | Reproducible backup/restore harness, optional restic comparison |
| Ransomware Sentinel | Gemini telemetry over filesystem snapshots (optional) |
| Workload plugins | Signed `.hpk` packages installed per agent, locally or from the dashboard; backup and point-in-time restore through the agent; chain-aware retention |
| Backup plans | Workload jobs schedule full, differential, and log backups of one selection separately; results per database |
| [SQL Server](../user-guide/sql-server.md) | `mssql` plugin over VDI: full, differential, and log backups; point-in-time restore |

## Now

| Item | Goal |
|------|------|
| **Windows bare-metal restore** | WinPE recovery media, NTFS security descriptors, and BCD rebuild — see below |

## Next

| Item | Goal |
|------|------|
| **Remote filesystem restore** | Restore to an agent in another network or a cloud VM |
| **Restore performance** | Parallel restore streams and resume after interruption |

## Bare-metal restore { #bare-metal-restore }

[Bare-metal restore](../user-guide/bare-metal-restore.md) rebuilds a Linux host
from a blank disk. Relax-and-Recover (ReaR) builds bootable rescue media on each
host, recreates the disk layout, and reinstalls the bootloader; HitStor restores
the files. Backups taken with `--bare-metal` also capture extended attributes
(SELinux contexts, file capabilities, POSIX ACLs) and the system profile
(partition table, filesystem UUIDs, LVM, `/etc/fstab`).

Windows is the remaining part. It needs:

1. **Boot media** — a WinPE image carrying the agent.
2. **Richer file metadata** — Windows security descriptors, which the current
   snapshot format does not capture.
3. **Bootloader reconstruction** — BCD on Windows.

Restoring files alone onto a blank disk produces an **unbootable** system, so
these land together rather than as independent features.

## Permanently removed { #permanently-removed }

These were removed from the product before the first public release (v0.1.0)
and are not planned to return in their original form:

- Microsoft 365 and Google Workspace backup
- The built-in database integrations (PostgreSQL, MySQL, MariaDB, SQL Server, Oracle)
- Cloud VM agentless backup (GCP, AWS, Azure) and VMware/vSphere
- The workload collector plugin type that served them

Database backup now comes back as installable
[workload plugins](../dev-guide/plugin-architecture.md) that run on the
database host, starting with [SQL Server](../user-guide/sql-server.md).

Existing `config.yaml` files containing `collector_plugins` or the related
integration keys still load; those keys are simply ignored (unknown fields).

## Also not on the roadmap

Autonomous DR (auto-failover, LLM-generated bootstrap scripts, DNS promotion,
scheduled RPO/RTO drills) remains out of scope. If demand returns, it should
build on agent-based restore rather than bespoke cloud provisioning.

## See also

- [Disaster Recovery](../user-guide/disaster-recovery.md)
- [Restore](../user-guide/restore.md)
- [Sentinel](sentinel.md)
- [Codebase Overview](../dev-guide/codebase-overview.md)
