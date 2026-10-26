# Agent Management

HitStor supports two methods for registering backup agents: **dashboard-managed** (recommended) and **CLI-based** (legacy). Dashboard-managed agents use a token-based flow where the server connects to the agent, authenticates, and auto-configures it. CLI-based agents use a manually-distributed API key.

## Dashboard-Managed Agents (Recommended)

Dashboard-managed agents are added directly from the web dashboard. The server connects to the agent over gRPC, authenticates using the agent's token, and sends it the API key and server address automatically.

### Prerequisites

1. Install the agent on the remote machine (see [Agent Deployment](agent-deployment.md))
2. The agent generates a token during installation — note the token and the machine's IP address
3. Ensure port **8422** (agent gRPC listen port) is reachable from the server

### Add an Agent from the Dashboard

1. Open the HitStor dashboard at `http://SERVER_IP:8421`
2. Navigate to **Agents**
3. Choose **Network Discovery** to scan local subnets, or **Manual Entry** to add by address
4. For manual entry, fill in:
    - **Name** — descriptive identifier (for example, `web-server-01`)
    - **Agent address** — `host:port` (for example, `192.168.1.50:8422`)
    - **Token** — 64-character hex token from the agent (`hitstor-agent show-id` on the agent machine)
5. Click **Add Agent**

See [Agent Discovery](agent-discovery.md) for the network scan workflow.

The server connects to the agent, verifies the token, sends an API key and server address, and the agent begins backing up automatically. No manual key copy is needed.

### Browse Agent Filesystem

For dashboard-managed agents, the Agent Detail page includes a **Backup Paths** section with a filesystem tree browser:

1. Click the agent name in the Agents list
2. Expand directories in the tree by clicking the arrow icons
3. Check the directories you want to back up
4. Click **Save Backup Paths**

The server connects to the agent over gRPC to browse its filesystem in real time. Only directories are selectable as backup paths.

### Remove an Agent

From the Agents list, or via the CLI:

```bash
hitstor serve agent remove AGENT_NAME
```

This revokes the agent's API key. The agent can no longer connect to the server. Existing snapshots created by the agent are not deleted.

## CLI-Based Agents (Legacy)

CLI-based agents are registered from the server command line. The API key must be manually copied to the agent machine.

### Register an Agent

```bash
hitstor serve agent add AGENT_NAME
```

On success, the command prints a 64-character hexadecimal API key:

```
Agent "web-server-01" registered.
API Key: a3f8c1d9e4b7...6f2a0e5d (64 characters)

Store this key securely. It cannot be displayed again.
```

Copy this key to the agent's `/etc/hitstor/agent.env` file.

### List Agents

View all registered agents (both managed and CLI-based):

```bash
hitstor serve agent list
```

Output:

```
NAME              STATUS     LAST BACKUP           SNAPSHOTS
web-server-01     active     2026-02-21 03:00:12   47
db-primary        active     2026-02-21 03:15:08   47
staging-app       stale      2026-02-19 12:30:00   23
```

### Remove an Agent

```bash
hitstor serve agent remove AGENT_NAME
```

To remove an agent and all of its snapshots:

```bash
hitstor serve agent remove AGENT_NAME --purge-snapshots
```

## Agent Status

The status column shows:

| Status | Meaning |
|--------|---------|
| `active` | Agent has backed up within the stale threshold |
| `stale` | Agent has not backed up within the stale threshold (default: 25 hours) |
| `new` | Agent is registered but has never completed a backup |

## Agent Token (Dashboard-Managed)

Dashboard-managed agents authenticate using a **token** — a 64-character hexadecimal string (256 bits of entropy) generated during agent installation.

**Token properties:**

- Generated automatically during install, stored at `/etc/hitstor/agent-token` (mode 0600)
- Retrievable any time with `sudo hitstor-agent show-id`
- Stored encrypted (AES-256-GCM) on the server in the agent registry
- Used by the server to authenticate gRPC calls to the agent (browse filesystem, configure paths)

## API Key Security (CLI-Based)

CLI-based agents authenticate to the server using a **64-character hexadecimal API key** (256 bits of entropy).

**Key properties:**

- One key per agent
- Displayed only once at registration time
- Cannot be retrieved or displayed again after registration
- Stored encrypted in the agent registry alongside the repository

**Best practices:**

- Copy the key directly into the agent's environment file at registration time
- Do not store keys in version control, wikis, or chat logs
- If a key is compromised, remove the agent and re-register it

### Rotating an API Key

1. Remove the agent: `hitstor serve agent remove AGENT_NAME`
2. Re-register the agent: `hitstor serve agent add AGENT_NAME`
3. Update the agent's configuration with the new key
4. Restart the agent service

Existing snapshots are preserved as long as `--purge-snapshots` is not used.

## Agent Registry

The agent registry is an encrypted data structure stored alongside the repository. It contains:

- Agent names and hashed API keys
- Agent metadata (registration time, last backup time, snapshot count)
- Per-agent scheduling configuration
- For managed agents: encrypted token and agent address

The registry is encrypted with the repository key and cannot be read without the repository password.

## Next Steps

- [Agent Deployment](agent-deployment.md) — install and configure agents on remote machines
- [Dashboard](dashboard.md) — view and manage agents from the web interface
- [Monitoring](monitoring.md) — set up alerts for stale agents
