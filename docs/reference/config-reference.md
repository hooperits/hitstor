# Configuration Reference

Complete reference for all HitStor configuration keys. Configuration is stored in YAML format. Without `--config`, HitStor uses `/etc/hitstor/config.yaml` when it exists and is readable, and otherwise `~/.config/hitstor/config.yaml` (`%AppData%\hitstor\config.yaml` on Windows). The Windows server service uses `%ProgramData%\HitStor\config.yaml`.

## Precedence

Values are resolved in this order (highest priority first):

1. **CLI flag** -- `--repo /path`
2. **Config file** -- `repo: /path`
3. **Environment variable** -- `HITSTOR_REPO=/path` (used only when neither a flag nor the config file sets the value)
4. **Default value**

## Configuration Keys

### Core Settings

| Key | Type | Default | CLI Flag | Env Var | Description |
|-----|------|---------|----------|---------|-------------|
| `repo` | string | | `--repo` | `HITSTOR_REPO` | Repository location (local path, `s3:endpoint/bucket`, or `plugin:name/args`) |
| `password_file` | string | | `--password-file` | | Path to file containing the repository password |
| `parallel` | int | `0` | `--parallel` | | Number of parallel workers (`0` = auto-detect based on CPU count) |
| `exclude` | list | | | | Global exclude patterns (applied to all backups) |
| `sync_writes` | bool | `false` | | `HITSTOR_SYNC_WRITES=1` | Fsync local repository files after each atomic rename (durability over speed) |
| `min_free_reserve_bytes` | int | `1073741824` (1 GiB) | | | Minimum free disk space required before writing pack files to a local repository |

### S3 Backend

| Key | Type | Default | CLI Flag | Description |
|-----|------|---------|----------|-------------|
| `s3_access_key` | string | | `--s3-access-key` | S3 access key (sensitive -- masked in `config show`) |
| `s3_secret_key` | string | | `--s3-secret-key` | S3 secret key (sensitive -- masked in `config show`) |
| `s3_region` | string | `us-east-1` | `--s3-region` | S3 region |

### Server Settings

Used by `hitstor serve` to configure the gRPC server and web dashboard.

