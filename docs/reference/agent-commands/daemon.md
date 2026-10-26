# hitstor-agent daemon

Run as a long-lived daemon with management server and backup polling.

## Synopsis

```
hitstor-agent daemon [flags]
```

## Description

Runs the agent as a long-lived process with two functions:

1. **Management server** — listens on a gRPC port (default: 8422) for commands from the HitStor server, including registration, filesystem browsing, and configuration
2. **Backup polling** — periodically polls the server for a backup schedule and runs backups when due

The daemon starts in one of two modes:

- **Unconfigured** (new install): Only runs the management server. Logs "waiting for registration from dashboard." When the server registers the agent via the dashboard, the daemon transitions to configured mode automatically.
- **Configured** (after registration or legacy install): Runs both the management server and the backup polling loop.

Graceful shutdown on SIGINT/SIGTERM (Linux) or service stop (Windows).

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--listen-port` | int | 8422 | gRPC listen port for server management connections |
| `--poll-interval` | duration | 5m | How often to check the server for schedule updates |

## Examples

Run in foreground (dashboard-managed):

```bash
hitstor-agent daemon --listen-port 8422
```

Run in foreground (legacy mode):

```bash
hitstor-agent daemon --server backup.internal:8420 --api-key YOUR_KEY
```

Run via systemd:

```bash
sudo systemctl enable --now hitstor-agent
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `HITSTOR_AGENT_LISTEN_PORT` | Override the default listen port (8422) |

## Notes

- On first start, the daemon generates an agent token at `/etc/hitstor/agent-token` if one does not exist
- The management server uses self-signed TLS (auto-generated)
- If no schedule is configured, the daemon polls every 5 minutes and logs a message
- Alternative to daemon mode: use systemd timers with `hitstor-agent backup` for cron-style scheduling
