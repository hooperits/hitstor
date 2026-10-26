# hitstor-agent service

Manage the Windows Service (Windows only).

## Synopsis

```
hitstor-agent service install
hitstor-agent service start
hitstor-agent service stop
hitstor-agent service uninstall
```

## Description

Manages the HitStor Agent as a Windows Service. The service runs in daemon mode, polling the server for scheduled backups.

## Subcommands

### `service install`

Register the agent as a Windows Service. Requires administrator privileges.

### `service start`

Start the Windows Service.

### `service stop`

Stop the Windows Service.

### `service uninstall`

Remove the Windows Service registration.

## Examples

Install and start the service:

```powershell
hitstor-agent service install
hitstor-agent service start
```

Check service status:

```powershell
Get-Service HitStorAgent
```

Stop and uninstall:

```powershell
hitstor-agent service stop
hitstor-agent service uninstall
```

## Notes

- The service runs as `LocalSystem` by default.
- Configuration is read from `%AppData%\hitstor\config.yaml` or environment variables.
- Service logs are written to the Windows Event Log.
- This command is a no-op on Linux (Linux uses systemd instead).