| Key | Type | Default | CLI Flag | Description |
|-----|------|---------|----------|-------------|
| `serve_port` | int | `8420` | `--port` | gRPC server listen port |
| `serve_tls_cert` | string | | `--tls-cert` | Path to TLS certificate file |
| `serve_tls_key` | string | | `--tls-key` | Path to TLS private key file |
| `serve_dashboard_port` | int | `8421` | `--dashboard-port` | HTTP dashboard listen port |
| `serve_dashboard_password` | string | | | Dashboard setup password (sensitive). It signs in while the server has no repository and is asked for once to create the first administrator; after that, people sign in with their own accounts. If empty, a random one is generated at startup and logged once; see [Dashboard Password and First Administrator](../installation/quickstart.md#dashboard-password) |
| `serve_dashboard_language` | string | | | Default dashboard language code (`en`, `es`). Empty uses the browser language; users can still switch with the selector. See [Translations](../dev-guide/translations.md#choosing-the-language) |
| `serve_advertise_addr` | string | | `--advertise-addr` | Address advertised to agents for registration (use when server is behind NAT) |
| `update_check` | bool | `false` | | Enable automatic version checks. When enabled, the server checks for updates every 24 hours and shows a banner in the dashboard. Can also be turned on under **Settings → General**. |
| `update_url` | string | | | Custom update check URL (empty = GitHub Releases API) |

### Alerting

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `alert_webhook_url` | string | | Webhook URL for alert notifications (receives POST with JSON payload) |
| `alert_stale_threshold` | int | `25` | Hours before an agent is considered stale (minimum: 1) |
| `alert_notifier_plugins` | list | | Notifier plugin names to load for alert delivery |

### Agent Settings

Used by `hitstor-agent` to configure the remote backup agent.

| Key | Type | Default | CLI Flag | Env Var | Description |
|-----|------|---------|----------|---------|-------------|
| `agent_server` | string | | `--server` | `HITSTOR_AGENT_SERVER` | Remote server address (`host:port`) |
| `agent_api_key` | string | | `--api-key` | `HITSTOR_AGENT_API_KEY` | Agent API key for authentication (sensitive -- 64-char hex) |
| `agent_ca_cert` | string | | `--ca-cert` | | Path to CA certificate for TLS verification |
| `agent_hostname` | string | | `backup --host` | | Override hostname sent to server (defaults to system hostname) |
| `agent_paths` | list | | | | Default backup paths (used when no paths given on command line) |
| `agent_excludes` | list | | | | Default exclude patterns for agent backups |

### Plugin Settings

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `plugin_dir` | string | | Directory to search for plugin binaries (searched before `$PATH`) |
| `hook_plugins` | list | | Hook plugin names to load for pre/post-backup lifecycle events |
| `fs_scanner_plugin` | string | | Filesystem scanner plugin name (`builtin` or a custom `Scanner` plugin; only one active at a time) |
| `plugin_store_dir` | string | `/var/lib/hitstor/plugins` (Linux), `C:\ProgramData\HitStor\plugins` (Windows) | Agent: directory where workload plugin packages are installed |
| `plugin_trusted_keys` | list | | Agent: extra base64 ed25519 public keys trusted to sign plugin packages, in addition to the official HitStor key |
| `plugin_allow_unsigned` | bool | `false` | Agent: accept unsigned plugin packages. Development only; packages signed by an untrusted key are still rejected |

### Integrations (`integrations`)

Used by Sentinel (Gemini). Sensitive values are stored encrypted (AES-256-GCM,
base64) using the repository encryption key. Editable from the dashboard
**Settings → Integrations** tab.

| Key | Type | Description |
|-----|------|-------------|
| `gemini_api_key_encrypted` | string | Encrypted Gemini API key for Sentinel (and `GEMINI_API_KEY` env export) |
| `gemini_model` | string | Gemini model (default: `gemini-1.5-pro`) |

`GEMINI_API_KEY` still works and takes precedence when set. Use **Import from
environment** in Settings to migrate it to encrypted YAML.

#### Ignored legacy keys

Microsoft 365 (`m365_*`), cloud provider (`gcp_project`, `aws_role_arn`,
`aws_region`, `azure_tenant`, `azure_client`, `azure_sub`), and
`collector_plugins` keys were removed from the config schema in an earlier release. Existing
`config.yaml` files containing them still load — YAML silently ignores unknown
keys — but the values are never read. Remove these keys at your convenience.

### Email (`email`)

The outgoing mail server for account emails: invitations to new users and
password reset links. Secrets are stored encrypted with the repository key.
Editable from the dashboard **Settings → Email** tab; see
[Users and access](../admin-guide/users.md#email).

| Key | Type | Description |
|-----|------|-------------|
| `host` | string | SMTP server. Empty turns account emails off |
| `port` | int | Port (default: 587 for `starttls`, 465 for `tls`, 25 for `none`) |
| `security` | string | `starttls`, `tls` or `none` (plain SMTP, for a relay on a trusted network) |
| `auth` | string | `none` (the server accepts mail by client address), `password` or `microsoft365` (OAuth 2.0 app) |
| `username` | string | `password`: user name. `microsoft365`: mailbox to sign in as, if not `from` |
| `password_encrypted` | string | `password`: encrypted password |
| `tenant_id` | string | `microsoft365`: directory (tenant) ID |
| `client_id` | string | `microsoft365`: application (client) ID |
| `client_secret_encrypted` | string | `microsoft365`: encrypted client secret |
| `from` | string | Sender address |
| `from_name` | string | Sender name |
| `tls_skip_verify` | bool | Accept any server certificate (self-signed relays only) |
| `dashboard_url` | string | The dashboard's address as people reach it, the base of the links in emails |

### Reports (`reports`) { #reports-reports }

The reports emailed on a schedule, a list. Editable from **Administration → Reports**; see [Reports](../admin-guide/reports.md).

| Key | Type | Description |
|-----|------|-------------|
| `id` | string | Identifier |
| `name` | string | Name shown in the list and the email subject |
| `type` | string | `summary` (protection summary) or `daily` (daily backup status) |
| `period` | string | `summary`: `week` or `month` (last 7 or 30 days) |
| `cron` | string | When to send it (cron expression) |
| `timezone` | string | Time zone of the schedule and of the dates in the report |
| `recipients` | list | Email addresses |
| `agents` | list | Agents covered; empty: every agent |
| `language` | string | `en` or `es`; empty: the dashboard's default |
| `enabled` | bool | Send it on schedule |
| `created`, `last_sent`, `last_error` | | Kept by the server |

### Logging

Server and agent log retention: the in-memory buffer behind the dashboard's **Logs** page and the rotated log file. Each key has an environment variable that overrides it.

| Key | Type | Default | Env Var | Description |
|-----|------|---------|---------|-------------|
| `log_max_entries` | int | `5000` | `HITSTOR_LOG_MAX_ENTRIES` | Entries kept in memory (500-50000) |
| `log_file_enabled` | bool | `true` | `HITSTOR_LOG_FILE_ENABLED` | Write the log file |
| `log_file_path` | string | `/var/log/hitstor/server.log` (`C:\ProgramData\HitStor\logs\server.log` on Windows); the agent writes `agent.log` in the same folder | `HITSTOR_LOG_FILE_PATH` | Log file path |
| `log_rotate_max_mb` | int | `50` | `HITSTOR_LOG_ROTATE_MAX_MB` | Rotate the file at this size (at most 500) |
| `log_rotate_max_files` | int | `5` | `HITSTOR_LOG_ROTATE_MAX_FILES` | Rotated files kept (at most 20) |

### Sentinel (`sentinel`)

Optional Sentinel telemetry settings (future dashboard editing). Keys mirror `HITSTOR_SENTINEL_*` environment variables.

## Multi-Repository (`repositories`)

When running `hitstor serve` with multiple repositories, define a list instead of a single `repo`:

```yaml
repositories:
  - name: production
    type: local
    path: /var/lib/hitstor/prod-repo
    password_file: /etc/hitstor/prod-password

  - name: offsite
    type: s3
    endpoint: s3.amazonaws.com
    bucket: company-backups
    region: us-east-1
    access_key_encrypted: <base64 AES-256-GCM ciphertext>
    secret_key_encrypted: <base64 AES-256-GCM ciphertext>
    password_file: /etc/hitstor/s3-repo-password
```

S3 credentials are stored encrypted in the config file. The dashboard Storage page can create repositories without hand-editing ciphertext.

If `repositories` is empty and `repo` is set, the server runs in single-repository legacy mode.

## Key Types

| Type | Description | Management Commands |
|------|-------------|-------------------|
| **string** | Single value | `config set`, `config unset` |
| **int** | Integer value | `config set`, `config unset` |
| **list** | Ordered list of values | `config add`, `config remove` |

## Example Configuration

### Local Server

```yaml
repo: /var/lib/hitstor/repo
parallel: 4
exclude:
  - "*.tmp"
  - ".cache"
serve_port: 8420
serve_tls_cert: /etc/hitstor/tls/cert.pem
serve_tls_key: /etc/hitstor/tls/key.pem
serve_dashboard_port: 8421
alert_webhook_url: https://hooks.slack.com/services/T.../B.../xxx
alert_stale_threshold: 25
```

### Remote Agent

```yaml
agent_server: backup.internal:8420
agent_api_key: a1b2c3d4e5f6...  # 64-char hex
agent_ca_cert: /etc/hitstor/ca.pem
agent_paths:
  - /etc
  - /home
  - /var/lib
agent_excludes:
  - "*.log"
  - ".cache"
```

### S3 Backend

```yaml
repo: s3:https://s3.amazonaws.com/my-backup-bucket
s3_access_key: AKIAIOSFODNN7EXAMPLE
s3_secret_key: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
s3_region: us-west-2
```

## See Also

- [Configuration Guide](../user-guide/configuration.md) -- usage tutorial
- [Environment Variables](environment-variables.md) -- all environment variables
