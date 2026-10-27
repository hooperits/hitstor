# Workload Plugin Architecture

> **Status: phases 1–3 implemented** (plugin framework, workload backup and
> restore, backup-chain retention, and the SQL Server plugin). The dashboard
> UI (phase 4) is next. The first plugin built on this architecture is Microsoft SQL Server
> backup through the Virtual Device Interface (VDI).
> It supersedes the "workload collector plugin type" entry under
> [Permanently removed](../admin-guide/roadmap.md#permanently-removed-v200) in
> the roadmap.

## Goals

- **The core stays workload-agnostic.** A HitStor agent without plugins backs
  up files exactly as today. The core never parses SQL Server (or any other
  application) data.
- **Plugins are optional, separately versioned software.** A plugin is
  installed only on the agents that need it, can be upgraded or removed without
  reinstalling the agent, and ships as its own release asset.
- **Installing a plugin adds capabilities to that agent only.** The server and
  dashboard offer a workload's jobs and restores only for agents that report the
  plugin.
- **Adding a new workload needs no core or dashboard changes.** Forms, the
  object picker, and restore planning come from the plugin.

Non-goals for the first version: agentless backup, plugins running on the
server, and third-party plugin marketplaces.

## Overview

```
            ┌──────────────── HitStor server ────────────────┐
            │  Plugin package store                           │
            │  Per-agent inventory: installed plugins and     │
            │  their capabilities                             │
            │  Jobs and restores, rendered from each          │
            │  plugin's declared schema                       │
            └───────▲──────────────────────────┬──────────────┘
      inventory     │                          │ InstallPlugin (package push)
      (heartbeat)   │                          │ jobs, restores
                    │                          ▼
┌───────────────────┴──────────── Agent ──────────────────────────────┐
│  Core pipeline: chunk · dedup · compress · encrypt · upload           │
│  Plugin manager: install, verify signature, version, remove           │
│     <data dir>/plugins/<id>/<version>/                                │
│                          │ gRPC over go-plugin, one process per job   │
│               ┌──────────▼───────────┐                                │
│               │  hitstor-plugin-<id> │──► application (e.g. SQL VDI)  │
│               └──────────────────────┘                                │
└───────────────────────────────────────────────────────────────────────┘
```

A plugin never talks to the server. It reads from and writes to the local
application; the agent owns everything that touches the repository.

## Plugin package

A plugin is distributed as a package file:
`hitstor-plugin-<id>-<version>-<os>-<arch>.hpk`. It is a zip archive with:

| Entry | Purpose |
|-------|---------|
| `plugin.yaml` | Manifest (below) |
| `bin/hitstor-plugin-<id>[.exe]` | Plugin executable |
| `LICENSE`, `README.md` | Optional documentation |
| `SIGNATURE` | Detached ed25519 signature over the canonical package digest |

### Manifest

```yaml
id: mssql                      # unique, lowercase, [a-z0-9-]
name: Microsoft SQL Server
version: 1.0.0                 # semver; independent from the agent version
type: workload
protocol_version: 1            # workload protocol version
min_agent_version: 0.2.0
platforms: [windows/amd64]
entrypoint: bin/hitstor-plugin-mssql.exe
publisher: hitstor             # selects the trusted key
```

The capabilities (backup kinds, restore options, and form schemas) are not
duplicated in the manifest. The agent asks the plugin for them through
`GetMetadata`, so the binary is the single source of truth.

### Signing and trust

Plugins run with the agent's privileges (`LocalSystem` on Windows, `root` on
Linux) and usually hold administrative credentials for the application, so
the agent verifies every package before installing it:

1. The signature must validate against a trusted publisher key. The HitStor
   release key is embedded in the agent; operators can add their own keys
   (`plugin_trusted_keys`) for in-house plugins.
2. `platforms` must include the agent's OS and architecture.
3. `min_agent_version` and `protocol_version` must be supported.

Unsigned packages are rejected unless the agent explicitly enables
`plugin_allow_unsigned` (meant for plugin development only).

## Agent plugin manager

- **Layout:** `<data dir>/plugins/<id>/<version>/` holds the unpacked package.
  On Windows the data directory is `C:\ProgramData\HitStor`. Only
  administrators and the agent service account may write there.
- **Registry:** `<data dir>/plugins/installed-plugins.json` records each
  plugin's id, active version, status (`active`, `degraded`,
  `unregistered`), and the metadata last returned by `GetMetadata`.
- **Install and upgrade:** the package is unpacked into a new version
  directory and verified, then the plugin is launched once for a `GetMetadata`
  handshake. Only then does the active version switch. A failed upgrade leaves
  the previous version active. Old versions are removed after a successful
  switch, never while a job is using them.
- **Remove:** refused while a job is running. Snapshots made by the plugin stay
  in the repository and remain restorable as plain files.
- **Inventory:** the agent reports installed plugins and their metadata to the
  server with its schedule poll, so the server always knows what each agent can
  do.
- **Runtime:** the plugin starts as a subprocess for each job and exits
  afterwards. A crash fails that job only; the agent keeps running.

