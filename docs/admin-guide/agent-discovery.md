# Agent Auto-Discovery

The dashboard can discover `hitstor-agent` instances on your local network without manually entering every hostname.

## Workflow

1. Open **Agents** in the dashboard and click **+ Add Agent** (administrators)
2. Stay on the **Network Scan** tab
3. Select one of the server's subnets (from `/api/discover/subnets`)
4. Click **Scan Network** — the server probes port **8422** on every address of the subnet (up to a `/16`; the scan stops after 30 seconds) and shows the agents found with their host name, OS, version, and whether they are already registered
5. Click **Register** on an agent, enter its name and token, and click **Register Agent**

## Manual Entry

Use **Manual Entry** for a single machine: type the agent's IP address and click **Connect**. The server probes that address on port **8422** and, if an agent answers, offers to register it.

Discovery and manual entry only use port 8422. For an agent on another port, or behind NAT, register it through the API with its full address: `POST /api/agents` with `name`, `agent_addr` (`host:port`) and `token` (see [Dashboard → JSON API](dashboard.md#json-api)).

## API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/discover/subnets` | List the server's IPv4 subnets |
| `POST` | `/api/discover/scan` | Scan a subnet (`{"cidr": "..."}`, IPv4, `/16` or smaller); returns an operation ID whose progress streams from `/api/operations/{id}/events` |
| `POST` | `/api/discover/probe` | Probe a specific IP address |

## Requirements

- Agents must run in **daemon mode** with AgentService listening (default port 8422)
- Server must reach agent addresses on the network (firewall rules)
- After discovery, complete registration with the agent's token (`hitstor-agent show-id`); the server then sends the agent its API key. What to back up is chosen afterwards in a [backup job](backup-jobs.md)

## See Also

- [Agent Management](agent-management.md)
- [Agent Deployment](agent-deployment.md)
