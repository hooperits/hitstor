# Plugins

HitStor extends functionality through external plugin binaries using the [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin) framework. Plugins communicate with HitStor over **gRPC subprocesses**, not stdin/stdout.

For plugin development details, see [Plugin Development](../reference/plugin-development.md).

## Plugin Discovery

HitStor searches for plugin binaries in:

1. **`plugin_dir`** config path (checked first)
2. **`$PATH`**

```yaml
plugin_dir: /usr/local/lib/hitstor/plugins
```

## Binary Naming Convention

| Plugin Type | Pattern | Example |
|-------------|---------|---------|
| Storage backend | `hitstor-backend-NAME` | `hitstor-backend-memory` |
| Notifier | `hitstor-notifier-NAME` | `hitstor-notifier-slack` |
| Hook | `hitstor-hook-NAME` | `hitstor-hook-log` |

## Built-in Plugin Binaries

The **filesystem scanner** (`hitstor-scanner-builtin`) is the core built-in plugin. The agent package installs it in `/usr/lib/hitstor/plugins/`. It provides Linux and Windows filesystem backup; when its binary is not found, `hitstor` and `hitstor-agent` use the same walker built into them.

Other plugins (storage backend, notifier, hook) are optional and extend how backups are stored, alerted on, or triggered around.

Build all plugins:

```bash
make plugins
```

| Binary | Purpose | Installed by default? |
|--------|---------|-----------------------|
| `hitstor-scanner-builtin` | Core filesystem scanner (Linux/Windows) | Yes (core package) |
| `hitstor-backend-memory` | In-memory storage (testing) | No |
| `hitstor-notifier-log` | Log notifier example | No |
| `hitstor-hook-log` | Hook example | No |

## Storage Backend Plugins

```console
hitstor init --repo plugin:memory --password "secret"
hitstor backup --repo plugin:memory --password "secret" /data
```

## Notifier Plugins

```yaml
alert_notifier_plugins:
  - slack
plugin_dir: /usr/local/lib/hitstor/plugins
```

Also configure `alert_webhook_url` for the built-in webhook notifier. Multiple notifiers can run in parallel.

## Hook Plugins

```yaml
hook_plugins:
  - log
```

Hooks fire at `PreBackup` and `PostBackup`. Pre-backup errors abort the backup; post-backup errors are logged only.

## Filesystem Scanner Plugins

Only **one** scanner plugin is active at a time:

```yaml
fs_scanner_plugin: builtin   # default walker
```

Use `builtin` for the default walker, or the name of a custom filesystem scanner
plugin you have installed. With `fs_scanner_plugin` unset, HitStor uses
`hitstor-scanner-builtin` when it finds it (in `plugin_dir` or `$PATH`) and its
in-process walker otherwise; a name set explicitly, `builtin` included, must be
found or the backup fails.

## Installing Plugins

```console
cp bin/hitstor-scanner-myfs /usr/local/lib/hitstor/plugins/
chmod +x /usr/local/lib/hitstor/plugins/hitstor-scanner-myfs
```

Storage backend and scanner plugins are started when a command needs them. Notifier and hook plugins used by the server are loaded when `hitstor serve` starts, so restart the server after changing `alert_notifier_plugins` or `hook_plugins`.

## Workload Plugins

Workload plugins teach an agent to back up an application, such as a database engine, instead of files. They are different from the plugins above: each is a signed `.hpk` package, installed per agent with [`hitstor-agent plugin`](../reference/agent-commands/plugin.md) or from the dashboard (**Settings → Plugins**, then the agent's page), and the agent verifies the signature before activating it. Packages are released as `hitstor-plugin-<id>-<version>-<os>-<arch>.hpk`.

| Plugin | Package | Workload |
|--------|---------|----------|
| `mssql` | `hitstor-plugin-mssql-<version>-windows-amd64.hpk` | Microsoft SQL Server, through the Virtual Device Interface. See [SQL Server](sql-server.md). |
| `example` | `hitstor-plugin-example-<version>-<os>-amd64.hpk` (Linux and Windows) | Reference plugin for developers; it backs up nothing useful. |

Jobs that use a workload plugin are created in the dashboard with a **backup plan**: a schedule for each backup kind the plugin supports (for SQL Server: full, differential, and transaction log). The job page shows the result of each database (object) separately.

## See Also

- [Plugin Development](../reference/plugin-development.md)