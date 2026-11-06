# Agent Commands

Reference for all `hitstor-agent` CLI commands.

## Commands

| Command | Description |
|---------|-------------|
| [backup](backup.md) | Send a backup to the remote server |
| [ping](ping.md) | Test connectivity to the server |
| [daemon](daemon.md) | Run as a long-lived daemon with management server and backup polling |
| [show-id](show-id.md) | Display the agent token for dashboard registration |
| [plugin](plugin.md) | Install and manage workload plugins |
| [service](service.md) | Manage the agent service (Windows Service, or the systemd unit on Linux) |
| [bmr](bmr.md) | Build and inspect bare-metal recovery media (Linux) |
| [recover](recover.md) | Run in bare-metal recovery mode (invoked from rescue media) |

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--server` | string | | Remote server address (`host:port`) |
| `--api-key` | string | | Agent API key for authentication |
| `--ca-cert` | string | | Path to CA certificate for TLS verification |
| `--config` | string | `~/.config/hitstor/config.yaml` | Path to config file (the agent service uses `/etc/hitstor/agent-config.yaml`, or `%ProgramData%\HitStor\agent-config.yaml` on Windows) |
| `--json` | bool | false | Output in JSON format |
| `-q`, `--quiet` | bool | false | Suppress non-error output |
| `-v`, `--verbose` | count | | Increase log verbosity (repeat for more: `-vv`) |

The hostname sent to the server is overridden with `backup --host` or the `agent_hostname` config key.
