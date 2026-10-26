# hitstor bmr

Build and inspect bare-metal recovery media.

## Synopsis

```
hitstor bmr check [flags]
hitstor bmr generate-config [flags]
hitstor bmr build-media [flags]
```

## Description

Bare-metal recovery media lets a machine with a blank disk boot, recreate its disk layout, and pull its data back from HitStor.

These commands run **on the protected host itself**: the image embeds that host's kernel, drivers, and disk layout, so it cannot be produced from a backup alone. Building media is a deliberate action, not something a backup does for you — run it after the host's storage layout changes.

Linux only; HitStor drives [Relax-and-Recover (ReaR)](https://relax-and-recover.org/) underneath.

The same command tree is available as `hitstor-agent bmr` — see [hitstor-agent bmr](../agent-commands/bmr.md) for the one behavioral difference.

## Subcommands

### `check`

Reports everything standing between this host and a bootable rescue image: whether ReaR is installed and which version, which supporting tools are present, and whether the command has the privileges ReaR needs.

Reports every gap at once rather than stopping at the first, and exits non-zero when the host is not ready, so it can gate a provisioning script.

```bash
hitstor bmr check
```

### `generate-config`

Writes `/etc/rear/local.conf` and the `EXTERNAL_RESTORE` hook the rescue system runs once it has recreated the disk layout.

No credentials are written: the recovery token and, where needed, the repository password are prompted for at recovery time, so the media can be stored without becoming a way into the repository.

`build-media` regenerates both files, so running this separately is only needed to review or hand-edit them first.

```bash
hitstor bmr generate-config --server backup.internal:8420 --agent web-01
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--stdout` | bool | false | Print the configuration instead of writing it |

Plus the [shared media flags](#shared-media-flags) below.

### `build-media`

Runs ReaR to produce a bootable rescue image for this host, after writing the configuration that points its recovery run back at HitStor.

The build takes several minutes and reads every disk on the machine, so it is deliberately manual. Rebuild it whenever the host's disk layout changes — a stale image recreates the old layout.

```bash
hitstor bmr build-media --server backup.internal:8420 --agent web-01
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Build even when the readiness check reports blockers |

Plus the [shared media flags](#shared-media-flags) below.

## Shared media flags

`generate-config` and `build-media` both take:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--output-dir` | string | `/var/lib/hitstor/bmr` | Directory ReaR writes the finished ISO into |
| `--agent-binary` | string | `/usr/bin/hitstor-agent` | `hitstor-agent` binary to embed in the rescue image |
| `--hook-script` | string | `/usr/share/hitstor/bmr/hitstor-recover.sh` | Path to write the ReaR `EXTERNAL_RESTORE` hook to |
| `--config-path` | string | `/etc/rear/local.conf` | Path to write the generated ReaR configuration to |
| `--server` | string | | HitStor server address to prefill in the recovery prompt (`host:port`) |
| `--agent` | string | | Registered agent name to prefill in the recovery prompt |
| `--migration-mode` | bool | false | Allow recovery onto dissimilar hardware or a virtual machine (ReaR `MIGRATION_MODE`) |
| `--copy` | stringArray | | Additional absolute path to embed in the rescue image (repeatable) |

## Notes

- `check`, `generate-config`, and `build-media` all support the global `--json` flag for machine-readable output.
- Recovery itself is driven by `hitstor-agent recover`, invoked automatically from the rescue media once ReaR has recreated the disk layout — see [hitstor-agent recover](../agent-commands/recover.md).
- See [Bare-metal restore](../../user-guide/bare-metal-restore.md) for the full workflow, including what the recovery media does and does not capture.
