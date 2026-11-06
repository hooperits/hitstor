# Monitoring and Alerting

HitStor provides Prometheus metrics and webhook alerting to integrate with existing monitoring infrastructure.

## Prometheus Metrics

The server exposes a Prometheus-compatible metrics endpoint at:

```
https://SERVER_IP:8421/metrics
```

This endpoint is unauthenticated, allowing Prometheus to scrape without credential configuration. Restrict access at the network level if needed. It exists once the server has a repository; repository metrics describe the server's first repository.

### Available Metrics

#### Repository and Agent Gauges

| Metric | Type | Description |
|--------|------|-------------|
| `hitstor_snapshots_total` | gauge | Total snapshots in the repository |
| `hitstor_packs_total` | gauge | Total pack files in the repository |
| `hitstor_indexes_total` | gauge | Total index files in the repository |
| `hitstor_repository_size_bytes` | gauge | Total repository size in bytes |
| `hitstor_active_sessions` | gauge | Current active backup sessions |
| `hitstor_registered_agents` | gauge | Total registered agents |
| `hitstor_agents_ok` | gauge | Agents with a backup in the last 25 hours |
| `hitstor_agents_stale` | gauge | Agents whose last backup is older than 25 hours |
| `hitstor_agents_never` | gauge | Agents that have never backed up |

#### Counters

Counters reset when the server restarts.

| Metric | Type | Description |
|--------|------|-------------|
| `hitstor_backups_completed_total` | counter | Successful backups |
| `hitstor_backups_failed_total` | counter | Failed or aborted backup sessions |
| `hitstor_bytes_received_total` | counter | Bytes received from agents |
| `hitstor_chunks_received_total` | counter | Chunks received from agents |
| `hitstor_checks_total` | counter | Integrity checks run from the dashboard |
| `hitstor_check_errors_total` | counter | Errors found by those checks |
| `hitstor_prunes_total` | counter | Prune operations run from the dashboard |
| `hitstor_prune_freed_bytes_total` | counter | Bytes freed by those prunes |

#### Per-Agent Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `hitstor_agent_last_backup_timestamp_seconds` | gauge | `agent` | Unix timestamp of the agent's last successful backup (0 if never) |
| `hitstor_agent_schedule_enabled` | gauge | `agent` | Legacy per-agent schedule flag (1/0); backup jobs keep their own schedules |
| `hitstor_agent_schedule_interval_hours` | gauge | `agent` | Legacy per-agent interval in hours |

### Prometheus Scrape Configuration

Add the following to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: "hitstor"
    scrape_interval: 30s
    scheme: https
    tls_config:
      ca_file: /etc/prometheus/hitstor-server.crt  # the server's certificate or CA
    static_configs:
      - targets: ["backup.example.com:8421"]
```

For multiple HitStor servers:

```yaml
scrape_configs:
  - job_name: "hitstor"
    scrape_interval: 30s
    scheme: https
    tls_config:
      ca_file: /etc/prometheus/hitstor-ca.crt
    static_configs:
      - targets:
          - "backup-01.example.com:8421"
          - "backup-02.example.com:8421"
