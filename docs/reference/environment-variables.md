# Environment Variables

HitStor reads the following environment variables. Environment variables override config file values but are overridden by CLI flags.

## Server (`hitstor`)

| Variable | Config Key | Description |
|----------|-----------|-------------|
| `HITSTOR_REPO` | `repo` | Repository location (local path, `s3:endpoint/bucket`, or `plugin:name/args`) |
| `HITSTOR_PASSWORD` | -- | Repository password (no config key -- only env var or interactive prompt) |
| `HITSTOR_PASSWORD_FILE` | `password_file` | Path to file containing the repository password |

## Agent (`hitstor-agent`)

| Variable | Config Key | Description |
|----------|-----------|-------------|
| `HITSTOR_AGENT_LISTEN_PORT` | `agent_listen_port` | gRPC listen port for dashboard management (default: 8422) |
| `HITSTOR_AGENT_SERVER` | `agent_server` | Remote server address (`host:port`) — set automatically for dashboard-managed agents |
| `HITSTOR_AGENT_API_KEY` | `agent_api_key` | Agent API key for authentication — set automatically for dashboard-managed agents |

## Usage Examples

### Server with environment variables

```bash
export HITSTOR_REPO=/var/lib/hitstor/repo
export HITSTOR_PASSWORD=mysecretpassword
hitstor serve --port 8420 --tls-cert cert.pem --tls-key key.pem
```

### Agent with environment variables

```bash
export HITSTOR_AGENT_SERVER=backup.internal:8420
export HITSTOR_AGENT_API_KEY=a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2
hitstor-agent backup /etc /home
```

### Systemd environment file

Both the server and agent systemd units support environment files:

```bash
# /etc/hitstor/server.env
HITSTOR_REPO=/var/lib/hitstor/repo
HITSTOR_PASSWORD=mysecretpassword

# /etc/hitstor/agent.env (dashboard-managed)
HITSTOR_AGENT_LISTEN_PORT=8422

# /etc/hitstor/agent.env (legacy)
# HITSTOR_AGENT_SERVER=backup.internal:8420
# HITSTOR_AGENT_API_KEY=a1b2c3d4...
```

### Docker

```bash
docker run -e HITSTOR_REPO=/data/repo \
           -e HITSTOR_PASSWORD=mysecretpassword \
           hitstor:latest serve
```

## Integrations (server)

These variables override the corresponding `integrations.*` keys in
`config.yaml` when set. Prefer encrypted YAML in production; use **Settings →
Import from environment** to migrate values into the config file.

| Variable | Config Key | Description |
|----------|-----------|-------------|
| `GEMINI_API_KEY` | `integrations.gemini_api_key_encrypted` | Gemini API key for Sentinel (plaintext env; stored encrypted when imported) |

When both env vars and YAML are set, environment variables take precedence.
After import, secrets in YAML are encrypted and the env vars can be removed.

### Ignored legacy variables

`HITSTOR_M365_*`, `HITSTOR_GCP_PROJECT`, `HITSTOR_AWS_ROLE_ARN`,
`HITSTOR_AWS_REGION`, and `HITSTOR_AZURE_SUB` are **no longer read** since
v2.0.0, when SaaS, database, and cloud-VM backup were removed from the
product. Setting them has no effect.

`HITSTOR_AZURE_TENANT` and `HITSTOR_AZURE_CLIENT` are still read, but their
meaning changed: they no longer configure cloud-VM discovery — they now
authenticate Sentinel's optional Azure Blob retention lock (see below).

## Sentinel (server)

Sentinel is configured entirely through environment variables, read once at
`hitstor serve` startup — there is no `hitstor config set` equivalent. The
Gemini API key/model fall back to the `integrations.*` values above when unset
here.

| Variable | Default | Description |
|----------|---------|-------------|
| `HITSTOR_SENTINEL_ENABLED` | `true` | Set to `false` to disable Sentinel entirely |
| `HITSTOR_SENTINEL_MODEL` | `integrations.gemini_model`, else `gemini-1.5-flash` | Gemini model used for threat analysis |
| `HITSTOR_SENTINEL_API_KEY` | `integrations.gemini_api_key_encrypted` | Gemini API key for Sentinel (falls back to the shared Gemini key) |
| `HITSTOR_SENTINEL_THRESHOLD` | `0.8` | Confidence (0-1) above which a detected threat triggers containment |
| `HITSTOR_SENTINEL_RETENTION_DAYS` | `30` | Retention period applied when Sentinel locks a storage bucket |
| `HITSTOR_SENTINEL_BACKEND_TYPE` | `gcs` | Which storage backend's retention lock to use: `gcs`, `s3`, or `azure` |
| `HITSTOR_GCS_BUCKET` | | GCS bucket to lock (when `BACKEND_TYPE=gcs`) |
| `HITSTOR_S3_BUCKET` | | S3 bucket to lock (when `BACKEND_TYPE=s3`) |
| `HITSTOR_S3_REGION` | | S3 bucket region |
| `HITSTOR_S3_ROLE_ARN` | | AWS role to assume for the S3 Object Lock API call |
| `HITSTOR_AZURE_CONTAINER` | | Azure Blob container to lock (when `BACKEND_TYPE=azure`) |
| `HITSTOR_AZURE_STORAGE_ACCOUNT` | | Azure storage account name |
| `HITSTOR_AZURE_TENANT` | | Azure AD tenant ID for the retention-lock API call |
| `HITSTOR_AZURE_CLIENT` | | Azure AD client ID for the retention-lock API call |

The retention lock only fires when Sentinel detects a threat above the
confidence threshold — it does not run on every backup. See
[Ransomware Sentinel](../admin-guide/sentinel.md).

## Notes

- The repository password can be provided via `--password`, `--password-file`, `HITSTOR_PASSWORD`, `HITSTOR_PASSWORD_FILE`, or the config file `password_file` key. CLI flags take highest precedence.
- `HITSTOR_PASSWORD_FILE` takes precedence over `HITSTOR_PASSWORD` if both are set.
- Sensitive values (`s3_access_key`, `s3_secret_key`, `agent_api_key`, `serve_dashboard_password`) do not have dedicated environment variables -- use the config file or CLI flags.

## See Also

- [Configuration Reference](config-reference.md) -- all config keys
- [Configuration Guide](../user-guide/configuration.md) -- usage tutorial
