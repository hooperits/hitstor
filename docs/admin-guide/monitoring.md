# Monitoring and Alerting

HitStor provides Prometheus metrics and webhook alerting to integrate with existing monitoring infrastructure.

## Prometheus Metrics

The server exposes a Prometheus-compatible metrics endpoint at:

```
http://SERVER_IP:8421/metrics
```

This endpoint is unauthenticated, allowing Prometheus to scrape without credential configuration. Restrict access at the network level if needed.

### Available Metrics

HitStor exports 14 metrics: 9 gauges, 4 counters, and 1 per-agent metric.

#### Gauges

| Metric | Type | Description |
|--------|------|-------------|
| `hitstor_agents_total` | gauge | Total number of registered agents |
| `hitstor_agents_active` | gauge | Number of agents with recent backups |
| `hitstor_agents_stale` | gauge | Number of agents past the stale threshold |
| `hitstor_snapshots_total` | gauge | Total number of snapshots in the repository |
| `hitstor_storage_bytes` | gauge | Total repository size in bytes |
| `hitstor_storage_raw_bytes` | gauge | Total size of original (pre-dedup) data in bytes |
| `hitstor_dedup_ratio` | gauge | Deduplication ratio (raw / stored) |
| `hitstor_packs_total` | gauge | Number of pack files in the repository |
| `hitstor_uptime_seconds` | gauge | Server uptime in seconds |

#### Counters

| Metric | Type | Description |
|--------|------|-------------|
| `hitstor_backups_total` | counter | Total number of completed backups since server start |
| `hitstor_backups_failed_total` | counter | Total number of failed backups since server start |
| `hitstor_restores_total` | counter | Total number of completed restores since server start |
| `hitstor_bytes_received_total` | counter | Total bytes received from agents since server start |

#### Per-Agent Metric

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `hitstor_agent_last_backup_timestamp` | gauge | `agent` | Unix timestamp of the agent's last successful backup |

### Prometheus Scrape Configuration

Add the following to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: "hitstor"
    scrape_interval: 30s
    static_configs:
      - targets: ["backup.example.com:8421"]
```

For multiple HitStor servers:

```yaml
scrape_configs:
  - job_name: "hitstor"
    scrape_interval: 30s
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
| Active Agents | `hitstor_agents_active` | Stat |
| Stale Agents | `hitstor_agents_stale` | Stat (red threshold > 0) |
| Total Snapshots | `hitstor_snapshots_total` | Stat |
| Storage Used | `hitstor_storage_bytes` | Stat (unit: bytes) |
| Dedup Ratio | `hitstor_dedup_ratio` | Stat |
| Backup Rate | `rate(hitstor_backups_total[1h])` | Time series |
| Failed Backups | `rate(hitstor_backups_failed_total[1h])` | Time series (alert threshold) |
| Data Ingestion | `rate(hitstor_bytes_received_total[5m])` | Time series (unit: bytes/s) |
| Last Backup per Agent | `hitstor_agent_last_backup_timestamp` | Table |

3. Set up Grafana alerts on `hitstor_agents_stale > 0` and `rate(hitstor_backups_failed_total[1h]) > 0`.

## Webhook Alerting

HitStor can send webhook notifications when backup failures or stale agents are detected.

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

### Alert Events

HitStor sends alerts for two event types:

#### `backup_failed`

Sent immediately when a backup fails.

```json
{
  "event": "backup_failed",
  "agent": "web-server-01",
  "timestamp": "2026-02-21T03:05:00Z",
  "error": "connection reset by peer",
  "server": "backup.example.com"
}
```

#### `agent_stale`

Sent when an agent has not completed a backup within the stale threshold.

```json
{
  "event": "agent_stale",
  "agent": "staging-app",
  "timestamp": "2026-02-21T14:00:00Z",
  "last_backup": "2026-02-19T12:30:00Z",
  "threshold_hours": 25,
  "server": "backup.example.com"
}
```

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
alert_notifier_plugins:
  - /usr/lib/hitstor/plugins/slack-notifier
  - /usr/lib/hitstor/plugins/pagerduty-notifier
```

Notifier plugins receive the same JSON payload as the webhook and can transform it for their target system. See the [Plugins](../user-guide/plugins.md) documentation for details on writing and installing plugins.

## Next Steps

- [Dashboard](dashboard.md) -- view metrics and agent status in the web UI
- [Scheduling](scheduling.md) -- configure backup schedules to keep agents active
- [Troubleshooting](troubleshooting.md) -- diagnose stale agents and failed backups
