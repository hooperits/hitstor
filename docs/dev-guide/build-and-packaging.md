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

These are the built-in plugin types (storage, notifier, hook, scanner). Workload plugins such as SQL Server are packaged separately; see below.

### Workload Plugin Packages (.hpk)

Workload plugins live under `plugins/<id>/` and ship as signed `.hpk` packages (see [Workload Plugin Architecture](plugin-architecture.md)).

```bash
make pluginkit                          # bin/hitstor-pluginkit
make plugin-packages PLUGIN_UNSIGNED=1  # unsigned packages, for local development
make plugin-packages PLUGIN_KEY_FILE=mykey.key
```

`make plugin-packages` builds each entry of `WORKLOAD_PLUGIN_TARGETS` (by default `example:linux/amd64 example:windows/amd64 mssql:windows/amd64`) into `bin/plugins/` and packs it with `hitstor-pluginkit pack` into `dist/hitstor-plugin-<id>-<version>-<os>-<arch>.hpk`. The package version comes from the plugin's `plugins/<id>/plugin.yaml`, not from `VERSION`.

The signing key (a base64 ed25519 seed) is read from `PLUGIN_KEY_FILE`, or else from the `HITSTOR_PLUGIN_SIGNING_KEY` environment variable, which is how the release workflow signs. Agents reject unsigned packages unless `plugin_allow_unsigned` is set, and accept only packages signed with the official key embedded in the agent or a key listed in `plugin_trusted_keys`. To sign in-house plugins, create a key pair and check a package:

```bash
bin/hitstor-pluginkit keygen --out mykey
bin/hitstor-pluginkit verify dist/hitstor-plugin-example-0.1.0-windows-amd64.hpk
```

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
| `make pkg-server-rpm` | `bin/hitstor` | `dist/hitstor-VERSION-1.x86_64.rpm` |
| `make pkg-agent-deb` | `bin/hitstor-agent`, `bin/hitstor-scanner-builtin` | `dist/hitstor-agent_VERSION_amd64.deb` |
| `make pkg-agent-rpm` | `bin/hitstor-agent`, `bin/hitstor-scanner-builtin` | `dist/hitstor-agent-VERSION-1.x86_64.rpm` |
| `make pkg-linux` | (all above) | All 4 Linux packages |

Each target automatically builds its prerequisite binaries first.

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
| `packaging/scripts/grant-hitstor-journal-access.sh` | `/usr/lib/hitstor/grant-journal-access.sh` | Lets the `hitstor` user read the journal (dashboard **Logs** page) |
| `packaging/systemd/hitstor-server.service` | `/usr/lib/systemd/system/hitstor-server.service` | systemd unit |
| `packaging/systemd/hitstor-server.env.example` | `/etc/hitstor/server.env.example` | Environment file template |
| `packaging/config/server-config.yaml.example` | `/etc/hitstor/config.yaml.example` | Configuration template |

Post-install script (`packaging/scripts/server-postinstall.sh`):

- Creates `hitstor` system user and group
- Adds the `hitstor` user to the `systemd-journal` group (through `grant-journal-access.sh`)
- Creates `/var/lib/hitstor`, `/var/log/hitstor`, `/etc/hitstor`, `/usr/lib/hitstor/plugins`
- Copies `server.env.example` to `server.env` and `config.yaml.example` to `config.yaml` (mode `600`) if they do not exist
- Runs `systemctl daemon-reload`

### Agent Package Contents

| Source | Installed To | Description |
|--------|-------------|-------------|
| `bin/hitstor-agent` | `/usr/bin/hitstor-agent` | Agent binary |
| `bin/hitstor-scanner-builtin` | `/usr/lib/hitstor/plugins/hitstor-scanner-builtin` | Built-in filesystem scanner plugin |
| `packaging/systemd/hitstor-agent.service` | `/usr/lib/systemd/system/hitstor-agent.service` | systemd unit |
| `packaging/systemd/hitstor-agent.timer` | `/usr/lib/systemd/system/hitstor-agent.timer` | Daily backup timer |
| `packaging/systemd/hitstor-agent.env.example` | `/etc/hitstor/agent.env.example` | Environment file template |
| `packaging/config/agent-config.yaml.example` | `/etc/hitstor/agent-config.yaml.example` | Configuration template |
| (directory) | `/usr/share/hitstor/bmr`, `/var/lib/hitstor/bmr` | Bare-metal recovery hook and media (`/var/lib/hitstor/bmr` is root-only) |
| (directory) | `/var/lib/hitstor/plugins` | Installed workload plugin packages (root-only) |

Post-install script (`packaging/scripts/agent-postinstall.sh`):

- Creates `/etc/hitstor` and `/usr/lib/hitstor/plugins`
- Copies `agent.env.example` to `agent.env` and `agent-config.yaml.example` to `agent-config.yaml` (mode `600`) if they do not exist
- Runs `systemctl daemon-reload`

The packaged `hitstor-agent.timer` (daily at 02:00) activates `hitstor-agent.service`, which runs `hitstor-agent daemon`; it does not run a one-shot backup.

### nfpm Configuration Files

- `packaging/nfpm/hitstor-server.yaml` — server package definition
- `packaging/nfpm/hitstor-agent.yaml` — agent package definition

Both use `${VERSION}` and `${ARCH}` environment variables, which are set by the Makefile at build time.

## Self-Extracting Installers (.run)

