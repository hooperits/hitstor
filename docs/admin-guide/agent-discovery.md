# Agent Auto-Discovery

The dashboard can discover `hitstor-agent` instances on your local network without manually entering every hostname.

## Workflow

1. Open **Agents** in the dashboard
2. Click **Network Discovery**
3. Select a subnet (from `/api/discover/subnets`) or enter a CIDR
4. Run **Scan** — the server probes common agent ports
5. Review discovered hosts and **Add** them with one click

## Manual Entry

Use **Manual Entry** when discovery is not possible (NAT, remote sites, non-standard ports). Provide:

- **Name** — display name for the agent
- **Agent address** — `host:port` (default agent management port: **8422**)

## API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/discover/subnets` | List local subnets |
| `POST` | `/api/discover/scan` | Scan a subnet |
| `POST` | `/api/discover/probe` | Probe a specific address |

## Requirements

- Agents must run in **daemon mode** with AgentService listening (default port 8422)
- Server must reach agent addresses on the network (firewall rules)
- After discovery, complete registration by assigning an API key and backup paths

## See Also

- [Agent Management](agent-management.md)
- [Agent Deployment](agent-deployment.md)