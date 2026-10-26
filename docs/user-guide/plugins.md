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

The **filesystem scanner** (`hitstor-scanner-builtin`) is the core built-in plugin and is installed with the agent/server packages into the standard plugin directory. It provides Linux and Windows filesystem backup.

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
plugin you have installed.

## Installing Plugins

```console
cp bin/hitstor-scanner-myfs /usr/local/lib/hitstor/plugins/
chmod +x /usr/local/lib/hitstor/plugins/hitstor-scanner-myfs
```

No restart is required — plugins are spawned on demand.

## See Also

- [Plugin Development](../reference/plugin-development.md)