# hitstor-agent show-id

Display the agent token for dashboard registration.

## Synopsis

```
hitstor-agent show-id
```

## Description

Reads and prints the agent token from `/etc/hitstor/agent-token`. The token is a 64-character hexadecimal string generated during agent installation or on first daemon start.

Use this token when adding the agent from the HitStor dashboard:

1. Open the dashboard and navigate to **Agents**
2. Click **+ Add Agent**
3. Enter the agent's IP address, port (default: 8422), and this token

## Example

```bash
sudo hitstor-agent show-id
```

Output:

```
a3f8c1d9e4b72a5601df83be9c7f4a2d0e6b1893fc5d74a0e2c8f961b3d507ea
```

## Notes

- The token file is located at `/etc/hitstor/agent-token` with mode `0600` (root-only readable)
- If the token file does not exist, start the agent daemon first (`hitstor-agent daemon`) — it generates the token automatically
- The token is used by the server to authenticate management gRPC calls (filesystem browsing, configuration)
