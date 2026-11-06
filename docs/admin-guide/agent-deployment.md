# Agent Deployment

This guide covers installing and configuring the `hitstor-agent` binary on Linux and Windows machines.

## Linux Agent

### Install with the .run Installer (Recommended)

The `.run` installer includes an interactive wizard that configures the agent and starts the service automatically:

```bash
sudo ./hitstor-agent-X.Y.Z-linux-amd64.run
```

The wizard asks for one value, the **listen port** (gRPC port the server uses to manage the agent; default `8422`). It then installs the binary and the systemd units, writes `/etc/hitstor/agent.env`, starts the `hitstor-agent` service, and displays the **agent token** with instructions for adding the agent from the dashboard. No server address or API key is needed at install time: the server sends them when you register the agent.

### Non-Interactive Install

For automation, use `--no-wizard`; the only setting is the listen port:

```bash
sudo HITSTOR_AGENT_LISTEN_PORT=8422 \
     ./hitstor-agent-X.Y.Z-linux-amd64.run --no-wizard
```

See [Linux Quick Install](../installation/linux-installer.md) for full details.

### Install from Package

Alternatively, install the agent from a deb or rpm package:

```bash
# Debian / Ubuntu
sudo apt install ./hitstor-agent_X.Y.Z_amd64.deb

# RHEL / Fedora / AlmaLinux
sudo rpm -i hitstor-agent-X.Y.Z-1.x86_64.rpm
```

The package does not start the agent. Enable it with `sudo systemctl enable --now hitstor-agent`; the token is generated on the first start. See [Installation](../installation/index.md) for additional installation methods.

### Configure the Agent

#### Dashboard-Managed Agent (Recommended)

The agent reads its listen port from `/etc/hitstor/agent.env`:

```bash
# /etc/hitstor/agent.env
HITSTOR_AGENT_LISTEN_PORT=8422
```

When the server registers the agent from the dashboard, the agent rewrites this file with the server address and API key (`HITSTOR_AGENT_SERVER`, `HITSTOR_AGENT_API_KEY`). The agent token is stored at `/etc/hitstor/agent-token`.

To view the agent token:

```bash
sudo hitstor-agent show-id
```

#### Manual (CLI) Registration { #manual-cli-registration }

If you registered the agent on the server with `hitstor serve agent add <name>`, put the server address and the printed API key in `/etc/hitstor/agent.env` yourself, then restart the service:

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
| `HITSTOR_AGENT_SERVER` | Server address in `host:port` format (written at registration) |
| `HITSTOR_AGENT_API_KEY` | 64-character hex API key (written at registration) |
| `HITSTOR_AGENT_CA_CERT` | Path to CA certificate for TLS verification (optional; without it the agent pins the server certificate on first use) |

The daemon and the Windows service read `agent.env` themselves; one-off commands such as `hitstor-agent ping` or `backup` read these variables from the environment, from `--server`/`--api-key`/`--ca-cert`, or from `agent_server`/`agent_api_key`/`agent_ca_cert` in the file given with `--config`.

### Retrieve the Agent Token

Dashboard-managed agents generate a token on their first start. To display it:

```bash
sudo hitstor-agent show-id
```

The token is stored at `/etc/hitstor/agent-token` with mode `0600`.

### Test the Connection

Once the agent is registered, verify it can reach the server (the variables come from `agent.env`):

```bash
sudo sh -c 'set -a; . /etc/hitstor/agent.env; hitstor-agent ping'
```

Expected output:

```
Server: backup.example.com:8420
Version: X.Y.Z
Repo ID: 03b9cb33...
```

If this fails, see [Troubleshooting](troubleshooting.md) for common connection issues.

### Run a Manual Backup

Create a backup of one or more directories:

```bash
sudo sh -c 'set -a; . /etc/hitstor/agent.env; hitstor-agent backup /etc /home /var/lib'
```

The agent sends the data to the server over gRPC. Progress is displayed in the terminal.

### Daemon Mode

In daemon mode, the agent runs continuously with two functions:

1. **Management server** — listens on port 8422 for gRPC commands from the HitStor server (registration, filesystem and plugin browsing, plugin installation, restores, wake-up)
2. **Backup polling** — polls the server for its backup schedule and executes backups automatically

