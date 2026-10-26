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
| remove      | Remove a registered backup agent |
| assign-repo | Assign target repo for an agent (for future jobs) |

## Description

Agents perform backups. In multi-repo setups, use `--repo-name` to target a specific repository's agent registry.

Supports both simple and dashboard-managed agents.

Service management is available on the agent binary (`hitstor-agent service ...` on both Linux and Windows).

## Examples

```bash
hitstor --config /etc/hitstor/config.yaml --repo-name Production agents list
hitstor --config /etc/hitstor/config.yaml agents add --addr 192.168.1.10:8422 my-agent
hitstor --config /etc/hitstor/config.yaml agents assign-repo my-agent Production
hitstor --config /etc/hitstor/config.yaml agents list --all-repos
```

See also: `hitstor jobs` (jobs reference agents), dashboard agent management, and `hitstor-agent service`.
