# Configuration Reference

Complete reference for all HitStor configuration keys. Configuration is stored in YAML format at `~/.config/hitstor/config.yaml` (Linux) or `%AppData%\hitstor\config.yaml` (Windows).

## Precedence

Values are resolved in this order (highest priority first):

1. **CLI flag** -- `--repo /path`
2. **Environment variable** -- `HITSTOR_REPO=/path`
3. **Config file** -- `repo: /path`
4. **Default value**

## Configuration Keys

### Core Settings

| Key | Type | Default | CLI Flag | Env Var | Description |
|-----|------|---------|----------|---------|-------------|
| `repo` | string | | `--repo` | `HITSTOR_REPO` | Repository location (local path, `s3:endpoint/bucket`, or `plugin:name/args`) |
| `password_file` | string | | `--password-file` | | Path to file containing the repository password |
| `parallel` | int | `0` | `--parallel` | | Number of parallel workers (`0` = auto-detect based on CPU count) |
| `exclude` | list | | | | Global exclude patterns (applied to all backups) |

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
| `serve_dashboard_password` | string | | | Dashboard login password (sensitive -- if empty, a random one is generated at startup and logged once; see [Dashboard Password](../installation/quickstart.md#dashboard-password)) |
| `serve_dashboard_language` | string | | | Default dashboard language code (`en`, `es`). Empty uses the browser language; users can still switch with the selector. See [Translations](../dev-guide/translations.md#choosing-the-language) |
| `serve_advertise_addr` | string | | `--advertise-addr` | Address advertised to agents for registration (use when server is behind NAT) |
| `update_check` | bool | `true` | | Enable automatic version checks. When enabled, the server checks for updates every 24 hours and shows a banner in the dashboard. |
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
| `agent_hostname` | string | | `--hostname` | | Override hostname sent to server (defaults to system hostname) |
| `agent_paths` | list | | | | Default backup paths (used when no paths given on command line) |
| `agent_excludes` | list | | | | Default exclude patterns for agent backups |

### Plugin Settings

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `plugin_dir` | string | | Directory to search for plugin binaries (searched before `$PATH`) |
| `hook_plugins` | list | | Hook plugin names to load for pre/post-backup lifecycle events |
| `fs_scanner_plugin` | string | | Filesystem scanner plugin name (`builtin` or a custom `Scanner` plugin; only one active at a time) |

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
`collector_plugins` keys were removed from the config schema in v2.0.0. Existing
`config.yaml` files containing them still load — YAML silently ignores unknown
keys — but the values are never read. Remove these keys at your convenience.

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
serve_dashboard_password: changeme
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
  - "/home/*/.cache"
```

### S3 Backend

```yaml
repo: s3:s3.amazonaws.com/my-backup-bucket
s3_access_key: AKIAIOSFODNN7EXAMPLE
s3_secret_key: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
s3_region: us-west-2
```

## See Also

- [Configuration Guide](../user-guide/configuration.md) -- usage tutorial
- [Environment Variables](environment-variables.md) -- all environment variables
