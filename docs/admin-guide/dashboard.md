# Web Dashboard

HitStor includes a built-in web dashboard for monitoring backup infrastructure, managing agents, browsing remote filesystems, and configuring schedules. The dashboard is served by the same `hitstor serve` process that handles gRPC connections.

## Accessing the Dashboard

The dashboard is available at:

```
http://SERVER_IP:8421
```

Replace `SERVER_IP` with the hostname or IP address of the HitStor server. The default dashboard port is `8421`, configurable with `--dashboard-port`.

## Password Protection

The dashboard always requires a password. Set it with the `serve_dashboard_password` configuration key:

```yaml
# /etc/hitstor/config.yaml
serve_dashboard_password: "DASHBOARD_PASSWORD"
```

If no password is configured, `hitstor serve` generates a random one at startup, stores its hash in the config file and prints the plaintext once in the server log (look for `no dashboard password configured`). Log in with it and change it under **Settings**. If the server has no config file, a new password is generated on every restart.

The dashboard presents a login page. Authentication uses session cookies with the following security properties:

- `HttpOnly` — cookies are not accessible to JavaScript
- `SameSite=Strict` — cookies are not sent on cross-site requests

Sessions expire when the browser is closed.

## Update Notifications

When `update_check: true` is set in the configuration (the default), the server checks for new releases via the GitHub Releases API every 24 hours. If a newer version is available, a notification banner appears at the top of every dashboard page:

