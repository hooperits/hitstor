# Agent Management

HitStor supports two methods for registering backup agents: **dashboard-managed** (recommended) and **CLI-based** (legacy). Dashboard-managed agents use a token-based flow where the server connects to the agent, authenticates, and auto-configures it. CLI-based agents use a manually-distributed API key.

## Dashboard-Managed Agents (Recommended)

Dashboard-managed agents are added directly from the web dashboard. The server connects to the agent over gRPC, authenticates using the agent's token, and sends it the API key and server address automatically.

### Prerequisites

1. Install the agent on the remote machine (see [Agent Deployment](agent-deployment.md))
2. The agent generates a token on its first start — note the token (`hitstor-agent show-id`) and the machine's IP address
3. Ensure port **8422** (agent gRPC listen port) is reachable from the server

### Add an Agent from the Dashboard

1. Open the HitStor dashboard at `https://SERVER_IP:8421` (administrators)
2. Navigate to **Agents** and click **+ Add Agent**
3. Choose **Network Scan** to scan a local subnet, or **Manual Entry** to type the agent's IP address and click **Connect**
4. Click **Register** on the agent found, and fill in **Register Agent**:
    - **Agent Name** — descriptive identifier (for example, `web-server-01`); letters, numbers, hyphens, and underscores
    - **Agent Address** — `host:port`, filled in from the scan (for example, `192.168.1.50:8422`)
    - **Agent Token** — 64-character hex token from the agent (`hitstor-agent show-id` on the agent machine)
5. Click **Register Agent**

See [Agent Discovery](agent-discovery.md) for the network scan workflow.

The server connects to the agent, verifies the token, and sends an API key and the server address, which the agent writes to its `agent.env`. No manual key copy is needed. Registering again with an existing name re-registers that agent (new API key) and keeps its jobs.

### Choose What to Back Up

What an agent backs up is set per backup job, not on the agent. In **Backups → + New Backup Job**, the selection tree browses the agent's filesystem (and its installed plugins) live over gRPC; check the folders to back up. See [Backup Jobs](backup-jobs.md).

### Remove an Agent

From the Agents list or the agent's page (**Remove**, administrators), or via the CLI:

```bash
hitstor serve agent remove AGENT_NAME
```

This revokes the agent's API key. The agent can no longer connect to the server. Existing snapshots created by the agent are not deleted. The dashboard asks whether to keep the agent's backup jobs; the CLI deletes them unless you add `--keep-jobs`.

## CLI-Based Agents (Legacy)

CLI-based agents are registered from the server command line. The API key must be manually copied to the agent machine.

### Register an Agent

```bash
hitstor serve agent add AGENT_NAME
```

On success, the command prints a 64-character hexadecimal API key:

```
Agent "web-server-01" registered.
API Key: a3f8c1d9e4b7...6f2a0e5d

Save this key — it cannot be displayed again.
```

Copy this key to the agent's `/etc/hitstor/agent.env` file as `HITSTOR_AGENT_API_KEY`, next to `HITSTOR_AGENT_SERVER` (see [Agent Deployment](agent-deployment.md#manual-cli-registration)).

### List Agents

View all registered agents (both managed and CLI-based):

```bash
hitstor serve agent list
```

Output:

```
NAME                  CREATED               LAST BACKUP           MANAGED   SUSPENDED   ADDR
web-server-01         2026-02-01 10:12:40   2026-02-21 03:00:12   yes       no          192.168.1.50:8422
db-primary            2026-02-01 10:20:05   2026-02-21 03:15:08   no        no          -
staging-app           2026-02-03 09:41:17   never                 yes       yes         192.168.1.52:8422
```

`--all-repos` lists the agents of every configured repository; `--json` prints the records as JSON.

### Remove an Agent

```bash
hitstor serve agent remove AGENT_NAME
```

To keep the agent's backup jobs (for example, to register it again under the same name):

```bash
hitstor serve agent remove AGENT_NAME --keep-jobs
```

Removing an agent never deletes its snapshots; use [`hitstor forget`](../user-guide/retention.md) for that.

## Agent Status

The dashboard's **Agents** page shows each agent's status:

| Status | Meaning |
|--------|---------|
| `ok` | Agent has backed up within the last 25 hours |
| `stale` | Agent has not backed up within the last 25 hours |
| `never` | Agent is registered but has never completed a backup |
| `suspended` | Agent is suspended (by an operator or by Sentinel); it cannot back up |

Webhook alerts use the configurable `alert_stale_threshold` instead (default: 25 hours); see [Monitoring](monitoring.md).

## Agent Token (Dashboard-Managed)

Dashboard-managed agents authenticate using a **token** — a 64-character hexadecimal string (256 bits of entropy) generated on the agent's first start (the installers start the agent, so it exists right after installation).

**Token properties:**

- Generated automatically on the agent's first start, stored at `/etc/hitstor/agent-token` (mode 0600; `C:\ProgramData\HitStor\agent-token` on Windows)
- Retrievable any time with `sudo hitstor-agent show-id`
- Stored encrypted (AES-256-GCM) on the server in the agent registry
- Used by the server to authenticate gRPC calls to the agent (registration, browsing, plugin installation, wake-up)

## API Key Security (CLI-Based)

CLI-based agents authenticate to the server using a **64-character hexadecimal API key** (256 bits of entropy).

**Key properties:**

- One key per agent
- Displayed only once at registration time
- Cannot be retrieved or displayed again after registration
- Stored as a hash in the encrypted agent registry, alongside the repository

**Best practices:**

- Copy the key directly into the agent's environment file at registration time
- Do not store keys in version control, wikis, or chat logs
- If a key is compromised, remove the agent and re-register it

### Rotating an API Key

For a dashboard-managed agent, register it again from **Agents → + Add Agent** with the same name and its token: the server issues a new key, sends it to the agent and keeps the jobs. From the CLI, `hitstor serve agent re-register AGENT_NAME --token TOKEN` issues the new key (put it in the agent's `agent.env`).

For a CLI-based agent:

1. Remove the agent, keeping its jobs: `hitstor serve agent remove AGENT_NAME --keep-jobs`
2. Register it again: `hitstor serve agent add AGENT_NAME`
3. Update the agent's configuration with the new key
4. Restart the agent service

Snapshots are never deleted by removing an agent.

## Agent Registry

The agent registry is an encrypted data structure stored alongside the repository. It contains:

- Agent names and hashed API keys
- Agent metadata (registration time, last backup time, suspension, installed plugins)
- Backup jobs, with their selection, schedule or backup plan, and latest results
- For managed agents: encrypted token and agent address

The registry is encrypted with the repository key and cannot be read without the repository password.

## Next Steps

- [Agent Deployment](agent-deployment.md) — install and configure agents on remote machines
- [Dashboard](dashboard.md) — view and manage agents from the web interface
- [Monitoring](monitoring.md) — set up alerts for stale agents
