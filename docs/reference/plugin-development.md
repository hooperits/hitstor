# Plugin Development Guide

HitStor supports five plugin types via the [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin) framework. Plugins run as separate processes and communicate with HitStor over gRPC.

## Plugin Types

| Type | Binary Pattern | Config Key | Proto File |
|------|---------------|------------|------------|
| Storage Backend | `hitstor-backend-<name>` | `repo: plugin:<name>/args` | `api/proto/hitstor/plugin/v1/storage.proto` |
| Notifier | `hitstor-notifier-<name>` | `alert_notifier_plugins` | `api/proto/hitstor/plugin/v1/notifier.proto` |
| Hook | `hitstor-hook-<name>` | `hook_plugins` | `api/proto/hitstor/plugin/v1/hook.proto` |
| Filesystem Scanner | `hitstor-scanner-<name>` | `fs_scanner_plugin` | `api/proto/hitstor/plugin/v1/scanner.proto` |
| Workload | `.hpk` package with `bin/hitstor-plugin-<id>` | installed with `hitstor-agent plugin install` | `api/proto/hitstor/plugin/v1/workload.proto` |

## Common Concepts

### Discovery

Plugins are discovered by searching for binaries matching the naming pattern:

1. The `plugin_dir` config path (if set)
2. The system `$PATH`

### Handshake

All plugins use the same magic cookie handshake (`HITSTOR_PLUGIN`). If the handshake fails, the plugin process is killed immediately.

### Configurable Interface

All plugin types optionally support a `Configure(map[string]string)` RPC, called once after launch. Implement `plugin.Configurable` to receive settings.

### Protocol Version

The handshake uses `ProtocolVersion: 1`. Incompatible protocol changes will bump this version, causing older plugins to fail cleanly.

---

## Storage Backend Plugins

Storage backend plugins provide alternative repository storage (cloud providers, databases, etc.).

### Architecture

```
hitstor (host)                    hitstor-backend-<name> (plugin)
     |                                    |
     |--- launch subprocess ------------->|
     |--- gRPC handshake (magic cookie) ->|
     |--- Configure(settings) ----------->|
     |--- Save/Load/Stat/Remove/List ---->|
     |--- Close() kills subprocess ------>|
```

### Service RPCs

| RPC | Description |
|-----|-------------|
| `Configure` | Receives plugin-specific key-value settings. Called once after launch. |
| `Save` | Store data under a file type and ID. |
| `Load` | Retrieve data for a file type and ID. |
| `Stat` | Return metadata (size) for a stored file. |
| `Remove` | Delete a stored file. |
| `List` | Return all IDs for a given file type. |

### File Types

| Value | Constant | Description |
|-------|----------|-------------|
| 0 | `DataFile` | Pack files containing encrypted chunks |
| 1 | `IndexFile` | Index files mapping chunk IDs to pack locations |
| 2 | `SnapshotFile` | Snapshot metadata |
| 3 | `KeyFile` | Encryption key files |
| 4 | `LockFile` | Repository locks |
| 5 | `ConfigFile` | Repository configuration |

### URI Scheme

```
--repo plugin:azure/myaccount/container  ->  hitstor-backend-azure
--repo plugin:gcs/my-bucket              ->  hitstor-backend-gcs
```

The portion after the plugin name is passed as the `uri` key in `Configure()`.

### Writing a Storage Backend Plugin

```go
package main

import (
    "github.com/hooperits/hitstor/internal/backend"
    hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
)

type myBackend struct{}

var _ backend.Backend = (*myBackend)(nil)

// Implement Save, Load, Stat, Remove, List, Close...

func (b *myBackend) Configure(settings map[string]string) error {
    uri := settings["uri"]
    // parse uri, set up connection
    return nil
}

func main() {
    hitstorplugin.ServeStorageBackend(&myBackend{})
}
```

**Example:** See `cmd/hitstor-backend-memory/main.go` for a complete in-memory backend (~90 lines).

---

## Notifier Plugins

Notifier plugins deliver backup event notifications to external systems (Slack, PagerDuty, email, etc.).

### Architecture

