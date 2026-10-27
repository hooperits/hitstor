# Build & Packaging

This page documents every Makefile target for compiling binaries and producing distributable packages. All packaging-related configuration lives under the `packaging/` directory.

## Binaries

### Server

```bash
make build                  # bin/hitstor        (linux/amd64)
make build-arm64            # bin/hitstor-arm64   (linux/arm64)
make build-windows          # bin/hitstor.exe     (windows/amd64)
```

### Agent

```bash
make agent                  # bin/hitstor-agent           (linux/amd64)
make agent-arm64            # bin/hitstor-agent-arm64      (linux/arm64)
make agent-windows          # bin/hitstor-agent.exe        (windows/amd64)
make agent-windows-arm64    # bin/hitstor-agent-arm64.exe  (windows/arm64)
```

### All Binaries

```bash
make build-all
```

Builds all server and agent binaries for every platform in a single command.

### Plugins

```bash
make plugins
```

Builds all four plugin binaries to `bin/`:

| Target | Output |
|--------|--------|
| `make plugin-memory` | `bin/hitstor-backend-memory` |
| `make notifier-log` | `bin/hitstor-notifier-log` |
| `make hook-log` | `bin/hitstor-hook-log` |
| `make scanner-builtin` | `bin/hitstor-scanner-builtin` |

## Protobuf Code Generation

Regenerate `.pb.go` and `_grpc.pb.go` files after modifying `.proto` definitions:

```bash
make proto            # All protos (backup + agent + plugin)
make proto-backup     # internal/proto/hitstor/v1/backup.proto only
make proto-agent      # internal/proto/hitstor/v1/agent.proto only
make proto-plugin     # api/proto/hitstor/plugin/v1/*.proto only
```

!!! note
    Plugin protos require the `-I api/proto` include path for cross-file imports. This is already configured in the Makefile.

## Linux Packages (.deb / .rpm)

