# Agent Commands

Reference for all `hitstor-agent` CLI commands.

## Commands

| Command | Description |
|---------|-------------|
| [backup](backup.md) | Send a backup to the remote server |
| [ping](ping.md) | Test connectivity to the server |
| [daemon](daemon.md) | Run as a long-lived daemon with management server and backup polling |
| [show-id](show-id.md) | Display the agent token for dashboard registration |
| [service](service.md) | Manage the Windows Service (Windows only) |

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--server` | string | | Remote server address (`host:port`) |
| `--api-key` | string | | Agent API key for authentication |
| `--ca-cert` | string | | Path to CA certificate for TLS verification |
| `--hostname` | string | system hostname | Override hostname sent to server |
| `--config` | string | | Path to config file |