```

### Grafana Dashboard

To build a Grafana dashboard for HitStor:

1. Add your Prometheus instance as a data source in Grafana.
2. Create a new dashboard with panels for key metrics:

**Suggested panels:**

| Panel | Query | Visualization |
|-------|-------|---------------|
| Healthy Agents | `hitstor_agents_ok` | Stat |
| Stale Agents | `hitstor_agents_stale` | Stat (red threshold > 0) |
| Total Snapshots | `hitstor_snapshots_total` | Stat |
| Storage Used | `hitstor_repository_size_bytes` | Stat (unit: bytes) |
| Running Backups | `hitstor_active_sessions` | Stat |
| Backup Rate | `rate(hitstor_backups_completed_total[1h])` | Time series |
| Failed Backups | `rate(hitstor_backups_failed_total[1h])` | Time series (alert threshold) |
| Data Ingestion | `rate(hitstor_bytes_received_total[5m])` | Time series (unit: bytes/s) |
| Last Backup per Agent | `hitstor_agent_last_backup_timestamp_seconds` | Table |

3. Set up Grafana alerts on `hitstor_agents_stale > 0` and `rate(hitstor_backups_failed_total[1h]) > 0`.

## Webhook Alerting

HitStor can send webhook notifications when backup sessions fail, agents go stale, or Sentinel detects a threat. Each event is a JSON `POST` with `Content-Type: application/json`.

### Configuration

Set the webhook URL in the config file:

```yaml
# /etc/hitstor/config.yaml
alert_webhook_url: "https://hooks.example.com/hitstor"
alert_stale_threshold: 25
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `alert_webhook_url` | string | (none) | URL to POST alert payloads to |
| `alert_stale_threshold` | integer | 25 | Hours without a backup before an agent is considered stale |
| `alert_notifier_plugins` | list | (none) | [Notifier plugins](#notifier-plugins) to load |

These keys can also be set under **Settings → Notifications**, which can send a test event. They are read at startup: restart the server after changing them.

### Alert Events

Every event has the same fields: `event`, `server` (always `hitstor`), `timestamp`, `agent`, `hostname` (when known), `message`, and `details`.

#### `backup_failed`

Sent immediately when a backup session fails on the server: the agent disconnects during the upload, or the snapshot cannot be finalized. Failures the agent reports before uploading (for example, a database that cannot be read) show on the job page and in [reports](reports.md), but do not send a webhook.

```json
{
  "event": "backup_failed",
  "server": "hitstor",
  "timestamp": "2026-02-21T03:05:00Z",
  "agent": "web-server-01",
  "hostname": "web-server-01",
  "message": "backup failed: agent disconnected during upload",
  "details": {"session_id": "3f9a...", "chunks_received": 1520}
}
```

#### `agent_stale`

Sent when an agent has not completed a backup within the stale threshold. The server checks every 5 minutes; agents that have never backed up are not reported.

```json
{
  "event": "agent_stale",
  "server": "hitstor",
  "timestamp": "2026-02-21T14:00:00Z",
  "agent": "staging-app",
  "message": "no backup in 2d1h (threshold: 1d1h)",
  "details": {"last_backup": "2026-02-19T12:30:00Z", "threshold_hours": 25}
}
```

#### `threat_detected`

Sent by [Ransomware Sentinel](sentinel.md) when it reports a threat at or above its confidence threshold.

### Cooldown and Recovery

To avoid alert fatigue, HitStor applies cooldown logic:

- After sending a stale alert for an agent, it does not re-alert until the threshold duration has elapsed again.
- When a stale agent completes a successful backup, the alert state is cleared. If the agent goes stale again, a new alert is sent.
- Failed backup alerts have no cooldown -- each failure triggers a notification.

## Notifier Plugins

For custom notification delivery (Slack, PagerDuty, email, etc.), use notifier plugins instead of or in addition to the webhook.

Configure notifier plugins in the config file:

```yaml
# /etc/hitstor/config.yaml
plugin_dir: /usr/lib/hitstor/plugins
alert_notifier_plugins:
  - slack
  - pagerduty
```

Each name maps to a binary `hitstor-notifier-<name>`, looked up in `plugin_dir` and then in `PATH`. Notifier plugins receive the same events as the webhook (over the plugin gRPC interface) and can transform them for their target system. See the [Plugins](../user-guide/plugins.md) documentation for details on writing and installing plugins.

## Next Steps

- [Dashboard](dashboard.md) -- view metrics and agent status in the web UI
- [Scheduling](scheduling.md) -- configure backup schedules to keep agents active
- [Troubleshooting](troubleshooting.md) -- diagnose stale agents and failed backups
