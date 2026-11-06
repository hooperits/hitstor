# hitstor agents

Manage registered backup agents (multi-repo aware).

## Synopsis

```
hitstor agents [command]
```

## Available Commands

| Command     | Description |
|-------------|-------------|
| add         | Register a new backup agent (supports --addr for managed) |
| list        | List registered backup agents (supports --repo-name, --all-repos, --json) |
| remove      | Remove a registered backup agent (`--keep-jobs` keeps its jobs) |
| re-register | Re-register an existing agent, keeping its jobs, with `--token` from `hitstor-agent show-id` (`--addr`, `--skip-rpc`) |
| assign-repo | Assign target repo for an agent (for future jobs) |

## Description

Agents perform backups. `hitstor agents` is the same command group as [`hitstor serve agent`](serve.md#subcommands). The server keeps its agent registry in the first configured repository (by name), and these commands use that repository by default; `--repo-name` with another repository prints a warning, because the running server does not read it. `list --all-repos` lists the agents recorded in every configured repository.

Supports both simple and dashboard-managed agents.

Service management is available on the agent binary (`hitstor-agent service ...` on both Linux and Windows).

## Examples

```bash
hitstor --config /etc/hitstor/config.yaml agents list
hitstor --config /etc/hitstor/config.yaml agents add --addr 192.0.2.10:8422 my-agent
hitstor --config /etc/hitstor/config.yaml agents assign-repo my-agent Production
hitstor --config /etc/hitstor/config.yaml agents list --all-repos
```

See also: `hitstor jobs` (jobs reference agents), dashboard agent management, and `hitstor-agent service`.