```
hitstor serve (host)                  hitstor-notifier-<name> (plugin)
     |                                       |
     |--- launch subprocess --------------->|
     |--- gRPC handshake (magic cookie) --->|
     |--- Configure(settings) ------------->|
     |--- Notify(event) ---- (per event) ->|
     |--- Close() kills subprocess -------->|
```

### Service RPCs

| RPC | Description |
|-----|-------------|
| `Configure` | Receives plugin-specific key-value settings. Called once after launch. |
| `Notify` | Deliver a backup event notification. Called for each event. |

### Event Fields

| Field | Type | Description |
|-------|------|-------------|
| `event_type` | string | `"backup_failed"` or `"agent_stale"` |
| `server` | string | Always `"hitstor"` |
| `timestamp` | string | RFC3339 timestamp |
| `agent` | string | Agent name that triggered the event |
| `hostname` | string | Agent hostname (may be empty) |
| `message` | string | Human-readable event description |
| `details` | map | Arbitrary key-value metadata |

### Configuration

```bash
hitstor config add alert_notifier_plugins slack
```

Both the built-in webhook notifier (`alert_webhook_url`) and plugin notifiers can be active simultaneously.

### Writing a Notifier Plugin

```go
package main

import (
    "context"
    "github.com/hooperits/hitstor/internal/notifier"
    hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
)

type myNotifier struct{}

var _ notifier.Notifier = (*myNotifier)(nil)

func (n *myNotifier) Notify(ctx context.Context, event notifier.Event) error {
    // Deliver the event to your notification channel
    return nil
}

func (n *myNotifier) Close() error { return nil }

func main() {
    hitstorplugin.ServeNotifier(&myNotifier{})
}
```

**Example:** See `cmd/hitstor-notifier-log/main.go` for a log-based notifier plugin.

---

## Hook Plugins

Hook plugins fire before and after backups, enabling custom logic such as database quiescing, cache warming, or audit logging.

### Architecture

```
hitstor (host)                        hitstor-hook-<name> (plugin)
     |                                       |
     |--- launch subprocess --------------->|
     |--- gRPC handshake (magic cookie) --->|
     |--- Configure(settings) ------------->|
     |--- PreBackup(event) --- (before) --->|
     |--- PostBackup(event) -- (after) ---->|
     |--- Close() kills subprocess -------->|
```

### Service RPCs

| RPC | Description |
|-----|-------------|
| `Configure` | Receives plugin-specific key-value settings. Called once after launch. |
| `PreBackup` | Called before backup starts. Return an error to abort the backup. |
| `PostBackup` | Called after a successful backup. Errors are logged but do not fail the backup. |

### Event Fields

| Field | Type | Description |
|-------|------|-------------|
| `agent` | string | Agent name performing the backup |
| `hostname` | string | Agent hostname |
| `paths` | repeated string | Backup source paths |
| `excludes` | repeated string | Exclude patterns |
| `tags` | repeated string | Backup tags |
| `snapshot_id` | string | Snapshot ID (PostBackup only) |
| `details` | map | Arbitrary key-value metadata |

### Integration Points

- **Server-side (`hitstor serve`):** PreBackup fires in `BeginSession()` after authentication. PostBackup fires in `FinalizeBackup()` after snapshot creation.
- **Local backup (`hitstor backup`):** PreBackup fires before the archiver starts. PostBackup fires after a successful backup.

### Configuration

```bash
hitstor config add hook_plugins myhook
```

### Writing a Hook Plugin

```go
package main

import (
    "context"
    "github.com/hooperits/hitstor/internal/hook"
    hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
)

type myHook struct{}

var _ hook.Hook = (*myHook)(nil)

func (h *myHook) PreBackup(ctx context.Context, event hook.HookEvent) error {
    // Return an error to abort the backup
    return nil
}

func (h *myHook) PostBackup(ctx context.Context, event hook.HookEvent) error {
    // event.SnapshotID contains the new snapshot ID
    return nil
}

func (h *myHook) Close() error { return nil }

func main() {
    hitstorplugin.ServeHook(&myHook{})
}
```

**Example:** See `cmd/hitstor-hook-log/main.go` for a log-based hook plugin.

---

## Filesystem Scanner Plugins