Linux packages are built with [nfpm](https://nfpm.goreleaser.com/) using YAML configuration files in `packaging/nfpm/`.

### Makefile Targets

| Target | Prerequisite | Output |
|--------|-------------|--------|
| `make pkg-server-deb` | `bin/hitstor` | `dist/hitstor_VERSION_amd64.deb` |
| `make pkg-server-rpm` | `bin/hitstor` | `dist/hitstor-VERSION.x86_64.rpm` |
| `make pkg-agent-deb` | `bin/hitstor-agent` | `dist/hitstor-agent_VERSION_amd64.deb` |
| `make pkg-agent-rpm` | `bin/hitstor-agent` | `dist/hitstor-agent-VERSION.x86_64.rpm` |
| `make pkg-linux` | (all above) | All 4 Linux packages |

Each target automatically builds its prerequisite binary first.

### Setting the Version

Pass the `VERSION` variable to control the package version:

```bash
make pkg-server-deb VERSION=1.2.0
```

If not specified, `VERSION` defaults to `dev`.

### How It Works

nfpm reads a YAML config (e.g. `packaging/nfpm/hitstor-server.yaml`), takes the pre-built binary from `bin/`, and bundles it together with systemd units, configuration examples, and install scripts into a `.deb` or `.rpm`.

### Server Package Contents

| Source | Installed To | Description |
|--------|-------------|-------------|
| `bin/hitstor` | `/usr/bin/hitstor` | Server binary |
| `packaging/systemd/hitstor-server.service` | `/usr/lib/systemd/system/hitstor-server.service` | systemd unit |
| `packaging/systemd/hitstor-server.env.example` | `/etc/hitstor/server.env.example` | Environment file template |
| `packaging/config/server-config.yaml.example` | `/etc/hitstor/config.yaml.example` | Configuration template |

Post-install script (`packaging/scripts/server-postinstall.sh`):

- Creates `hitstor` system user and group
- Creates `/var/lib/hitstor`, `/var/log/hitstor`, `/etc/hitstor`
- Copies `server.env.example` to `server.env` if it does not exist
- Runs `systemctl daemon-reload`

### Agent Package Contents

| Source | Installed To | Description |
|--------|-------------|-------------|
| `bin/hitstor-agent` | `/usr/bin/hitstor-agent` | Agent binary |
| `packaging/systemd/hitstor-agent.service` | `/usr/lib/systemd/system/hitstor-agent.service` | systemd unit |
| `packaging/systemd/hitstor-agent.timer` | `/usr/lib/systemd/system/hitstor-agent.timer` | Daily backup timer |
| `packaging/systemd/hitstor-agent.env.example` | `/etc/hitstor/agent.env.example` | Environment file template |
| `packaging/config/agent-config.yaml.example` | `/etc/hitstor/agent-config.yaml.example` | Configuration template |

Post-install script (`packaging/scripts/agent-postinstall.sh`):

- Creates `/etc/hitstor`
- Copies `agent.env.example` to `agent.env` if it does not exist
- Runs `systemctl daemon-reload`

### nfpm Configuration Files

- `packaging/nfpm/hitstor-server.yaml` — server package definition
- `packaging/nfpm/hitstor-agent.yaml` — agent package definition

Both use `${VERSION}` and `${ARCH}` environment variables, which are set by the Makefile at build time.

## Self-Extracting Installers (.run)

Self-extracting `.run` installers bundle the binary, systemd units, configuration templates, and an interactive wizard into a single executable shell archive. The user downloads one file, runs it with `sudo`, and the wizard configures everything — passwords, ports, repository initialization, and service startup.

### Makefile Targets

| Target | Prerequisite | Output |
|--------|-------------|--------|
| `make installer-server` | `bin/hitstor` | `dist/hitstor-server-VERSION-linux-amd64.run` |
| `make installer-server-arm64` | `bin/hitstor-arm64` | `dist/hitstor-server-VERSION-linux-arm64.run` |
| `make installer-agent` | `bin/hitstor-agent` | `dist/hitstor-agent-VERSION-linux-amd64.run` |
| `make installer-agent-arm64` | `bin/hitstor-agent-arm64` | `dist/hitstor-agent-VERSION-linux-arm64.run` |
| `make installers` | (server + agent amd64) | Both amd64 installers |

```bash
make installer-server VERSION=1.2.0
make installer-agent VERSION=1.2.0
```

### How It Works

The build script (`packaging/installer/build-installer.sh`) assembles a `.run` file in four steps:

1. **Collect payload** — copies the binary, systemd units, and config examples into a temp directory
2. **Create archive** — compresses the payload into a `tar.gz`
3. **Inline libraries** — replaces `__INLINE_*__` markers in the `.sh.in` template with the contents of the corresponding `lib/*.sh` files
4. **Assemble** — substitutes `__VERSION__`, then concatenates the assembled shell header and the tar.gz payload into a single file

The resulting `.run` file has this structure:

```
┌─────────────────────────────────┐
│ Shell script header             │  ← Installer logic (v2 template
│ (with inlined library code)     │     + all libraries inlined)
├─────────────────────────────────┤
│ __PAYLOAD_BELOW__               │  ← Marker line
├─────────────────────────────────┤
│ tar.gz binary payload           │  ← Binary + configs + service files
└─────────────────────────────────┘
```

At runtime, the shell header finds the `__PAYLOAD_BELOW__` marker, extracts the tar.gz from that offset into a temp directory, and runs the installation steps.

### Installer Source Files

The v2 installer uses a modular architecture with shared libraries inlined at build time:

| File | Description |
|------|-------------|
| `packaging/installer/build-installer.sh` | Build script that assembles the `.run` file |
| `packaging/installer/server-installer-v2.sh.in` | Shell header template for the server installer |
| `packaging/installer/agent-installer-v2.sh.in` | Shell header template for the agent installer |

Shared libraries in `packaging/installer/lib/`:

| File | `__INLINE_*__` Marker | Description |
|------|----------------------|-------------|
| `lib/common.sh` | `__INLINE_COMMON__` | Flag parsing, color output, payload extraction, banner, root/systemd checks |
| `lib/wizard-server.sh` | `__INLINE_WIZARD_SERVER__` | Interactive server wizard (repo path, password, ports, TLS, dashboard password) |
| `lib/wizard-agent.sh` | `__INLINE_WIZARD_AGENT__` | Interactive agent wizard (server address, API key, backup paths) |
| `lib/verify.sh` | `__INLINE_VERIFY__` | Post-install verification (port listening, service status) |
| `lib/uninstall.sh` | `__INLINE_UNINSTALL__` | Clean removal of binaries, service files, config, data |
| `lib/upgrade.sh` | `__INLINE_UPGRADE__` | In-place upgrade with automatic rollback on failure |

### Installer Flags

Both server and agent installers accept the same flags:

| Flag | Description |
|------|-------------|
| `--no-wizard` | Skip the interactive wizard (reads config from environment variables) |
| `--uninstall` | Remove all installed files, stop and disable services |
| `--version` | Print installer version and exit |
| `--help` | Show usage information |

### Installer Behavior

The installer auto-detects the installation mode:

1. **Fresh install** — if no existing binary is found, runs the interactive wizard (or `--no-wizard` for automation)
2. **Upgrade** — if an existing binary is found at `/usr/bin/hitstor` or `/usr/bin/hitstor-agent`, runs the upgrade flow automatically
3. **Uninstall** — when `--uninstall` is passed, removes everything (with confirmation prompt)

#### Fresh Install (Wizard)

The wizard prompts for all required configuration, writes config files, initializes the repository (server only), enables and starts the service, then verifies ports are listening.

#### Fresh Install (`--no-wizard`)

For automated deployments, pass configuration via environment variables:

```bash
# Server
sudo HITSTOR_REPO=/var/lib/hitstor/repo \
     HITSTOR_PASSWORD=secret \
     ./hitstor-server-1.0.0-linux-amd64.run --no-wizard

# Agent
sudo HITSTOR_AGENT_SERVER=backup.example.com:8420 \
     HITSTOR_AGENT_API_KEY=<64-char-hex> \
     ./hitstor-agent-1.0.0-linux-amd64.run --no-wizard
```

#### Upgrade with Rollback

When the installer detects an existing installation:

1. Backs up the current binary to a temp location
2. Stops the service
3. Replaces the binary with the new version
4. Starts the service
5. Verifies the service is running
6. On failure: automatically restores the backup binary and restarts the old version

Configuration files are **never** overwritten — user edits are always preserved. Example files are updated to the latest version.

#### Uninstall

```bash
sudo ./hitstor-server-1.0.0-linux-amd64.run --uninstall
```

Stops and disables the service, removes the binary, systemd units, and optionally removes configuration and data directories (with confirmation).

### Server Installer Payload

| File in Archive | Installed To |
|----------------|-------------|
| `hitstor` | `/usr/bin/hitstor` |
| `hitstor-server.service` | `/usr/lib/systemd/system/hitstor-server.service` |
| `config.yaml.example` | `/etc/hitstor/config.yaml.example` + `/etc/hitstor/config.yaml` |
| `server.env.example` | `/etc/hitstor/server.env.example` + `/etc/hitstor/server.env` |

The server installer also creates the `hitstor` system user/group and the `/var/lib/hitstor` and `/var/log/hitstor` directories.

### Agent Installer Payload

| File in Archive | Installed To |
|----------------|-------------|
| `hitstor-agent` | `/usr/bin/hitstor-agent` |
| `hitstor-agent.service` | `/usr/lib/systemd/system/hitstor-agent.service` |
| `hitstor-agent.timer` | `/usr/lib/systemd/system/hitstor-agent.timer` |
| `agent-config.yaml.example` | `/etc/hitstor/agent-config.yaml.example` + `/etc/hitstor/agent-config.yaml` |
| `agent.env.example` | `/etc/hitstor/agent.env.example` + `/etc/hitstor/agent.env` |

## Windows Packages (.exe Installers)

Windows installers are built with [NSIS](https://nsis.sourceforge.io/) using scripts in `packaging/nsis/`.

| Target | Prerequisite | Output |
|--------|-------------|--------|
| `make pkg-server-windows` | `bin/hitstor.exe` | `dist/hitstor-VERSION-windows-amd64-setup.exe` |
| `make pkg-agent-windows` | `bin/hitstor-agent.exe` | `dist/hitstor-agent-VERSION-windows-amd64-setup.exe` |
| `make pkg-windows` | (both above) | Both Windows installers |

### NSIS Scripts

- `packaging/nsis/hitstor-server.nsi` — server installer (installs to `%PROGRAMFILES%\HitStor`, adds to PATH, optional firewall rules and Windows Service registration; supports `/S`)
- `packaging/nsis/hitstor-agent.nsi` — agent installer (installs to `%PROGRAMFILES%\HitStor Agent`, optional Windows Service registration)

## Docker Images

### Build

```bash
make docker-build VERSION=1.2.0
```

Uses the multi-stage `Dockerfile` at the project root:

1. **Builder stage** — `golang:1.26-alpine`, compiles the server binary
2. **Runtime stage** — `alpine:3.22`, copies the binary, exposes ports 8420 (gRPC) and 8421 (dashboard)

### Save for Distribution

```bash
make docker-save VERSION=1.2.0
```

Produces `dist/hitstor-VERSION-docker.tar.gz`, a gzipped tarball that can be loaded with `docker load`.

## Releases

Pushing a `vX.Y.Z` tag runs two workflows:

- `.github/workflows/release.yml` — runs `go test ./...`, then `make release-assets VERSION=X.Y.Z`, and creates the GitHub release `vX.Y.Z` with every file in `dist/` attached (release notes are generated from the commits).
- `.github/workflows/docker-publish.yml` — builds and pushes the Docker image to GHCR.

Release assets:

| File | Built by |
|------|----------|
| `hitstor-server-X.Y.Z-linux-{amd64,arm64}.run` | `make installer-server`, `make installer-server-arm64` |
| `hitstor-agent-X.Y.Z-linux-{amd64,arm64}.run` | `make installer-agent`, `make installer-agent-arm64` |
| `hitstor_X.Y.Z_amd64.deb`, `hitstor-X.Y.Z-1.x86_64.rpm` | `make pkg-server-deb`, `make pkg-server-rpm` |
| `hitstor-agent_X.Y.Z_amd64.deb`, `hitstor-agent-X.Y.Z-1.x86_64.rpm` | `make pkg-agent-deb`, `make pkg-agent-rpm` |
| `hitstor-X.Y.Z-windows-amd64-setup.exe` | `make pkg-server-windows` |
| `hitstor-agent-X.Y.Z-windows-amd64-setup.exe` | `make pkg-agent-windows` |
| `SHA256SUMS` | `make checksums` |

To cut a release:

```bash
git tag -a v1.2.0 -m "HitStor 1.2.0"
git push origin v1.2.0
```

To build the same assets locally (requires nfpm, NSIS and Pillow for the Windows icon), run `make release-assets VERSION=1.2.0`. `make release` additionally saves the Docker image tarball.

## Documentation

```bash
make docs-serve     # Local preview at http://127.0.0.1:8000/hitstor/
make docs-build     # Build to site/ (with --strict)
make docs-deploy    # Deploy to GitHub Pages
```

## Output Directory

All distributable artifacts are written to `dist/`. This directory is created automatically by the packaging targets and cleaned by:

```bash
make clean          # Removes bin/, dist/, and site/
```
