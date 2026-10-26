# Ransomware Sentinel

HitStor Sentinel (`internal/sentinel`) monitors backup session telemetry for ransomware-like behavior using **Google Gemini**. When risk is elevated, it can alert administrators and apply protective actions such as agent suspension and storage retention locks.

## How it works

After every backup session finalizes, the server:

1. Gathers up to 10 prior snapshots for the same host as a baseline.
2. Sends the current session's telemetry plus that baseline to Gemini for analysis.
3. If the analysis reports a threat with confidence at or above `HITSTOR_SENTINEL_THRESHOLD` (default `0.8`), the server:
   - Suspends the agent's credentials, blocking further backups from that host.
   - Locks the configured storage bucket with an immutable retention hold, so ransomware cannot delete historical snapshots even with valid credentials.
   - Fires an alert, visible via `GET /api/alerts` and any configured webhook/notifier.

This only runs the containment steps above the confidence threshold — most backups complete with Sentinel silently finding nothing.

## Telemetry Signals

- FastCDC compression ratios (encrypted/random data compresses poorly)
- Mass file change or deletion rates
- Session-level anomaly patterns compared to the host's own history

## Configuration

Sentinel is enabled by default when the server starts. It is configured entirely through environment variables read at `hitstor serve` startup — there is no `hitstor config set` equivalent. The Gemini API key/model fall back to the encrypted `integrations.*` values in `config.yaml` (set via **Settings → Integrations**) when the Sentinel-specific variables are unset.

See [Environment Variables → Sentinel](../reference/environment-variables.md#sentinel-server) for the full variable list, defaults, and the three supported retention-lock backends (GCS, S3, Azure Blob).

Configure alerting alongside existing webhook and notifier plugins:

```yaml
alert_webhook_url: https://hooks.slack.com/services/...
alert_notifier_plugins:
  - slack
plugin_dir: /usr/local/lib/hitstor/plugins
```

## Dashboard Alerts

Active sentinel alerts appear via `GET /api/alerts`. Resolve handled incidents with `POST /api/alerts/{id}/resolve`.

## Retention Lock

For supported object storage backends (GCS, S3, Azure Blob), Sentinel can trigger immutable retention locks on the backup bucket to prevent ransomware from deleting historical snapshots — even with valid, unrevoked credentials. This is separate from the repository's own backend (local/S3); the lock targets whichever bucket `HITSTOR_SENTINEL_BACKEND_TYPE` and its bucket variable point at.

## See Also

- [Monitoring](monitoring.md)
- [Security](security.md)
- [Environment Variables](../reference/environment-variables.md)