Scanner plugins replace the built-in directory walker with custom scanning logic for cloud storage, database dumps, network shares, or custom filtering.

### Architecture

```
hitstor (host)                        hitstor-scanner-<name> (plugin)
     |                                       |
     |--- launch subprocess --------------->|
     |--- gRPC handshake (magic cookie) --->|
     |--- Configure(settings) ------------->|
     |--- Scan(paths, excludes) ----------->|
     |<-- stream ScanEntry (per file) ------|
     |--- Close() kills subprocess -------->|
```

The `Scan` RPC uses **server-side streaming** -- the plugin sends one `ScanEntry` per file/directory.

### Service RPCs

| RPC | Description |
|-----|-------------|
| `Configure` | Receives plugin-specific key-value settings. Called once after launch. |
| `Scan` | Walks paths and streams file/directory entries back to the host. |

### ScanEntry Fields

| Field | Type | Description |
|-------|------|-------------|
| `path` | string | Absolute path of the file or directory |
| `node` | `NodeInfo` | File metadata |
| `error_message` | string | Non-fatal error for this entry |

### NodeInfo Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Base name |
| `type` | string | `"file"`, `"dir"`, or `"symlink"` |
| `mode` | uint32 | Unix file mode bits |
| `uid` | uint32 | Owner user ID |
| `gid` | uint32 | Owner group ID |
| `mtime_ns` | int64 | Modification time (nanoseconds since epoch) |
| `atime_ns` | int64 | Access time (nanoseconds since epoch) |
| `ctime_ns` | int64 | Change time (nanoseconds since epoch) |
| `size` | uint64 | File size in bytes |
| `inode` | uint64 | Inode number |
| `links` | uint64 | Hard link count |
| `link_target` | string | Symlink target path |

### Configuration

```bash
# Only one scanner plugin active at a time
hitstor config set fs_scanner_plugin myscanner
```

### Writing a Scanner Plugin

```go
package main

import (
    "github.com/hooperits/hitstor/internal/scanner"
    hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
)

type myScanner struct{}

var _ scanner.Scanner = (*myScanner)(nil)

func (s *myScanner) Scan(paths []string, excludes []string) (<-chan scanner.ScanResult, error) {
    ch := make(chan scanner.ScanResult)
    go func() {
        defer close(ch)
        // Walk paths and send results on the channel
    }()
    return ch, nil
}

func (s *myScanner) Close() error { return nil }

func main() {
    hitstorplugin.ServeFilesystemScanner(&myScanner{})
}
```

**Example:** See `cmd/hitstor-scanner-builtin/main.go` for a scanner plugin that wraps the built-in walker.

---

## Workload Plugins

Workload plugins teach an agent to back up an application, for example a
database engine. Unlike the other types they are not loose binaries found on
`$PATH`: they ship as signed `.hpk` packages that the agent installs into its
plugin store with [`hitstor-agent plugin`](agent-commands/plugin.md). The
design is described in
[Workload Plugin Architecture](../dev-guide/plugin-architecture.md).

### Service RPCs

Defined in `api/proto/hitstor/plugin/v1/workload.proto`. Only `GetMetadata`
is mandatory; the host treats an unimplemented RPC as an unsupported
capability.

| RPC | Go interface | Purpose |
|-----|--------------|---------|
| `GetMetadata` | `plugin.Workload` | Return id, name, version, protocol version, description, and capabilities. The agent calls it at install time and rejects the package if the id or version differ from the manifest |
| `Configure` | `plugin.Configurable` | Receive the job's plugin settings |
| `Browse` | `plugin.WorkloadBrowser` | List selectable objects under a path (for example instance → databases) |
| `Backup` | `plugin.WorkloadBackuper` | Back up the selected objects, streaming one or more items |

Restore RPCs (`PlanRestore`, `Restore`) are being added.

### Items

A backup produces **items**: named byte streams, such as one database backup
file, each with optional JSON metadata. A plugin emits them through the
`ItemSink` it receives:

