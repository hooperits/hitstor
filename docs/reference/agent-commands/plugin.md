# hitstor-agent plugin

Install and manage workload plugins on this agent.

## Synopsis

```
hitstor-agent plugin install <package.hpk>
hitstor-agent plugin upgrade <package.hpk> [--allow-downgrade]
hitstor-agent plugin list
hitstor-agent plugin info <id>
hitstor-agent plugin remove <id>
hitstor-agent plugin browse <id> [path]
hitstor-agent plugin test-backup <id> --select <object> --out <dir>
```

## Description

Workload plugins are signed `.hpk` packages that teach an agent to back up an
application, such as a database engine. A plugin is installed only on the
agents that need it and can be upgraded or removed without reinstalling the
agent. See [Workload Plugin Architecture](../../dev-guide/plugin-architecture.md)
for the design.

Before activating a package, the agent:

1. Verifies its signature against the trusted keys: the official HitStor key
   plus any listed in `plugin_trusted_keys`.
2. Checks that it supports this agent's OS and architecture, workload protocol,
   and version (`min_agent_version`).
3. Unpacks it into a staging directory and launches the plugin once to confirm
   it reports the id and version its manifest declares.

Only then does the new version become active. If any step fails, the
previously installed version stays active and untouched.

The agent reports its installed plugins to the server on every schedule poll,
so the server knows which workloads each agent can handle.

## Subcommands

| Subcommand | Description |
|------------|-------------|
| `install <package.hpk>` | Install a plugin that is not installed yet |
| `upgrade <package.hpk>` | Replace an installed plugin with another version. Older versions are refused unless `--allow-downgrade` is given |
| `list` | List installed plugins with version, status, and signature |
| `info <id>` | Show details of one plugin, including its capabilities and binary path |
| `remove <id>` | Uninstall a plugin. Snapshots it produced stay in the repository. Refused while a backup is using the plugin |
| `browse <id> [path]` | List the objects the plugin can back up, under `path` (top level when omitted) |
| `test-backup <id>` | Diagnostics: run a backup of `--select` objects and write the items and their metadata into the new directory `--out`. Nothing is sent to the server |

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--store` | `plugin_store_dir`, else the platform default | Plugin store directory |
| `--allow-downgrade` | `false` | `upgrade` only: allow installing an older version |
| `--setting key=value` | | `browse`, `test-backup`: plugin setting (repeatable) |
| `--select` | | `test-backup`: object to back up (repeatable) |
| `--kind` | `full` | `test-backup`: backup kind |
| `--option key=value` | | `test-backup`: backup option (repeatable) |
| `--out` | | `test-backup`: directory to create for the items |
| `--json` | `false` | Print results as JSON |

Without `--config`, the command reads the installed agent's configuration
(`/etc/hitstor/agent-config.yaml` or `C:\ProgramData\HitStor\agent-config.yaml`),
so it applies the same trust settings as the agent service.

## Plugin store

| Platform | Default directory |
|----------|-------------------|
| Linux | `/var/lib/hitstor/plugins` |
| Windows | `C:\ProgramData\HitStor\plugins` |

The store is restricted to its owner on Linux (mode `0700`), and to SYSTEM and
Administrators on Windows, because plugins run with the agent's privileges.
Installing and removing plugins therefore requires root or an elevated prompt.

## Examples

```bash
sudo hitstor-agent plugin install hitstor-plugin-example-0.1.0-linux-amd64.hpk
sudo hitstor-agent plugin list
```

Output:

```
ID       VERSION  STATUS  SIGNED    NAME
example  0.1.0    active  official  Example workload plugin
```

The `SIGNED` column shows `official` for packages signed by the HitStor
release key, `third-party` for packages signed by a key from
`plugin_trusted_keys`, and `unsigned` when `plugin_allow_unsigned` is enabled.

To trust your own signing key, for example for in-house plugins:

```yaml
# agent-config.yaml
plugin_trusted_keys:
  - "<base64 ed25519 public key>"
```

## See Also

- [Plugin Development Guide](../plugin-development.md#workload-plugins)
- [Config Reference](../config-reference.md#plugin-settings)
