# hitstor-agent recover

Run in bare-metal recovery mode (invoked from rescue media).

## Synopsis

```
hitstor-agent recover --token TOKEN [flags]
```

## Description

Recovery mode is what the rescue system runs after ReaR has recreated the machine's disk layout and mounted the new root.

It is not a normal agent: it starts no schedule, writes no configuration, and exits as soon as the restore it is here for reaches a terminal state.

Every other restore in HitStor is started by the server dialling an agent it already knows the address of. A machine that just booted off rescue media has whatever address DHCP handed it, which nobody has told the server about — so this calls home instead, presenting a single-use recovery token, and the server dials back over the ordinary restore path.

ReaR installs a bootloader only if this command exits `0`.

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--token` | string | | **Required.** Single-use recovery token, issued from the dashboard (Disaster Recovery → Bare Metal) |
| `--agent` | string | this host's hostname | Registered agent name being recovered |
| `--target` | string | `/mnt/local` | Directory the recreated root is mounted on |
| `--listen-port` | int | 8422 | gRPC port for the server to push the restore to |
| `--timeout` | duration | 12h | Give up if no restore completes within this window |

Plus `--server` and `--ca-cert` from the agent's [global flags](index.md).

## Examples

```bash
hitstor-agent recover --token abc123... --server backup.internal:8420
```

## Notes

- Refuses to run if `--target` is not an existing directory — this catches the case where ReaR did not actually mount the recreated root, which would otherwise make the restore silently write into the rescue system's ramdisk and vanish at reboot.
- The listener authenticates on a token generated for this recovery run; the original agent's token lived on the disk that no longer exists.
- Non-fatal per-file restore errors are reported as warnings after the restore completes rather than failing the command, since the exit code gates whether ReaR installs a bootloader — review them before rebooting.
- See [Bare-metal restore](../../user-guide/bare-metal-restore.md) for the full recovery workflow and how the recovery token is issued.