```go
func (p *myPlugin) Backup(ctx context.Context, req plugin.BackupRequest, sink plugin.ItemSink) error {
    for _, obj := range req.Selection {
        w, err := sink.Item(obj + "/full-20260101T020000Z.bak")
        if err != nil {
            return err
        }
        if err := p.streamBackup(ctx, obj, w); err != nil { // write the bytes
            return err
        }
        if err := w.Finish([]byte(`{"first_lsn":"..."}`)); err != nil { // metadata, known at the end
            return err
        }
    }
    return nil
}
```

Item paths are relative and slash-separated. The agent stores each item as the
file `<plugin id>/<item path>` in the snapshot, next to
`<item path>.meta.json`, which holds the plugin's metadata plus the item's size,
SHA-256, kind, and creation time. Items go through the same chunking,
deduplication, compression, and encryption as files, and can always be
restored as plain files.

Settings arrive through `Configure` and are stored in the job on the server.
**Never put secrets in settings**: read them from environment variables, which
the plugin inherits from the agent service (`agent.env`).

To try a plugin on a host without a server, install it and run:

```bash
hitstor-agent plugin browse example
hitstor-agent plugin test-backup example --select demo/alpha --out /tmp/items
```

### Writing a Workload Plugin

Put the source in `plugins/<id>/` with a source `plugin.yaml` and embed it, so
the binary always reports the version its package declares:

```yaml
# plugins/example/plugin.yaml
id: example
name: Example workload plugin
version: 0.1.0
type: workload
protocol_version: 1
min_agent_version: 0.1.0
publisher: hitstor
description: Reference workload plugin.
```

```go
//go:embed plugin.yaml
var sourceManifest []byte

func main() {
    m, _ := pluginpkg.ParseSource(sourceManifest)
    plugin.ServeWorkload(&myWorkload{manifest: m})
}
```

A source manifest omits `platforms` and `entrypoint`: packaging fills them in,
producing one package per platform.

### Packaging and Signing

`hitstor-pluginkit` builds and signs packages:

```bash
hitstor-pluginkit keygen --out mykey            # mykey.key (secret) + mykey.pub
hitstor-pluginkit pack --source plugins/example/plugin.yaml \
    --binary bin/hitstor-plugin-example.exe --os windows --arch amd64 \
    --key-file mykey.key --out dist
hitstor-pluginkit verify dist/hitstor-plugin-example-0.1.0-windows-amd64.hpk \
    --trusted-key "$(cat mykey.pub)"
```

`make plugin-packages PLUGIN_KEY_FILE=mykey.key` packages every plugin listed
in `WORKLOAD_PLUGINS` for every platform in `PLUGIN_PLATFORMS`.
`PLUGIN_UNSIGNED=1` builds unsigned packages for local testing; agents accept
them only with `plugin_allow_unsigned: true`.

A package is a zip with `plugin.yaml`, the binary under `bin/`, optional
documentation, and a `SIGNATURE` file: an ed25519 signature over a SHA-256
digest of every other entry (name and content hash, sorted). Adding, removing,
or changing any file invalidates the signature.

Official packages are signed in the release workflow with the
`HITSTOR_PLUGIN_SIGNING_KEY` secret of the `release` environment, and their
public key is embedded in the agent (`pluginpkg.OfficialKeys`). To install your
own plugins, add your public key to `plugin_trusted_keys` on the agents.

**Example:** See `plugins/example/` for a minimal workload plugin.

---

## Testing Plugins

### Unit Tests

Test your interface implementation directly without launching a subprocess:

```go
be := &myBackend{}
be.Configure(map[string]string{"uri": "test"})
be.Save(ctx, backend.DataFile, id, data)
// ...
```

### Integration Tests

Build the plugin binary, then launch it via the plugin manager:

```go
be, err := plugin.OpenStorageBackend("mycloud", "/path/to/bin", map[string]string{
    "uri": "test-bucket",
})
defer be.Close()
```

See `internal/plugin/integration_test.go` for working examples of all four plugin types.

## Message Size Limits

go-plugin sets max gRPC message size to ~2 GiB. HitStor pack files cap at 128 MiB, so no streaming is needed for storage backends. The filesystem scanner plugin uses server-side streaming for large directory trees.

## See Also

- [Plugin Usage Guide](../user-guide/plugins.md) -- installing and configuring plugins
- [API Reference](api-reference.md) -- main gRPC backup protocol
