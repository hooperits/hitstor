# hitstor serve

Start the gRPC backup server.

## Synopsis

```
hitstor serve [flags]
hitstor serve agent add|list|remove|re-register|assign-repo ...
hitstor serve user add|list|grant|passwd|reset-2fa|disable|enable|remove ...
```

## Description

Starts the HitStor gRPC server that accepts remote agent backups over TLS. The server handles deduplication, encryption, and storage on behalf of remote agents. It also starts the web dashboard on a separate HTTP port.

If no TLS certificate is provided, a self-signed certificate is generated automatically. The dashboard uses the same certificate: once a repository is configured it is served over HTTPS on the dashboard port, and a plain-HTTP listener on the next port (dashboard port + 1) redirects to it. With no repository configured the server starts in setup mode and serves only the dashboard, over plain HTTP.

Dashboard sign-in uses personal accounts with roles and two-step verification; see [Users and Access](../../admin-guide/users.md). The `serve_dashboard_password` setting is only used until the first administrator exists.

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--port` | int | 8420 | gRPC listen port |
| `--tls-cert` | string | | Path to TLS certificate file |
| `--tls-key` | string | | Path to TLS private key file |
| `--dashboard-port` | int | 8421 | Dashboard listen port |
| `--advertise-addr` | string | auto-detected | Address sent to agents during registration (`host:port`) |

Plus all global flags.

## Subcommands

### `serve agent add <name>`

Register a new backup agent. Outputs a 64-character hex API key that the agent uses for authentication. Save this key immediately — it cannot be displayed again.

### `serve agent list`

List all registered agents with their names, creation dates, and last backup times.

### `serve agent remove <name>`

Remove a registered agent. The agent will no longer be able to connect. Its backup jobs are deleted too unless `--keep-jobs` is given.

### `serve agent re-register <name> --token <token>`

Re-register an existing agent, keeping its jobs, with the token printed by `hitstor-agent show-id`. `--addr` sets the agent's address; `--skip-rpc` only updates the registry without contacting the agent.

### `serve agent assign-repo <name> <repo-name>`

Set the repository the agent's future jobs use.

`hitstor agents` is the same command group; see [agents](agents.md).

### `serve user ...`

Manage dashboard users from the server console, for example when no administrator can sign in:

| Subcommand | Description |
|------------|-------------|
| `add <name> --grant <role>[=agent1,agent2]` | Create a user (repeat `--grant`; `--display-name`, `--email`). Prints a temporary password |
| `list` | List users |
| `grant <name> --grant ...` | Replace a user's roles |
| `passwd <name>` | Reset the password to a temporary one and end the user's sessions |
| `reset-2fa <name>` | Remove the user's authenticator and recovery codes; they set it up again at the next sign-in |
| `disable <name>` / `enable <name>` | Stop or allow sign-in; `enable` also lifts a lockout |
| `remove <name>` | Delete a user |

Roles are `admin`, `backup_operator`, `restore_operator`, and `viewer`. See [Users and Access](../../admin-guide/users.md#cli).

The `serve agent` and `serve user` commands work on the repository where the server keeps its agent registry and users (the first configured repository by name), so `--repo-name` is not needed; naming another repository prints a warning, because the running server does not read it.

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

- The first repository (by name) must be on a local filesystem (not S3): the agent registry, dashboard users, and audit log are stored alongside it.
- Graceful shutdown on SIGINT/SIGTERM.
- The dashboard is served as an embedded HTML5 application with no external dependencies.
