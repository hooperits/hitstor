# Codebase Overview

This guide maps the HitStor repository for developers joining the project or updating documentation. It reflects the current tree, not historical README content.

## What HitStor Is

HitStor is an encrypted, deduplicated backup platform written in Go (`CGO_ENABLED=0`). It supports:

- **Local CLI backups** — `hitstor backup` against a local, S3, or plugin repository
- **Centralized server/agent backups** — thick-agent design over gRPC/TLS
- **Bare-metal restore** — ReaR-based recovery media for Linux hosts
- **Embedded dashboard** — monitoring, scheduling, restore, user accounts with roles and two-step verification, email notifications, and scheduled PDF reports

The core backs up filesystems (Windows and Linux); applications are backed up through installable workload plugins (`plugins/`, see [Workload Plugin Architecture](plugin-architecture.md)), starting with SQL Server. M365, Google Workspace, the built-in database integrations, and cloud-VM/VMware backup were removed before the first public release (v0.1.0) — see [Roadmap](../admin-guide/roadmap.md#permanently-removed).

## Directory Layout

```
cmd/                          # Binaries (entry points)
  hitstor/                    # Main CLI + hitstor serve
  hitstor-agent/              # Remote backup agent
  hitstor-backend-memory/     # Example storage plugin
  hitstor-notifier-log/       # Example notifier plugin
  hitstor-hook-log/           # Example hook plugin
  hitstor-scanner-builtin/    # Built-in filesystem scanner plugin (ships with core agent)
  hitstor-pluginkit/          # Builds, signs, and verifies workload plugin packages (.hpk)

plugins/                      # Workload plugins, one directory each (packaged as .hpk)
  example/                    # Reference workload plugin
  mssql/                      # SQL Server plugin (VDI; Windows only)

internal/                     # Core libraries (not importable externally)
  archiver/                   # Local backup pipeline
  restorer/                   # Restore pipeline
  repository/                 # Content-addressable store
  server/                     # gRPC BackupService, sessions, multi-repo
  agent/                      # Remote backup client + AgentService
  dashboard/                  # Embedded web UI + HTTP API
  discovery/                  # LAN agent auto-discovery
  bmr/                        # Bare-metal restore (ReaR recovery media)
  sentinel/                   # Ransomware telemetry + Gemini analysis
  config/                     # YAML config + multi-repo repositories[]
  plugin/                     # HashiCorp go-plugin manager
  pluginpkg/                  # Plugin package format, signing, and trust
  pluginmgr/                  # Agent plugin store: install, upgrade, remove
  users/                      # Dashboard accounts, roles, two-step verification
  mailer/                     # Outgoing email (SMTP relay, user/password, Microsoft 365 OAuth)
  report/                     # PDF reports
  runlog/                     # History of backup runs (for reports)
  retention/                  # Retention policies, incl. workload backup chains
  ...                         # chunker, crypto, cron, metrics, etc.

api/proto/hitstor/plugin/v1/  # Public plugin gRPC definitions
internal/proto/hitstor/v1/    # Server/agent backup protos (backup.proto, agent.proto)

packaging/                    # systemd, nfpm, NSIS, installers
docs/                         # MkDocs site (this documentation)
specs/                        # Feature specs (local only — gitignored, not part of the published repo)
```

## Binaries

| Output | Makefile target | Role |
|--------|-----------------|------|
| `bin/hitstor` | `make build` | CLI + `serve` |
| `bin/hitstor-agent` | `make agent` | Remote agent |
| `bin/hitstor-backend-memory` | `make plugin-memory` | Storage plugin example |
| `bin/hitstor-notifier-log` | `make notifier-log` | Notifier plugin example |
| `bin/hitstor-hook-log` | `make hook-log` | Hook plugin example |
| `bin/hitstor-scanner-builtin` | `make scanner-builtin` | Built-in FS scanner plugin (ships with agent) |
| `bin/hitstor-pluginkit` | `make pluginkit` | Builds, signs, and verifies `.hpk` packages |
| `dist/hitstor-plugin-<id>-<version>-<os>-<arch>.hpk` | `make plugin-packages` | Workload plugin packages (`plugins/*`) |

Build all plugins: `make plugins`

## Runtime Architecture

```mermaid
flowchart LR
    subgraph serverBox [hitstor serve]
        GRPC[gRPC :8420 BackupService]
        HTTP[HTTP :8421 Dashboard]
        GRPC --> Repo[(Repository)]
    end

    Agent[hitstor-agent daemon] -->|upload chunks| GRPC
    HTTP -->|WakeUp browse configure| AgentSvc[AgentService :8422]
    AgentSvc --- Agent

    CLI[hitstor backup] --> Repo
```

### Local backup flow

`hitstor backup` → scan (`internal/fs` or scanner plugin) → FastCDC → dedup (session + SQLite cache + index) → encrypt → pack → snapshot.

### Agent backup flow

Agent scans/chunks/compresses locally → `CheckBlobs` / `UploadChunks` over gRPC → server encrypts, packs, snapshots. Incremental detection uses **previous snapshot metadata** from `BeginSession` (no agent-side SQLite).

## Feature Maturity

| Feature | Status | Notes |
|---------|--------|-------|
| Filesystem backup (CLI + agent) | Stable | Core engine |
| Remote server + dashboard | Stable | gRPC + HTTP |
| Backup jobs + scheduling | Stable | Cron (incl. `L`, `D#N`, `DL`), interval, timezone; `job_type` is `files` or `workload` |
| Workload plugins | Available | Signed `.hpk` packages; SQL Server (VDI) full, differential, and log backups as a per-kind backup plan, with per-database results and point-in-time restore |
| Users, roles, two-step verification | Available | Personal dashboard accounts, per-agent roles, mandatory TOTP, audit log |
| Email and PDF reports | Available | SMTP or Microsoft 365 (OAuth 2.0); scheduled PDF reports |
| Bare-metal restore | Available (Linux) | ReaR-based recovery media; Windows comes later — see [Roadmap](../admin-guide/roadmap.md) |
| Agent auto-discovery | Stable | Dashboard + `/api/discover/*` |
| Ransomware Sentinel | Stable | Optional; Gemini via Settings or `HITSTOR_SENTINEL_API_KEY` |

M365, Google Workspace, database, and cloud-VM/VMware backup — along with the workload collector plugin type that served them — were permanently removed before the first public release (v0.1.0). See [Roadmap](../admin-guide/roadmap.md#permanently-removed).

## Source of Truth

When updating docs, prefer these files over README prose:

| Topic | File |
|-------|------|
| Config keys | `internal/config/config.go` (`Config`, `RepoConfig`) and `internal/config/config_registry.go` (`Registry`) |
| Dashboard HTTP routes | `internal/dashboard/dashboard.go` (`Handler()` mux) |
| Backup gRPC | `internal/proto/hitstor/v1/backup.proto` |
| Agent management gRPC | `internal/proto/hitstor/v1/agent.proto` |
| Plugin APIs | `api/proto/hitstor/plugin/v1/*.proto` |
| Makefile targets | `Makefile` |

## Conventions

- **Test-first** — table-driven tests in each package
- **Error wrapping** — `fmt.Errorf("package: action: %w", err)`
- **Logging** — `log/slog`
- **No `utils/`** — domain-specific packages only

## See Also

- [Dev Environment](dev-environment.md)
- [Build & Packaging](build-and-packaging.md)
- [Internals](../reference/internals.md)
- [gRPC API](../reference/grpc-api.md)
- [Dashboard HTTP API](../reference/dashboard-api.md)