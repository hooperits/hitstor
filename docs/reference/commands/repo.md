# `hitstor repo`

Repository management commands (multi-repository aware).

## Usage

```bash
hitstor repo [command]
```

## Subcommands

| Command | Description |
|---------|-------------|
| `list` | List configured repositories from the multi-repo config |
| `create <name> <path>` | Create and initialize a new local repository |
| `delete <name>` | Remove a repository from the config (does not delete data) |
| `health` | Show operational health for the selected repository |

## Global Flags (relevant)

- `--repo-name <name>` — Select a named repository from the `repositories:` list in config.
- `--config /etc/hitstor/config.yaml` — Recommended when running on the server.

## Examples

```bash
# List all repositories known to this server
hitstor --config /etc/hitstor/config.yaml repo list

# Create a new local repo (creates password file next to config)
hitstor --config /etc/hitstor/config.yaml repo create MyRepo /data/myrepo

# Health check on a named repo
hitstor --config /etc/hitstor/config.yaml --repo-name MyRepo repo health
```

See also: `hitstor config show` (shows modern repositories section when present).
