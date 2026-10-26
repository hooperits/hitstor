# Server Settings

The **Settings** page (`/settings`) centralizes server configuration that was previously split across environment variables, YAML files, and CLI commands. It follows the same patterns as enterprise backup consoles: grouped sections, masked secrets, test-before-save, and a restart banner when network or TLS changes require it.

## Access

Open **System → Settings** in the dashboard sidebar, or use a deep link (`/settings?tab=integrations`).

## Sections

| Tab | What you configure |
|-----|-------------------|
| **General** | Update checks, parallel workers, sync writes, minimum free disk reserve |
| **Security & TLS** | Dashboard password, TLS certificate upload, certificate status |
| **Network** | gRPC port, dashboard HTTPS port, advertise address for agents |
| **Integrations** | Gemini API key (Sentinel) |
| **Notifications** | Alert webhook, stale-agent threshold |

## Integrations

Configure integrations from **Settings → Integrations**:

1. **Gemini API key** — encrypted at rest in `config.yaml`; used by [Ransomware Sentinel](sentinel.md) when `HITSTOR_SENTINEL_API_KEY` is not set
2. **Gemini model** — defaults to `gemini-1.5-pro` (or Sentinel default when unset)

Gemini is the only integration. SaaS and cloud-provider integrations were
removed in v2.0.0 along with the workloads they served.

Use **Test Gemini** before saving. If you already have a value in an environment
variable, click **Import from environment** to migrate it into encrypted YAML.

## TLS certificates

Upload a PEM certificate and private key from **Security & TLS**. Files are stored under `/etc/hitstor/tls/` and paths are written to `serve_tls_cert` and `serve_tls_key` in the config file.

**Restart required** after upload — TLS is loaded at server startup.

The status card shows source (custom, repository auto-generated, or self-signed), subject, issuer, expiry, and SANs.

## Notifications

`alert_stale_threshold` (default 25 hours) controls when agents are marked stale for alerting. Configure from **Settings → Notifications**.

## API

See [Dashboard HTTP API — Settings](../reference/dashboard-api.md#settings).

## Related

- [Sentinel](sentinel.md)
- [TLS Certificates](tls-certificates.md)
- [Configuration Reference](../reference/config-reference.md)