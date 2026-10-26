# hitstor serve

Start the gRPC backup server.

## Synopsis

```
hitstor serve [flags]
hitstor serve agent add <name>
hitstor serve agent list
hitstor serve agent remove <name>
```

## Description

Starts the HitStor gRPC server that accepts remote agent backups over TLS. The server handles deduplication, encryption, and storage on behalf of remote agents. It also starts the web dashboard on a separate HTTP port.

If no TLS certificate is provided, a self-signed certificate is generated automatically.

An exclusive lock is acquired on the repository for the duration of the server lifetime.

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--port` | int | 8420 | gRPC listen port |
| `--tls-cert` | string | | Path to TLS certificate file |
| `--tls-key` | string | | Path to TLS private key file |
| `--dashboard-port` | int | 8421 | HTTP dashboard listen port |

Plus all global flags.

## Subcommands

### `serve agent add <name>`

Register a new backup agent. Outputs a 64-character hex API key that the agent uses for authentication. Save this key immediately — it cannot be displayed again.

### `serve agent list`

List all registered agents with their names, creation dates, and last backup times.

### `serve agent remove <name>`

Remove a registered agent. The agent will no longer be able to connect.

## Examples

Start the server with default settings:

```bash
hitstor serve --repo /var/lib/hitstor/repo --password "secret"
```

Start with custom TLS certificates:

```bash
hitstor serve --repo /var/lib/hitstor/repo --password "secret" \
  --tls-cert /etc/hitstor/tls/server.crt \
  --tls-key /etc/hitstor/tls/server.key
```

Register a new agent:

```bash
hitstor serve agent add webserver-01 \
  --repo /var/lib/hitstor/repo --password "secret"
```

List registered agents:

```bash
hitstor serve agent list \
  --repo /var/lib/hitstor/repo --password "secret"
```

## Notes

- The server requires a local filesystem backend (not S3) because the agent registry is stored alongside the repository.
- Graceful shutdown on SIGINT/SIGTERM.
- The dashboard is served as an embedded HTML5 application with no external dependencies.
