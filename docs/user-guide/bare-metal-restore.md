# Bare-metal restore

[Guided disaster recovery](disaster-recovery.md) rebuilds a host that **already
boots** and **already runs a registered agent**. Bare-metal restore covers the
case it cannot: a machine with a blank disk, where there is no OS to install an
agent onto and no partition table for the restored files to land in.

HitStor drives [Relax-and-Recover (ReaR)](https://relax-and-recover.org/) for
the parts that are not about data. ReaR builds bootable rescue media, recreates
the disk layout, and reinstalls the bootloader; HitStor restores the contents.

!!! note "Linux only for now"
    Windows bare-metal recovery (WinPE media, NTFS security descriptors,
    `bcdboot`) arrives in a later release.

## Why the media has to be built in advance

A rescue image embeds the host's own kernel, its drivers, and its disk layout.
It can only be produced **on the machine it protects**, while that machine is
still running — which is exactly what you no longer have when you need it.

So building recovery media is a deliberate, scheduled task, not something a
backup does for you. Rebuild it whenever a host's storage layout changes: a
stale image recreates the *old* layout.

The image is cached on the server rather than fetched from the host on demand.
A design that rebuilt the ISO at download time would need the source host
online and spend several minutes doing it, which breaks the feature in the one
scenario it exists for.

## Capturing what a bare-metal restore needs

File contents alone do not describe a bootable machine. Add `--bare-metal` to a
backup to capture the rest:

```bash
hitstor backup /etc /home /var --bare-metal
```

That records two things beyond the usual file tree:

- **Extended attributes** — on Linux, SELinux contexts, file capabilities and
  POSIX ACLs are all xattrs, so one mechanism covers all three. Without them an
  SELinux system restores to an unlabeled filesystem whose services refuse to
  start.
- **The system profile** — partition table, filesystem UUIDs and labels, LVM,
  software RAID, `/etc/fstab`, and bootloader state. Stored as an encrypted
  blob referenced from the snapshot, so the backup describes the machine's
  shape and not only its contents. The layout can be reconstructed even if the
  rescue media is lost.

Capture is best-effort throughout. A missing tool leaves that section empty and
records a warning rather than failing the backup.

## Checking a host can build media

Run this on the protected host:

```bash
sudo hitstor-agent bmr check
```

It reports everything standing between that host and a bootable image at once —
ReaR and its version, supporting tools, privileges — rather than stopping at the
first problem, and exits non-zero when the host is not ready, so it can gate a
provisioning script.

The same readiness appears per-host under **Disaster Recovery → Bare Metal** in
the dashboard.

## Building the media

From the dashboard, under **Disaster Recovery → Bare Metal**, press **Build
media** on a host. The server asks that host's agent to run ReaR and streams the
resulting ISO back, showing the build as it happens. When it finishes, the image
is cached on the server and **Download** appears.

Or on the host itself:

```bash
sudo hitstor-agent bmr build-media --server backup.example.com:8420
```

Useful flags:

| Flag | Purpose |
|------|---------|
| `--migration-mode` | Allow recovery onto dissimilar hardware or a VM (P2V). ReaR remaps devices and regenerates the initramfs with the drivers the new hardware needs. |
| `--copy PATH` | Embed an extra file in the rescue image — a corporate CA bundle, say, without which the agent cannot verify the server. |
| `--agent NAME` | Registered agent name to prefill at the rescue prompt, when it differs from the hostname. |
| `--force` | Build despite readiness blockers. |

`rear mkrescue` reads every disk on the machine and takes several minutes, so
run it at a time when the extra load is acceptable.

Store the ISO somewhere that survives losing the host it came from.

## What is not on the media

**No credentials.** The recovery token, and where needed the repository
password, are typed in at recovery time.

This is deliberate. Removable media that carried a working key would be a
repository-compromise vector, and an ISO that still works a year after it was
built, sitting on a shelf, is exactly the risk a long-lived credential creates.
The cost is that unattended recovery is not possible unless you opt into it.

## Recovering a machine

1. **Download** that host's ISO from the Bare Metal tab and boot the
   replacement hardware from it.
2. Back in the dashboard, press **Issue token** for the host. The token appears
   **once** — copy it. It works a single time, only for that host, and expires
   after 24 hours by default. If it leaks before use, **Revoke** it.
3. At the rescue console, confirm the server address and agent name, then type
   the token. ReaR recreates the disk layout, mounts the new root at
   `/mnt/local`, and the recovery agent announces itself to the server.
4. Return to **Guided recovery** and start the restore with the announced host
   as the target. Nothing starts on its own: a bare-metal restore is confirmed
   by an operator, like any other.
5. When the restore reaches a terminal state the recovery agent exits, ReaR
   installs the bootloader, and the machine can be rebooted into the restored
   system.

### How the rescue host is reached

Every other restore is server-initiated: the server already knows an agent's
address and dials in. A machine that just booted off rescue media has whatever
address DHCP handed it, which nobody has told the server about.

So the rescue system calls home — presenting its recovery token and current
address — and the server then dials back over the ordinary restore path,
unchanged. The rescue system generates its own listener token for that
dial-back; the original agent's token is never disclosed and stays valid for
when the restored OS boots.

!!! warning "The server must be able to reach the rescue host"
    The dial-back needs a direct route to the address the rescue host
    announces. That holds for the same-LAN and P2V cases this release supports.
    A rescue host behind aggressive NAT across a WAN is not yet supported; the
    restore fails at start with the address it tried, rather than hanging.

## API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/bmr/hosts` | Per-host readiness and cached media |
| POST | `/api/bmr/media/{agent}/build` | Start a build; returns an operation id |
| GET | `/api/operations/{id}/events` | Build progress (SSE) |
| GET | `/api/bmr/media/{agent}/download` | Download the cached ISO |
| DELETE | `/api/bmr/media/{agent}` | Discard the cached ISO |
| POST | `/api/bmr/prepare/{agent}` | Mint a recovery token (returned once) |
| GET | `/api/bmr/tokens` | List outstanding tokens |
| DELETE | `/api/bmr/tokens/{id}` | Revoke an unused token |

```bash
# Issue a token with a tighter window than the 24h default.
curl -sS -X POST "$SERVER/api/bmr/prepare/web-01" \
  -H 'Content-Type: application/json' \
  -d '{"ttl_hours": 2}'
```

## Troubleshooting

**`bmr check` reports blockers.** Install what it names. On most distributions
that is `rear` plus an ISO writer (`xorriso` or `genisoimage`). The command must
run as root: ReaR reads raw block devices.

**The build fails with a ReaR error.** The message carries ReaR's own diagnosis.
A previously cached image is left untouched — a rebuild that fails does not cost
you the media you already had.

**The rescue host announces but the restore will not start.** The server could
not reach the address it announced. Check routing and firewalls between the two,
and that the port the rescue agent listens on (8422 by default) is open.

**The token is rejected.** Tokens are single-use, expire, and are bound to one
host. Issue a fresh one; the error says which of the three applied.

## Related

- [Disaster recovery](disaster-recovery.md) — guided restore onto a booting host
- [Restore files](restore.md) — single-snapshot granular restore
- [Agents](../admin-guide/agent-deployment.md) — install and register an agent
