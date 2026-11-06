# Environment Variables

HitStor reads the following environment variables. CLI flags take precedence over them. For the repository location and the agent's server, API key, and CA certificate, a value in the config file also takes precedence: the environment variable is only used when neither a flag nor the config file sets it. The `HITSTOR_LOG_*` variables override their config keys.

## Server (`hitstor`)

| Variable | Config Key | Description |
|----------|-----------|-------------|
| `HITSTOR_REPO` | `repo` | Repository location (local path, `s3:endpoint/bucket`, or `plugin:name/args`) |
| `HITSTOR_PASSWORD` | -- | Repository password (no config key). Used when `--password` is not given; takes precedence over `--password-file` |
| `HITSTOR_SYNC_WRITES` | `sync_writes` | `1` fsyncs local repository files after each atomic rename |
| `HITSTOR_LOG_MAX_ENTRIES`, `HITSTOR_LOG_FILE_ENABLED`, `HITSTOR_LOG_FILE_PATH`, `HITSTOR_LOG_ROTATE_MAX_MB`, `HITSTOR_LOG_ROTATE_MAX_FILES` | `log_*` | Log retention and rotation (also read by the agent); see [Configuration Reference](config-reference.md#logging) |
| `PORT` | -- | When set, `hitstor serve` serves gRPC and the dashboard together on this single port (for platforms that expose one port per container) |

### Docker image and Linux installer

These are read by the container entrypoint or the `.run` installer, not by the `hitstor` binary:

| Variable | Read by | Description |
|----------|---------|-------------|
| `HITSTOR_DASHBOARD_PASSWORD` | Docker entrypoint, server `.run --no-wizard` | Dashboard setup password, written to `serve_dashboard_password` |
| `HITSTOR_AUTO_INIT` | Docker entrypoint | `false` skips the automatic `hitstor init` of `HITSTOR_REPO` (default `true`) |
| `HITSTOR_DATA_DIR`, `HITSTOR_GRPC_PORT`, `HITSTOR_DASHBOARD_PORT`, `HITSTOR_ADVERTISE_ADDR` | Server `.run --no-wizard` | Data directory, ports, and advertise address; see [Linux Quick Install](../installation/linux-installer.md#non-interactive-install) |

## Agent (`hitstor-agent`)

| Variable | Config Key | Description |
|----------|-----------|-------------|
| `HITSTOR_AGENT_LISTEN_PORT` | -- | gRPC listen port for dashboard management (default: 8422; `--listen-port` on `daemon`) |
| `HITSTOR_AGENT_SERVER` | `agent_server` | Remote server address (`host:port`) — set automatically in `agent.env` for dashboard-managed agents |
| `HITSTOR_AGENT_API_KEY` | `agent_api_key` | Agent API key for authentication — set automatically in `agent.env` for dashboard-managed agents |
| `HITSTOR_AGENT_CA_CERT` | `agent_ca_cert` | CA certificate for verifying the server — set in `agent.env` at registration when the server sends its certificate (saved as `server-ca.crt`) |
| `HITSTOR_MSSQL_PASSWORD` | -- | Read by the `mssql` workload plugin: the SQL login password for instances without saved settings (the plugin's `password_env` setting names this variable) |

On Windows, the agent service reads these from `%ProgramData%\HitStor\agent.env`; on Linux, systemd loads `/etc/hitstor/agent.env` into the service environment.

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
docker run -e HITSTOR_REPO=/var/lib/hitstor/repo \
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

`HITSTOR_M365_*`, `HITSTOR_GCP_PROJECT`, and `HITSTOR_AWS_REGION` are **no
longer read**: SaaS and cloud-VM backup were removed from the product in an
earlier release. Setting them has no effect.

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
| `HITSTOR_AWS_ROLE_ARN` | | Fallback for `HITSTOR_S3_ROLE_ARN` |
| `HITSTOR_AZURE_CONTAINER` | | Azure Blob container to lock (when `BACKEND_TYPE=azure`) |
| `HITSTOR_AZURE_STORAGE_ACCOUNT` | | Azure storage account name |
| `HITSTOR_AZURE_TENANT` | | Azure AD tenant ID for the retention-lock API call |
| `HITSTOR_AZURE_CLIENT` | | Azure AD client ID for the retention-lock API call |
| `HITSTOR_AZURE_SUB` | | Azure subscription ID for the retention-lock API call |

The retention lock only fires when Sentinel detects a threat above the
confidence threshold — it does not run on every backup. See
[Ransomware Sentinel](../admin-guide/sentinel.md).

## Notes

- The repository password is taken from `--password`, then `HITSTOR_PASSWORD`, then `--password-file`, then the `password_file` of the repository selected with `--repo-name`. There is no `HITSTOR_PASSWORD_FILE` variable.
- `s3_access_key`, `s3_secret_key`, and `serve_dashboard_password` do not have environment variables read by the `hitstor` binary -- use the config file or CLI flags.

## See Also

- [Configuration Reference](config-reference.md) -- all config keys
- [Configuration Guide](../user-guide/configuration.md) -- usage tutorial