> **HitStor v1.1.0 is available (current: v1.0.0).** [View release](#)

The banner links to the release page on GitHub. To disable automatic checks, set `update_check: false` in `/etc/hitstor/config.yaml`. You can also check manually from the CLI with `hitstor version --check`.

## Layout

The dashboard uses a fixed header bar with the HitStor logo and version, a sidebar with icon navigation, and a main content area. The sidebar groups pages into two sections:

- **Monitoring** — Overview, Backups, Agents
- **Management** — Storage, Settings

## Dashboard Pages

### Overview

The main page displays a summary of the backup infrastructure:

- Total number of agents, snapshots, and last backup time
- Agent health breakdown (OK, stale, never backed up)
- Quick links to Agents, Snapshots, and Storage pages

### Agents

Lists all registered agents with their current status, type (managed or CLI), last backup time, and creation date. Click an agent name to view its detail page.

**Adding an agent from the dashboard:**

1. Use **Network Discovery** to scan subnets, or **Manual Entry** for a specific `host:port`
2. Enter the agent name, `agent_addr`, and token (manual entry)
3. Confirm registration

The server connects to the agent over AgentService (port 8422), verifies the token, and sends API key + server address. See [Agent Management](agent-management.md) and [Agent Discovery](agent-discovery.md).

### Agent Detail

Shows detailed information for a single agent:

- **Target Repository** — select which repository this agent backs up to
- **Backup Schedule** — enable/disable scheduled backups, set interval in hours
- **Backup Paths** (managed agents only) — filesystem tree browser for selecting directories to back up
- **Snapshot History** — table with snapshot IDs, timestamps, file counts, sizes, and durations

#### Filesystem Tree Browser

For dashboard-managed agents, the Backup Paths section displays a live filesystem tree from the agent machine. The server connects to the agent over gRPC to list directories in real time.

- Click the arrow icon to expand a directory and load its contents
- Check the directories you want to include in backups
- Click **Save Backup Paths** to send the configuration to the agent

Directories that are not readable by the agent process appear dimmed.

### Snapshots

Lists all snapshots across all agents. Supports filtering by agent name. Each entry shows the snapshot ID, agent, timestamp, paths, file count, and total size.

### Storage

The Storage page provides full repository management:

- **Repository list** — view all configured repositories (local and S3) with status badges, disk usage bars, snapshot counts, and pack/index statistics
- **Create repository** — initialize a new local or S3-compatible repository directly from the dashboard. Local repositories show available disk space and detect existing repos at the given path
- **Connect existing repository** — if a repository already exists at a path (for example, after reinstalling), the dashboard detects it and connects instead of reinitializing
- **Disconnect repository** — remove a repository from the configuration (does not delete data on disk)
- **Integrity check** — run `hitstor check` on a repository with real-time progress via Server-Sent Events. Results show pass/fail status and any errors found
- **Prune** — free space by removing unreferenced data, with confirmation dialog and live progress. Shows packs deleted, bytes freed, and indexes rewritten
- **Growth chart** — visualize storage growth over time with configurable date range (30 days, 90 days, 6 months, 1 year, all) and granularity (daily, weekly)
- **Snapshot detail** — browse snapshot contents and restore individual files or whole paths. See [Restore](../user-guide/restore.md).

### Settings

Central hub for server configuration: general performance, TLS, network ports, **Integrations** (Gemini), and notifications. See [Server Settings](settings.md).

## Schedule Management

The dashboard allows administrators to configure per-agent backup schedules without logging into the server command line.

From the Agent Detail page:

1. Toggle scheduling on or off for the agent
2. Set the backup interval in hours (for example, every 24 hours)
3. Click **Save Schedule**

Agents running in daemon mode (`hitstor-agent daemon`) poll the server for schedule updates and execute backups accordingly.

## JSON API

The dashboard exposes a JSON API for programmatic access. All endpoints return JSON responses.

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/agents` | List all registered agents with status |
| `POST` | `/api/agents` | Add a dashboard-managed agent (name, address, token) |
| `DELETE` | `/api/agents/{name}` | Remove an agent |
| `POST` | `/api/agents/{name}/schedule` | Update agent backup schedule |
| `POST` | `/api/agents/{name}/repo` | Set agent target repository |
| `POST` | `/api/agents/{name}/browse` | Browse agent filesystem (managed agents) |
| `POST` | `/api/agents/{name}/configure` | Configure agent backup paths (managed agents) |
| `GET` | `/api/agents/{name}/info` | Query agent info (managed agents) |
| `GET` | `/api/snapshots` | List all snapshots (supports `?agent=NAME` filter) |
| `GET` | `/api/stats` | Repository statistics (storage, deduplication, counts) |
| `GET` | `/api/repos` | List all configured repositories |
| `POST` | `/api/repos` | Create a new repository (local or S3) |
| `DELETE` | `/api/repos/{name}` | Disconnect a repository |
| `POST` | `/api/repos/test-s3` | Test S3 endpoint connectivity |
| `POST` | `/api/repos/detect` | Detect existing repository at a path |
| `GET` | `/api/repos/{name}/growth` | Storage growth data for charting |
| `GET` | `/api/repos/{name}/sessions` | Active backup sessions for a repository |
| `POST` | `/api/repos/{name}/check` | Start integrity check (returns operation ID) |
| `POST` | `/api/repos/{name}/prune` | Start prune operation (returns operation ID) |
| `GET` | `/api/operations/{id}/events` | SSE stream for operation progress |
| `GET` | `/api/disk-space` | Available disk space at a given path |

### Example: Add an Agent

```bash
curl -X POST http://localhost:8421/api/agents \
  -H 'Content-Type: application/json' \
  -d '{"name":"web-01","address":"192.168.1.50:8422","token":"a3f8c1d9e4b7..."}'
```

### Example: Browse Agent Filesystem

```bash
curl -X POST http://localhost:8421/api/agents/web-01/browse \
  -H 'Content-Type: application/json' \
  -d '{"path":"/home"}'
```

### Example: List Agents

```bash
curl -s http://localhost:8421/api/agents | jq .
```

If dashboard password protection is enabled, include the session cookie or authenticate first:

```bash
# Authenticate and save cookie
curl -c cookies.txt -d "password=DASHBOARD_PASSWORD" http://localhost:8421/login

# Use cookie for API requests
curl -b cookies.txt -s http://localhost:8421/api/stats | jq .
```

## Prometheus Metrics

The dashboard exposes a Prometheus-compatible metrics endpoint:

```
http://SERVER_IP:8421/metrics
```

This endpoint is **unauthenticated** (accessible without the dashboard password) to allow Prometheus to scrape metrics without credential management. If this is a concern, restrict access at the network level.

See [Monitoring](monitoring.md) for the full list of metrics and Prometheus configuration.

## Next Steps

- [Agent Management](agent-management.md) — register and manage agents
- [Monitoring](monitoring.md) — configure Prometheus scraping and alerting
- [Scheduling](scheduling.md) — detailed scheduling configuration options
- [Security](security.md) — dashboard authentication and hardening
