# Quick Start

Get HitStor running in about 15 minutes. This guide covers the four recommended paths for early adopters:

1. **Docker** — fastest way to run the server (developers and homelab)
2. **Linux VM installer** — production-style server on a single machine
3. **Windows Server installer** — server as a Windows Service
4. **Agent only** — connect laptops and servers to an existing HitStor server

Agents are always native binaries installed on each machine you want to back up. The server can run in Docker or on a VM.

## Path A: Docker (Recommended)

### Prerequisites

- Docker 20.10+ and Docker Compose v2
- 2 GB RAM and a few GB of free disk for the repository volume

### 1. Configure environment

```bash
git clone https://github.com/hooperits/hitstor.git
cd hitstor
cp .env.example .env
```

Edit `.env` and set a strong `HITSTOR_PASSWORD` (repository encryption) and `HITSTOR_DASHBOARD_PASSWORD` (web UI login). If you leave `HITSTOR_DASHBOARD_PASSWORD` empty, the server generates a new random dashboard password on every container start — see [Dashboard Password](#dashboard-password).

### 2. Start the server

```bash
docker compose -f docker-compose.quickstart.yml up -d
```

On first start, the container automatically runs `hitstor init` and then `hitstor serve`.

### 3. Open the dashboard

Navigate to [http://localhost:8421](http://localhost:8421) and log in with `HITSTOR_DASHBOARD_PASSWORD` (or the generated password, see [Dashboard Password](#dashboard-password)).

| Port | Service |
|------|---------|
| 8421 | Web dashboard |
| 8420 | gRPC (agents connect here) |

### 4. Install an agent

On each machine to back up, install the agent binary. See [Path D](#path-d-agent-only) below.

### Optional: S3 storage with MinIO

To store backups in S3-compatible storage instead of a local Docker volume:

```bash
docker compose -f docker-compose.quickstart.yml --profile s3 up -d
```

This starts HitStor with `s3:http://minio:9000/hitstor-backups` and a bundled MinIO instance. The MinIO console is at [http://localhost:9001](http://localhost:9001) (`minioadmin` / `minioadmin`).

!!! warning "Development only"
    The bundled MinIO credentials are for local testing. Do not expose this stack to the internet without changing credentials and enabling TLS.

### Pull a pre-built image

When release images are published to GHCR:

```bash
export HITSTOR_IMAGE=ghcr.io/hooperits/hitstor
export HITSTOR_VERSION=latest
docker compose -f docker-compose.quickstart.yml up -d
```

Build locally instead:

```bash
docker compose -f docker-compose.quickstart.yml build
docker compose -f docker-compose.quickstart.yml up -d
```

### Stop and remove

```bash
docker compose -f docker-compose.quickstart.yml down
# Add -v to delete repository data volumes
```

---

## Path B: Linux VM Installer

Use this when you prefer a native server install without Docker — typical for a dedicated backup VM.

### 1. Download the server installer

From [GitHub Releases](https://github.com/hooperits/hitstor/releases):

```bash
wget https://github.com/hooperits/hitstor/releases/download/vX.Y.Z/hitstor-server-X.Y.Z-linux-amd64.run
chmod +x hitstor-server-*.run
sudo ./hitstor-server-*.run
```

The interactive wizard initializes the repository, configures systemd, and starts the server.

### 2. Open the dashboard

Default URL: `https://<server-ip>:8421` (self-signed TLS) or `http://<server-ip>:8421` depending on wizard settings. Log in with the dashboard password you entered in the wizard; if you left it empty, see [Dashboard Password](#dashboard-password).

See [Linux Quick Install](linux-installer.md) for wizard details and [Server Setup](../admin-guide/server-setup.md) for production hardening.

### 3. Install agents

Continue with [Path D](#path-d-agent-only).

---

## Path C: Windows Server Installer

Use this to run the server on Windows Server 2016+ (or Windows 10 1809+) as a Windows Service.

### 1. Run the installer

Download `hitstor-X.Y.Z-windows-amd64-setup.exe` from [GitHub Releases](https://github.com/hooperits/hitstor/releases) and run it as Administrator, or install unattended from an elevated PowerShell:

```powershell
.\hitstor-X.Y.Z-windows-amd64-setup.exe /S
```

The installer copies `hitstor.exe` to `C:\Program Files\HitStor`, creates `C:\ProgramData\HitStor\config.yaml`, adds firewall rules for ports 8420 and 8421, and registers and starts the `HitStor` service. Running a newer installer the same way upgrades in place.

### 2. Get the dashboard password

The server generates a random dashboard password on first start. Read it on the server as described in [Dashboard Password](#dashboard-password).

### 3. Open the dashboard

Open `http://<server-ip>:8421`, log in, and change the password under **Settings**. Then create a repository from the **Storage** page. Until a repository exists the server runs in setup mode and serves the dashboard over plain HTTP; HTTPS (self-signed by default) is enabled after a repository is configured and the service restarts (`Restart-Service HitStor`).

See [Windows Server](windows-server.md) for service management and firewall details.

### 4. Install agents

Continue with [Path D](#path-d-agent-only).

---

## Path D: Agent Only

Install `hitstor-agent` on every machine whose data you want in the repository.

### Linux

```bash
wget https://github.com/hooperits/hitstor/releases/download/vX.Y.Z/hitstor-agent-X.Y.Z-linux-amd64.run
chmod +x hitstor-agent-*.run
sudo ./hitstor-agent-*.run
```

Choose **dashboard registration** mode. The wizard prints an agent token. In the HitStor dashboard:

1. Go to **Agents** → **Add Agent**
2. Enter the token and the agent's IP address
3. Create a backup job under **Backups**

For scripted installs:

```bash
curl -fsSL https://raw.githubusercontent.com/hooperits/hitstor/master/scripts/install-agent.sh | sudo bash
```

See [Agent Deployment](../admin-guide/agent-deployment.md) for daemon mode, database jobs, and Windows.

### Windows

Download `hitstor-agent-X.Y.Z-windows-amd64.exe` from Releases and run the installer. See [Windows Agent](windows-agent.md).

### Verify connectivity

From the agent host, the server gRPC port must be reachable (default **8420**). Agents poll the server for backup schedules; you can also trigger **Backup Now** from the dashboard.

---

## Dashboard Password

HitStor has **no default dashboard password**. The password comes from:

| Install path | Source |
|--------------|--------|
| Docker | `HITSTOR_DASHBOARD_PASSWORD` in `.env` |
| Linux VM installer | The password entered in the wizard |
| Windows installer | Generated on first start |

When the config file has no dashboard password, `hitstor serve` generates a random one at startup, saves its hash to the config file, and writes the plaintext **once** to the server log, in an entry that contains `no dashboard password configured; generated one`. You do not need dashboard access to read it — read the log on the server.

### Read the generated password

**Windows** (PowerShell as Administrator; the log is readable only by Administrators and SYSTEM):

```powershell
Select-String -Path C:\ProgramData\HitStor\logs\server.log -Pattern 'generated one' |
  Select-Object -Last 1 |
  ForEach-Object { ($_.Line | ConvertFrom-Json).fields.password }
```

**Linux:**

```bash
sudo grep 'generated one' /var/log/hitstor/server.log | tail -1
# or
sudo journalctl -u hitstor-server | grep 'generated one' | tail -1
```

**Docker:**

```bash
docker compose -f docker-compose.quickstart.yml logs hitstor | grep 'generated one' | tail -1
```

Copy and paste the value rather than typing it — generated passwords mix characters that look alike (`l`/`I`, `O`/`0`).

Log in and change the password under **Settings** right away. The log keeps the original entry, so after you change the password the logged value no longer works.

### Reset a lost password

Remove the stored password from the config file and restart the server; a new password is generated and logged as above.

**Windows:**

```powershell
Stop-Service HitStor
$cfg = 'C:\ProgramData\HitStor\config.yaml'
(Get-Content $cfg) -notmatch '^serve_dashboard_password:' | Set-Content $cfg -Encoding ascii
Start-Service HitStor
```

**Linux:**

```bash
sudo sed -i '/^serve_dashboard_password:/d' /etc/hitstor/config.yaml
sudo systemctl restart hitstor-server
```

**Docker:** set a new `HITSTOR_DASHBOARD_PASSWORD` in `.env` and recreate the container (`docker compose -f docker-compose.quickstart.yml up -d --force-recreate`).

---

## First Backup Checklist

After server and at least one agent are connected:

1. **Storage** — confirm a repository exists (Docker quickstart creates one automatically; VM wizard creates one during install)
2. **Agents** — agent shows as connected in the dashboard
3. **Backups** — create a job (`files`, `database`, etc.) and assign it to the agent
4. **Run** — click **Backup Now** or wait for the schedule
5. **Restore** — test a restore from the dashboard or CLI

## Next Steps

- [Getting Started](../user-guide/getting-started.md) — backup, restore, retention
- [Docker Deployment](docker.md) — production compose, image details, offline export
- [Agent Deployment](../admin-guide/agent-deployment.md) — production agent setup
- [Configuration](../user-guide/configuration.md) — config file reference