# hitstor-agent service

Manage the HitStor agent as a system service: a Windows Service on Windows, the systemd unit on Linux.

## Synopsis

Windows:

```
hitstor-agent service install [--name HitStorAgent] [--display "HitStor Backup Agent"] [--config <file>]
hitstor-agent service start|stop|uninstall [--name HitStorAgent]
```

Linux:

```
hitstor-agent service install|uninstall|start|stop|restart|status|enable|disable [--name hitstor-agent]
```

## Description

On Windows, the command registers and controls the `HitStorAgent` Windows Service. The service runs the agent in [daemon mode](daemon.md): it listens on the agent's listen port for the server and polls the server for its backup jobs.

On Linux, the command drives the systemd unit installed by the agent package or installer (`hitstor-agent.service`, which runs `hitstor-agent daemon`) through `systemctl`. It does not create the unit file.

## Subcommands

### `service install`

Windows: register the agent as a Windows Service with automatic start. Requires administrator privileges. `--config`, `--server`, `--api-key`, and `--ca-cert` given here are passed to the service.

Linux: `systemctl enable --now` the unit.

### `service start` / `service stop`

Start or stop the service.

### `service uninstall`

Windows: remove the Windows Service registration. Linux: stop and disable the unit.

### `service restart`, `service status`, `service enable`, `service disable` (Linux)

Restart the unit, show its status, or enable or disable it at boot.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | `HitStorAgent` (Windows), `hitstor-agent` (Linux) | Service or unit name |
| `--display` | `HitStor Backup Agent` | Windows only, `install`: display name in the Services console |

## Examples

Install and start the service on Windows (elevated prompt):

```powershell
hitstor-agent service install --config "$env:ProgramData\HitStor\agent-config.yaml"
hitstor-agent service start
Get-Service HitStorAgent
```

Stop and uninstall:

```powershell
hitstor-agent service stop
hitstor-agent service uninstall
```

On Linux:

```bash
sudo hitstor-agent service status
sudo hitstor-agent service restart
```

## Notes

- The Windows Service runs as `LocalSystem`. It reads `%ProgramData%\HitStor\agent.env` and the file given with `--config` at install time; the Windows installer passes `%ProgramData%\HitStor\agent-config.yaml`.
- The agent logs to `%ProgramData%\HitStor\logs\agent.log` on Windows and `/var/log/hitstor/agent.log` on Linux (see [daemon](daemon.md)).
- The Windows installer installs and starts the service for you; see [Windows Agent](../../installation/windows-agent.md).
