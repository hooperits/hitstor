# Docker Deployment

HitStor provides a multi-stage Dockerfile that produces a minimal Alpine-based container image. The image runs the HitStor server with gRPC and web dashboard support.

## Prerequisites

- Docker 20.10+ or a compatible container runtime
- Docker Compose v2 (for the compose-based deployment)

## Build the Image

From the repository root:

```bash
docker build -t hitstor:latest .
```

To build with a specific version tag:

```bash
docker build -t hitstor:1.0.0 \
  --build-arg VERSION=1.0.0 \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg DATE=$(date -u +%Y-%m-%d) .
```

Or use the Makefile target:

```bash
make docker-build VERSION=1.0.0
```

## Image Details

The Docker image uses a two-stage build:

1. **Builder stage** -- `golang:1.26-alpine` compiles the static binary with `CGO_ENABLED=0`.
2. **Runtime stage** -- `alpine:3.22` with only `ca-certificates` and `tzdata` installed.

The image runs as a non-root `hitstor` user.

| Property | Value |
|----------|-------|
| Base image | `alpine:3.22` |
| User | `hitstor` |
| Exposed ports | `8420` (gRPC), `8421` (dashboard) |
| Volumes | `/var/lib/hitstor` (data), `/etc/hitstor` (config) |
| Entrypoint | `/entrypoint.sh` |
| Default command | `serve` |

## Quick Start

For the full end-to-end guide (Docker server + agent installation), see [Quick Start](quickstart.md).

Run the server directly with Docker:

```bash
docker run -d \
  --name hitstor \
  -p 8420:8420 \
  -p 8421:8421 \
  -v hitstor-data:/var/lib/hitstor \
  -e HITSTOR_PASSWORD=my-secret-password \
  -e HITSTOR_REPO=/var/lib/hitstor/repo \
  hitstor:latest
```

## Docker Compose Deployment

### Quickstart (local repository)

For evaluation and homelab use:

```bash
cp .env.example .env
# edit HITSTOR_PASSWORD
docker compose -f docker-compose.quickstart.yml up -d
```

See [Quick Start](quickstart.md) for agent setup and the optional MinIO (`--profile s3`) stack.

### Production

The production deployment uses `docker-compose.prod.yml`:

### 1. Create the Configuration Directory

```bash
mkdir -p config
```

Place your `config.yaml` in this directory. You can copy and edit the example:

```bash
cp packaging/config/server-config.yaml.example config/config.yaml
editor config/config.yaml
```

### 2. Set Environment Variables

Create a `.env` file in the repository root:

```bash
HITSTOR_VERSION=latest
HITSTOR_PASSWORD=my-secret-password
```

Or export the variables in your shell before running `docker compose`.

### 3. Start the Services

```bash
docker compose -f docker-compose.prod.yml up -d
```

### 4. Verify

Check that the container is running and healthy:

```bash
docker compose -f docker-compose.prod.yml ps
```

The health check uses `wget` to poll the dashboard on port 8421 every 30 seconds.

View the logs:

```bash
docker compose -f docker-compose.prod.yml logs -f
```

## Ports

| Port | Protocol | Description |
|------|----------|-------------|
| 8420 | TCP | gRPC server -- agent connections and CLI operations |
| 8421 | TCP | Web dashboard |

## Volumes

| Mount Point | Description |
|-------------|-------------|
| `/var/lib/hitstor` | Repository data. This is where encrypted backup packs, indexes, and snapshots are stored. Use a named volume or bind mount to persistent storage. |
| `/etc/hitstor` | Configuration files. Mounted read-only from the host `config/` directory. |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `HITSTOR_PASSWORD` | Repository password used for encryption and decryption. |
| `HITSTOR_REPO` | Path to the repository inside the container (default: `/var/lib/hitstor/repo`). |
| `HITSTOR_VERSION` | Image version tag used by `docker-compose.prod.yml`. |

## Health Check

The production compose file includes a built-in health check:

```yaml
healthcheck:
  test: ["CMD", "wget", "-qO-", "http://localhost:8421/"]
  interval: 30s
  timeout: 5s
  retries: 3
```

Docker reports the container as `healthy` once the dashboard responds on port 8421. Use `docker inspect` or `docker compose ps` to check the health status.

## Saving the Image

To export the image as a tarball for offline deployment:

```bash
make docker-save VERSION=1.0.0
```

This writes `dist/hitstor-1.0.0-docker.tar.gz`. Load it on the target host with:

```bash
docker load < hitstor-1.0.0-docker.tar.gz
```

## Next Steps

- [Getting Started](../user-guide/getting-started.md) -- initialize a repository and create your first backup.
- [Configuration](../user-guide/configuration.md) -- full config file reference.
- [Upgrading](upgrading.md) -- how to upgrade a Docker deployment.
