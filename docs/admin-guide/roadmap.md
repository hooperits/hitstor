# Roadmap

HitStor is a **filesystem backup and restore product for Windows and Linux**, and
the roadmap exists to make it excellent at exactly that. Work that does not
improve backing up and restoring files fast, safely, and verifiably is out of
scope by default.

## Shipped

| Item | Notes |
|------|-------|
| Deduplication & encryption engine | FastCDC chunking, Zstd, AES-256-GCM, scrypt KDF |
| Server / agent architecture | gRPC over TLS, thick agent chunks and compresses locally |
| Web dashboard | Jobs, agents, storage, restores, progress via SSE |
| Incremental backups & synthetic full | Parent-snapshot reuse, periodic full rescan |
| Advanced scheduling | Interval, 5-field cron, timezone-aware with DST handling |
| Agent auto-discovery | Network scan for unregistered agents |
| Snapshot restore | File-tree browse, granular and whole-path restore |
| Restore verification | Automated post-backup restore checks per job |
| Consistent snapshots | VSS (Windows), LVM and btrfs (Linux) |
| Guided disaster recovery | Multi-job full-system restore onto a rebuilt host |
| Benchmarks | Reproducible backup/restore harness, optional restic comparison |
| Ransomware Sentinel | Gemini telemetry over filesystem snapshots (optional) |

## Now

| Item | Goal |
|------|------|
| **Bare-metal restore** | Bootable recovery media plus the metadata needed to rebuild a bootable system — see below |

## Next

| Item | Goal |
|------|------|
| **Remote filesystem restore** | Restore to an agent in another network or a cloud VM |
| **Restore performance** | Parallel restore streams and resume after interruption |

## Bare-metal restore (in design)

Today's [disaster recovery](../user-guide/disaster-recovery.md) wizard restores
files onto a host that **already boots** and already runs a registered agent.
True bare-metal restore additionally requires:

1. **Boot media** — a bootable Linux/WinPE image carrying the agent, so a machine
   with a blank disk can be recovered.
2. **Richer file metadata** — Windows security descriptors and POSIX
   ACLs/xattrs (including SELinux contexts), which the current snapshot format
   does not capture.
3. **Disk layout capture** — partition table, filesystem UUIDs, and LVM
   configuration, so partitions can be recreated and `/etc/fstab` still resolves.
4. **Bootloader reconstruction** — GRUB/EFI on Linux and BCD on Windows.

Restoring files alone onto a blank disk produces an **unbootable** system, so
these land together rather than as independent features.

## Permanently removed (v2.0.0)

These are **not** deferred — they were removed from the product and are not
planned to return:

- Microsoft 365 and Google Workspace backup
- Database backup (PostgreSQL, MySQL, MariaDB, SQL Server, Oracle)
- Cloud VM agentless backup (GCP, AWS, Azure) and VMware/vSphere
- The workload collector plugin type that served them

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
