# Linux Quick Install

The fastest way to install HitStor on a Linux server. Download a single `.run` file, execute it, and the interactive wizard configures everything — no package manager, no build tools, no dependencies.

!!! info "Requirements"
    - Linux (amd64 or arm64)
    - systemd
    - Root privileges (`sudo`)
    - Tested on Ubuntu 22.04+, Debian 12+, RHEL 9+, Fedora 39+, AlmaLinux 9+

## Install the Server

Download the installer from the [GitHub Releases](https://github.com/hooperits/hitstor/releases) page and run it:

```bash
wget https://github.com/hooperits/hitstor/releases/download/vX.Y.Z/hitstor-server-X.Y.Z-linux-amd64.run
chmod +x hitstor-server-X.Y.Z-linux-amd64.run
sudo ./hitstor-server-X.Y.Z-linux-amd64.run
```

The interactive wizard will prompt for:

1. **Data directory** — where HitStor stores data (default: `/var/lib/hitstor`)
2. **Dashboard password** — the setup password for the web UI, used until the first administrator account exists (optional; if empty, the server generates one on first start — see [Dashboard Password and First Administrator](quickstart.md#dashboard-password))
3. **gRPC port** — for agent connections (default: 8420)
4. **Dashboard port** — for the web UI (default: 8421)
5. **Server advertise address** — the address sent to agents so they can reach the server (default: the detected outbound IP and the gRPC port)

After confirming, the installer creates the system user, starts the service, and verifies everything is running. Repositories are created from the dashboard after installation; after creating the first one, restart the service (`sudo systemctl restart hitstor-server`) and create the first administrator.

### Non-Interactive Install

For automation, use `--no-wizard` with environment variables:

```bash
sudo HITSTOR_DATA_DIR=/var/lib/hitstor \
     HITSTOR_DASHBOARD_PASSWORD=dashpass \
     HITSTOR_GRPC_PORT=8420 \
     HITSTOR_DASHBOARD_PORT=8421 \
     HITSTOR_ADVERTISE_ADDR=192.0.2.10:8420 \
     ./hitstor-server-X.Y.Z-linux-amd64.run --no-wizard
```

`HITSTOR_ADVERTISE_ADDR` is optional; when unset, the installer detects the outbound IP.

### Installed Files

| Path | Description |
|------|-------------|
| `/usr/bin/hitstor` | Server binary |
| `/usr/lib/systemd/system/hitstor-server.service` | systemd service unit |
| `/etc/hitstor/config.yaml` | Server configuration |
| `/etc/hitstor/server.env` | Environment variables |
| `/var/lib/hitstor/` | Data directory (default path) |
| `/var/log/hitstor/` | Log directory |

## Install the Agent

Download the agent installer and run it:

```bash
wget https://github.com/hooperits/hitstor/releases/download/vX.Y.Z/hitstor-agent-X.Y.Z-linux-amd64.run
chmod +x hitstor-agent-X.Y.Z-linux-amd64.run
sudo ./hitstor-agent-X.Y.Z-linux-amd64.run
```

The agent is managed from the dashboard. The wizard asks only for the **listen port** (default: 8422), the port the server connects to for registration, browsing, and backup triggers. It then installs and starts the `hitstor-agent` service, which generates the agent token on first start.

After installation the installer displays the **agent token** and the next steps: in the dashboard, go to **Agents → Add Agent** and enter this machine's IP address, the listen port, and the token. Backup paths and schedules are configured from the dashboard.

### Non-Interactive Install

```bash
sudo HITSTOR_AGENT_LISTEN_PORT=8422 \
     ./hitstor-agent-X.Y.Z-linux-amd64.run --no-wizard
```

### Installed Files

| Path | Description |
|------|-------------|
| `/usr/bin/hitstor-agent` | Agent binary |
| `/usr/lib/systemd/system/hitstor-agent.service` | systemd service unit |
| `/usr/lib/systemd/system/hitstor-agent.timer` | Daily backup timer |
| `/etc/hitstor/agent-config.yaml` | Agent configuration |
| `/etc/hitstor/agent.env` | Environment variables (listen port, server address) |
| `/etc/hitstor/agent-token` | Agent token for dashboard registration (mode 0600) |

## Upgrading

Running the installer on an existing installation automatically detects the upgrade:

```bash
sudo ./hitstor-server-X.Y.Z-linux-amd64.run
```

The upgrade process:

1. Displays the current and new version for confirmation
2. Backs up the current binary to `/usr/bin/hitstor.rollback`
3. Stops the service (if running)
4. Installs the new binary and systemd units
5. Updates example config files (your working configs are **never** touched)
6. Restarts the service and verifies it is healthy
7. On verification failure: **automatically rolls back** to the previous version

Use `--no-wizard` to skip the confirmation prompt (for automation).

## Uninstall

The v2 installer includes a built-in uninstaller:

### Server

```bash
sudo ./hitstor-server-X.Y.Z-linux-amd64.run --uninstall
```

This removes the binary, systemd service, configuration, logs, and system user. You will be asked separately whether to remove the data directory (`/var/lib/hitstor`) — default is **no** to protect your backups.

### Agent

```bash
sudo ./hitstor-agent-X.Y.Z-linux-amd64.run --uninstall
```

This removes the binary, systemd service/timer, and configuration.

## Version Check

HitStor can check for newer versions via the GitHub Releases API.

### CLI

```bash
hitstor version --check
```

### Automatic (Server)

When `update_check: true` is set in `/etc/hitstor/config.yaml` (it is off unless set; it can also be turned on under **Settings → General**), the server checks for updates every 24 hours. If a newer version is available, a banner appears at the top of the web dashboard.

## ARM64 Support

ARM64 (aarch64) installers are also available. Download the `arm64` variant:

```bash
sudo ./hitstor-server-X.Y.Z-linux-arm64.run
sudo ./hitstor-agent-X.Y.Z-linux-arm64.run
```

## Installer Flags

| Flag | Description |
|------|-------------|
| `--no-wizard` | Non-interactive mode (reads from environment variables) |
| `--uninstall` | Remove HitStor and all associated files |
| `--yes` | Skip confirmation prompts (with `--uninstall`) |
| `--version` | Print installer version and exit |
| `--help` | Show help |

## Troubleshooting

### Permission Denied

The installer must be run as root:

```bash
sudo ./hitstor-server-X.Y.Z-linux-amd64.run
```

### systemd Not Found

The `.run` installer requires systemd. If your system does not have systemd, use the [Build from Source](linux-source.md) method and manage the process manually.

### Status 226/NAMESPACE

On some virtualized environments (Hyper-V VMs, containers, WSL), systemd security hardening may cause the service to fail with exit code 226. To fix this, edit the service unit and comment out the hardening directives:

```bash
sudo systemctl edit hitstor-server
```

Add the following overrides:

```ini
[Service]
ProtectSystem=
ProtectHome=
ProtectKernelTunables=
ProtectKernelModules=
ProtectControlGroups=
PrivateTmp=
NoNewPrivileges=
```

Then restart:

```bash
sudo systemctl restart hitstor-server
```

See [Troubleshooting](../admin-guide/troubleshooting.md) for more details.