Self-extracting `.run` installers bundle the binary, systemd units, configuration templates, and an interactive wizard into a single executable shell archive. The user downloads one file, runs it with `sudo`, and the wizard configures the essentials (data directory, dashboard password and ports for the server; the listen port for the agent), then enables and starts the service. Repositories are created afterwards from the dashboard.

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
| `lib/wizard-server.sh` | `__INLINE_WIZARD_SERVER__` | Interactive server wizard (data directory, dashboard password, gRPC and dashboard ports, advertise address) |
| `lib/wizard-agent.sh` | `__INLINE_WIZARD_AGENT__` | Interactive agent wizard (listen port; the agent is then added from the dashboard) |
| `lib/verify.sh` | `__INLINE_VERIFY__` | Post-install verification (port listening, service status) |
| `lib/uninstall.sh` | `__INLINE_UNINSTALL__` | Clean removal of binaries, service files, config, data |
| `lib/upgrade.sh` | `__INLINE_UPGRADE__` | In-place upgrade with automatic rollback on failure |

### Installer Flags

Both server and agent installers accept the same flags:

| Flag | Description |
|------|-------------|
| `--no-wizard` | Skip the interactive wizard (reads config from environment variables) |
| `--uninstall` | Remove all installed files, stop and disable services |
| `--yes` | Skip confirmation prompts (with `--uninstall`) |
| `--version` | Print installer version and exit |
| `--help`, `-h` | Show usage information |

### Installer Behavior

The installer auto-detects the installation mode:

1. **Fresh install** — if no existing binary is found, runs the interactive wizard (or `--no-wizard` for automation)
2. **Upgrade** — if an existing binary is found at `/usr/bin/hitstor` or `/usr/bin/hitstor-agent`, runs the upgrade flow automatically
3. **Uninstall** — when `--uninstall` is passed, removes everything (with confirmation prompt)

#### Fresh Install (Wizard)

The wizard prompts for the configuration, writes the config files, enables and starts the service, then verifies the ports are listening. It does not create a repository: the server starts without one and the dashboard offers to create or connect one.

#### Fresh Install (`--no-wizard`)

For automated deployments, pass configuration via environment variables:

```bash
# Server (all optional; defaults shown)
sudo HITSTOR_DATA_DIR=/var/lib/hitstor \
     HITSTOR_GRPC_PORT=8420 \
     HITSTOR_DASHBOARD_PORT=8421 \
     HITSTOR_ADVERTISE_ADDR=backup.example.com:8420 \
     HITSTOR_DASHBOARD_PASSWORD='choose-a-password' \
     ./hitstor-server-1.0.0-linux-amd64.run --no-wizard

# Agent (default listen port 8422)
sudo HITSTOR_AGENT_LISTEN_PORT=8422 \
     ./hitstor-agent-1.0.0-linux-amd64.run --no-wizard
```

`HITSTOR_ADVERTISE_ADDR` defaults to the detected outbound IP and the gRPC port. Without `HITSTOR_DASHBOARD_PASSWORD`, the server generates a dashboard password at first start and prints it once in the log; it is used to create the first administrator. The agent is then added from the dashboard with its address and token (`hitstor-agent show-id`).

#### Upgrade with Rollback

When the installer detects an existing installation:

1. Backs up the current binary (to `/usr/bin/hitstor.rollback` or `/usr/bin/hitstor-agent.rollback`)
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
| `config.yaml.example` | `/etc/hitstor/config.yaml.example` |
| `server.env.example` | `/etc/hitstor/server.env.example` |

`/etc/hitstor/config.yaml` (ports, advertise address, dashboard password) and `/etc/hitstor/server.env` are written from the wizard answers. The server installer also creates the `hitstor` system user/group and the `/var/lib/hitstor` and `/var/log/hitstor` directories.

### Agent Installer Payload

| File in Archive | Installed To |
|----------------|-------------|
| `hitstor-agent` | `/usr/bin/hitstor-agent` |
| `hitstor-agent.service` | `/usr/lib/systemd/system/hitstor-agent.service` |
| `hitstor-agent.timer` | `/usr/lib/systemd/system/hitstor-agent.timer` |
| `agent-config.yaml.example` | `/etc/hitstor/agent-config.yaml.example` + `/etc/hitstor/agent-config.yaml` |
| `agent.env.example` | `/etc/hitstor/agent.env.example` |

`/etc/hitstor/agent.env` is written with `HITSTOR_AGENT_LISTEN_PORT`; if it already holds a registration (`HITSTOR_AGENT_SERVER`), only the listen port is updated.

## Windows Packages (.exe Installers)

Windows installers are built with [NSIS](https://nsis.sourceforge.io/) using scripts in `packaging/nsis/`.

| Target | Prerequisite | Output |
|--------|-------------|--------|
| `make pkg-server-windows` | `bin/hitstor.exe` | `dist/hitstor-VERSION-windows-amd64-setup.exe` |
| `make pkg-agent-windows` | `bin/hitstor-agent.exe` | `dist/hitstor-agent-VERSION-windows-amd64-setup.exe` |
| `make pkg-windows` | (both above) | Both Windows installers |

Both targets first run `make render-windows-icon`, which regenerates `packaging/nsis/hitstor.ico` from `assets/icon.png` (needs Python with Pillow).

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

### Quickstart Stack

```bash
make docker-quickstart-up     # needs .env (copy .env.example and set HITSTOR_PASSWORD)
make docker-quickstart-down
```

These wrap `docker compose -f docker-compose.quickstart.yml`. Published images are built by `.github/workflows/docker-publish.yml`, not by these targets.

## Releases

Pushing a `vX.Y.Z` tag builds every release asset in CI and publishes the GitHub release. See [Release Process](release-process.md).

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
