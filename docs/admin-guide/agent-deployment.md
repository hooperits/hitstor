# Agent Deployment

This guide covers installing and configuring the `hitstor-agent` binary on Linux and Windows machines.

## Linux Agent

### Install with the .run Installer (Recommended)

The `.run` installer includes an interactive wizard that configures the agent and starts the service automatically:

```bash
sudo ./hitstor-agent-X.Y.Z-linux-amd64.run
```

The wizard offers two setup modes:

#### Dashboard-Managed (Recommended)

The agent generates a token and listens for registration from the server dashboard. No server address or API key is needed at install time.

The wizard prompts for:

1. **Setup mode** — select `1` for dashboard-managed
2. **Listen port** — gRPC port for server connections (default: 8422)
3. **Backup paths** — optional, can be configured later from the dashboard

After installation, the wizard displays the **agent token** and instructions for adding the agent from the dashboard.

#### Legacy (Manual)

The agent is configured with a server address and API key at install time. Use this if you registered the agent via `hitstor serve agent add`.

The wizard prompts for:

1. **Setup mode** — select `2` for legacy
2. **Server address** — `host:port` of the HitStor server
3. **API key** — 64-character hex key from `hitstor serve agent add <name>` (masked input)
4. **Connection test** — verifies connectivity before proceeding
5. **Backup paths** — comma-separated directories to back up (default: `/etc,/home`)

### Non-Interactive Install

For automation, use `--no-wizard` with environment variables.

**Dashboard-managed mode** (default when no server/key env vars are set):

```bash
sudo HITSTOR_AGENT_LISTEN_PORT=8422 \
     ./hitstor-agent-X.Y.Z-linux-amd64.run --no-wizard
```

**Legacy mode** (triggered by setting server and API key):

```bash
sudo HITSTOR_AGENT_SERVER=backup.example.com:8420 \
     HITSTOR_AGENT_API_KEY=<64-char-hex-key> \
     HITSTOR_AGENT_PATHS=/etc,/home \
     ./hitstor-agent-X.Y.Z-linux-amd64.run --no-wizard
```

See [Linux Quick Install](../installation/linux-installer.md) for full details.

### Install from Package

Alternatively, install the agent from a deb or rpm package:

```bash
# Debian / Ubuntu
sudo apt install ./hitstor-agent_1.0.0_amd64.deb

# RHEL / Fedora / AlmaLinux
sudo rpm -i hitstor-agent-1.0.0.x86_64.rpm
```

See [Installation](../installation/index.md) for additional installation methods.

### Configure the Agent

Configuration depends on the setup mode.

#### Dashboard-Managed Agent

The agent reads its listen port from `/etc/hitstor/agent.env`:

```bash
# /etc/hitstor/agent.env
HITSTOR_AGENT_LISTEN_PORT=8422
```

After the server registers the agent via the dashboard, it writes the server address and API key to this file automatically. The agent token is stored at `/etc/hitstor/agent-token`.

To view the agent token:

```bash
sudo hitstor-agent show-id
```

#### Legacy Agent

The agent reads its server address and API key from `/etc/hitstor/agent.env`:

```bash
# /etc/hitstor/agent.env
HITSTOR_AGENT_SERVER=backup.example.com:8420
HITSTOR_AGENT_API_KEY=a3f8c1d9e4b7...6f2a0e5d
```

Set file permissions so only root can read the key:

```bash
sudo chmod 0600 /etc/hitstor/agent.env
sudo chown root:root /etc/hitstor/agent.env
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `HITSTOR_AGENT_LISTEN_PORT` | gRPC listen port for dashboard management (default: 8422) |
| `HITSTOR_AGENT_SERVER` | Server address in `host:port` format (legacy mode) |
| `HITSTOR_AGENT_API_KEY` | 64-character hex API key (legacy mode) |
| `HITSTOR_AGENT_CA_CERT` | Path to CA certificate for TLS verification |

### Retrieve the Agent Token

Dashboard-managed agents generate a token during installation. To display it:

```bash
sudo hitstor-agent show-id
```

The token is stored at `/etc/hitstor/agent-token` with mode `0600`.

### Test the Connection (Legacy Mode)

For legacy agents, verify the agent can reach the server:

```bash
hitstor-agent ping
```

Expected output:

```
Server: backup.example.com:8420
Status: connected
Agent:  web-server-01
```

If this fails, see [Troubleshooting](troubleshooting.md) for common connection issues.

### Run a Manual Backup

Create a backup of one or more directories:

```bash
hitstor-agent backup /etc /home /var/lib
```

The agent sends the data to the server over gRPC. Progress is displayed in the terminal.

### Daemon Mode

In daemon mode, the agent runs continuously with two functions:

1. **Management server** — listens on port 8422 for gRPC commands from the HitStor server (registration, filesystem browsing, configuration)
2. **Backup polling** — polls the server for its backup schedule and executes backups automatically

```bash
hitstor-agent daemon
```

If the agent is not yet registered (dashboard-managed mode), the daemon waits for registration from the dashboard and begins backing up once registered.

### Systemd Integration

The agent package includes two systemd units:

#### Daemon Mode (Recommended)

Enable the persistent daemon service:

```bash
sudo systemctl enable hitstor-agent
sudo systemctl start hitstor-agent
```

This starts `hitstor-agent daemon` as a long-running service. The agent listens for management commands and polls the server for schedule updates.

#### Timer Mode

For cron-style execution, use the systemd timer instead:

```bash
sudo systemctl enable hitstor-agent.timer
sudo systemctl start hitstor-agent.timer
```

The timer triggers `hitstor-agent backup` at the configured interval. Edit the timer to customize the schedule:

```bash
sudo systemctl edit hitstor-agent.timer
```

```ini
[Timer]
OnCalendar=*-*-* 02:00:00
```

See [Scheduling](scheduling.md) for more details on timer configuration.

#### Check Agent Status

```bash
sudo systemctl status hitstor-agent
sudo journalctl -u hitstor-agent -f
```

## Windows Agent

### Install the Agent

Download the Windows installer from the HitStor releases page and run it. The installer places the `hitstor-agent.exe` binary in `C:\Program Files\HitStor\` and adds it to the system PATH.

### Configure the Agent

Create or edit the configuration file at `C:\ProgramData\HitStor\config.yaml`:

```yaml
# C:\ProgramData\HitStor\config.yaml
agent_server: "backup.example.com:8420"
agent_api_key: "a3f8c1d9e4b7...6f2a0e5d"

# Optional: CA certificate for TLS verification
# agent_ca_cert: "C:\\ProgramData\\HitStor\\ca.crt"
```

### Test the Connection

Open a Command Prompt or PowerShell window:

```powershell
hitstor-agent ping
```

### Run a Manual Backup

```powershell
hitstor-agent backup C:\Users C:\Data
```

### Register as a Windows Service

To run the agent as a background Windows Service:

```powershell
hitstor-agent service install
hitstor-agent service start
```

This registers `hitstor-agent daemon` as a Windows Service that starts automatically on boot.

Manage the service:

```powershell
hitstor-agent service stop
hitstor-agent service uninstall
```

You can also manage the service through the Windows Services console (`services.msc`).

## Deploying to Multiple Machines

For large-scale deployments, use configuration management tools (Ansible, Puppet, Chef, or similar) to:

1. Install the `hitstor-agent` package
2. The agent automatically generates a token on first start
3. Collect agent tokens (via `hitstor-agent show-id`) and add agents from the dashboard
4. Configure backup paths from the dashboard's filesystem browser

Alternatively, for legacy deployments:

1. Register each agent with `hitstor serve agent add` on the server
2. Deploy `/etc/hitstor/agent.env` with the machine-specific API key
3. Deploy the CA certificate file if using self-signed certificates
4. Enable and start the `hitstor-agent` systemd service

## Next Steps

- [Agent Management](agent-management.md) — add agents from the dashboard or CLI
- [Scheduling](scheduling.md) — configure backup schedules for deployed agents
- [Monitoring](monitoring.md) — set up alerts for agent health
- [Troubleshooting](troubleshooting.md) — resolve connection and backup issues