### Installation channels

| Channel | How |
|---------|-----|
| Local | `hitstor-agent plugin install <file.hpk>`, plus `list`, `upgrade`, and `remove` |
| Remote | Upload the package to the server once, then install it on chosen agents from the dashboard. The server streams the package over a new `AgentService.InstallPlugin` RPC, and the agent applies the same checks as a local install. |

## Workload protocol

New service `Workload` in `api/proto/hitstor/plugin/v1/workload.proto`, served
over the existing go-plugin handshake:

| RPC | Purpose |
|-----|---------|
| `GetMetadata` | Plugin id, version, protocol version, capabilities, and form schemas for jobs, restores, and settings |
| `Configure` | Connection settings (instance, authentication) |
| `Browse` | Tree of selectable objects, e.g. instance → databases, for the dashboard object picker |
| `Backup` | Takes the selection and backup kind; streams back one or more **items**: name, bytes, and an opaque metadata blob |
| `PlanRestore` | Takes the metadata of candidate items and a target point; returns the ordered list of items to apply (e.g. full → differential → logs) |
| `Restore` | Receives the planned items' bytes in order and applies them, with the restore options from the form |

### Items in snapshots

Each item a plugin produces is stored as a virtual file in the snapshot, under
`/<plugin id>/<object path>/<item name>`. For example:

```
/mssql/<instance>/<database>/full-20260101T020000Z.bak
/mssql/<instance>/<database>/full-20260101T020000Z.meta.json
```

The agent feeds each item's stream through the same chunk, dedup, compress, and
encrypt pipeline as regular files. As a result, deduplication, encryption,
retention, verification, and replication work without changes. The metadata
blob, which the core never interprets, is stored next to the item. The snapshot
is tagged with `plugin:<id>` and the backup kind.

Because items are ordinary files, they can also be restored to a folder through
the normal file restore. This is the fallback when the plugin is unavailable.

### Restore flow

1. The operator picks an agent, an object, and a target point in the dashboard.
   The restore form comes from the plugin schema.
2. The server collects the candidate items' metadata from the plugin's
   snapshots and sends it to the agent.
3. The agent asks the plugin to `PlanRestore` and returns the plan to the
   server.
4. The server streams the planned items to the agent, which pipes them into the
   plugin's `Restore` in order.

The core only ships bytes in the order the plugin asked for. All
application-specific logic stays in the plugin, such as backup chains, LSNs,
and recovery options.

### Retention

A retention policy must not delete an item that a still-retained item depends
on (for example, a full backup that later differentials or logs need). The
agent tags workload snapshots with `plugin:<id>` and `kind:<kind>`; when a
policy keeps a snapshot whose kind is not `full`, `forget` also keeps the
earlier snapshots of the same host and plugin back to the previous full. The
rule needs no plugin-specific knowledge and errs on the side of keeping
more.

## Server and dashboard

- Jobs gain a `plugin` field next to `job_type` (`files` stays the default).
  The job's settings are stored as an opaque map validated against the plugin
  schema.
- The dashboard renders job, restore, and settings forms from the declarative
  schema in `GetMetadata`. The schema is a small JSON Schema subset with field
  types, enums, defaults, and conditional visibility. Plugins never ship HTML.
- **Agents → Plugins** lists the plugins installed on each agent, with install,
  upgrade, and remove actions. **Settings → Plugins** manages uploaded packages.

## First plugin: SQL Server (VDI)

| Aspect | Decision |
|--------|----------|
| Package | `hitstor-plugin-mssql-<version>-windows-amd64.hpk` |
| Location | `plugins/mssql/` in this repository, released as its own asset |
| Data path | VDI (`sqlvdi.dll`, `IClientVirtualDeviceSet2`) through COM in pure Go, with no cgo, plus T-SQL through `go-mssqldb` |
| Backup kinds | Full (optionally `COPY_ONLY`), differential, transaction log |
| Restore | Full chain with `NORECOVERY`/`RECOVERY`, point in time (`STOPAT`), restore under a new name with `MOVE` |
| Authentication | Windows integrated (the agent service account needs `sysadmin`) or a SQL login whose password comes from the agent environment file, never `config.yaml` |
| Metadata | LSNs (`first`, `last`, `checkpoint`, `database_backup_lsn`), backup times, and recovery model, read from `msdb.dbo.backupset` |

## Delivery phases

1. **Plugin framework:** package format, signing, agent plugin manager, local
   CLI, and inventory reporting.
2. **Workload protocol:** items as virtual files in the agent pipeline, and
   generic plugin jobs on the server.
3. **SQL Server plugin:** VDI backup and restore of every kind, plus point in
   time.
4. **Dashboard and docs:** remote install, object picker, schema-driven forms,
   and the roadmap update.

## See also

- [Plugin Development Guide](../reference/plugin-development.md): existing
  plugin types (storage, notifier, hook, scanner)
- [Codebase Overview](codebase-overview.md)
- [Roadmap](../admin-guide/roadmap.md)