```bash
hitstor-agent daemon
```

If the agent is not yet registered, the daemon waits for registration from the dashboard, then runs the backup jobs created for it.

### Systemd Integration

The agent package includes two systemd units:

#### Daemon Mode (Recommended)

Enable the persistent daemon service:

```bash
sudo systemctl enable hitstor-agent
sudo systemctl start hitstor-agent
```

This starts `hitstor-agent daemon` as a long-running service. The agent listens for management commands and polls the server for schedule updates.

#### Timer Unit

The package also ships `hitstor-agent.timer` (daily at 02:00). It activates `hitstor-agent.service`, the same daemon, rather than running a one-shot backup, so it adds nothing when the daemon is enabled. For OS-scheduled one-shot backups, see [Scheduling → Systemd Timer](scheduling.md#systemd-timer-alternative).

#### Check Agent Status

```bash
sudo systemctl status hitstor-agent
sudo journalctl -u hitstor-agent -f
```

## Windows Agent

### Install the Agent

Download the Windows agent installer (`hitstor-agent-X.Y.Z-windows-amd64-setup.exe`) from the HitStor releases page and run it as an administrator. The installer:

- Asks for the **listen port** (default `8422`)
- Installs `hitstor-agent.exe` and adds its folder to the system PATH
- Writes `C:\ProgramData\HitStor\agent.env` and `agent-config.yaml`, readable only by SYSTEM and Administrators
- Adds a Windows Firewall rule for the listen port
- Registers and starts the `HitStorAgent` Windows Service ("HitStor Backup Agent"), which runs the daemon
- Shows the **agent token**

Then add the agent from the dashboard (**Agents → + Add Agent**) with its IP address and token. See [Windows Agent Installation](../installation/windows-agent.md).

### Configure the Agent

Registration from the dashboard writes the server address and API key to `C:\ProgramData\HitStor\agent.env`, which the service reads. For an agent registered with `hitstor serve agent add`, write that file yourself:

```ini
# C:\ProgramData\HitStor\agent.env
HITSTOR_AGENT_SERVER=backup.example.com:8420
HITSTOR_AGENT_API_KEY=a3f8c1d9e4b7...6f2a0e5d
# Optional: CA certificate for TLS verification
# HITSTOR_AGENT_CA_CERT=C:\ProgramData\HitStor\ca.crt
```

Then restart the service (`hitstor-agent service stop`, `hitstor-agent service start`).

### Test the Connection

Open an elevated PowerShell window and pass the server and key (the command line does not read `agent.env`):

```powershell
hitstor-agent ping --server backup.example.com:8420 --api-key KEY
```

### Run a Manual Backup

```powershell
hitstor-agent backup --server backup.example.com:8420 --api-key KEY C:\Users C:\Data
```

### Manage the Windows Service

The installer registers the service. To manage it by hand:

```powershell
hitstor-agent service install
hitstor-agent service start
hitstor-agent service stop
hitstor-agent service uninstall
```

You can also manage the service through the Windows Services console (`services.msc`).

## Deploying to Multiple Machines

For large-scale deployments, use configuration management tools (Ansible, Puppet, Chef, or similar) to:

1. Install the `hitstor-agent` package and enable the `hitstor-agent` service
2. The agent automatically generates a token on first start
3. Collect agent tokens (via `hitstor-agent show-id`) and add agents from the dashboard
4. Create backup jobs for them in the dashboard (**Backups → + New Backup Job**)

Alternatively, for legacy deployments:

1. Register each agent with `hitstor serve agent add` on the server
2. Deploy `/etc/hitstor/agent.env` with the server address and the machine-specific API key
3. Deploy the CA certificate file if you do not want the agent to pin the server certificate on first use (see [TLS Certificates](tls-certificates.md#agent-ca-certificate))
4. Enable and start the `hitstor-agent` systemd service

## Next Steps

- [Agent Management](agent-management.md) — add agents from the dashboard or CLI
- [Scheduling](scheduling.md) — configure backup schedules for deployed agents
- [Monitoring](monitoring.md) — set up alerts for agent health
- [Troubleshooting](troubleshooting.md) — resolve connection and backup issues
