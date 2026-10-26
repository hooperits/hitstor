# Quick Start

Get HitStor running in about 15 minutes. This guide covers the three recommended paths for early adopters:

1. **Docker** — fastest way to run the server (developers and homelab)
2. **Linux VM installer** — production-style server on a single machine
3. **Agent only** — connect laptops and servers to an existing HitStor server

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

Edit `.env` and set a strong `HITSTOR_PASSWORD`. Optionally set `HITSTOR_DASHBOARD_PASSWORD` to protect the web UI.

### 2. Start the server

```bash
docker compose -f docker-compose.quickstart.yml up -d
```

On first start, the container automatically runs `hitstor init` and then `hitstor serve`.

### 3. Open the dashboard

Navigate to [http://localhost:8421](http://localhost:8421).

| Port | Service |
|------|---------|
| 8421 | Web dashboard |
| 8420 | gRPC (agents connect here) |

### 4. Install an agent

On each machine to back up, install the agent binary. See [Path C](#path-c-agent-only) below.

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

Default URL: `https://<server-ip>:8421` (self-signed TLS) or `http://<server-ip>:8421` depending on wizard settings.

See [Linux Quick Install](linux-installer.md) for wizard details and [Server Setup](../admin-guide/server-setup.md) for production hardening.

### 3. Install agents

Continue with [Path C](#path-c-agent-only).

---

## Path C: Agent Only

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