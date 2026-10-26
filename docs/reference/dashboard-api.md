# Dashboard HTTP API

The embedded dashboard served by `hitstor serve` exposes a JSON API on the HTTP port (default **8421**). Routes are defined in `internal/dashboard/dashboard.go`.

Unless noted, endpoints require dashboard authentication when `serve_dashboard_password` is set. Session cookies are issued via `POST /login`.

## Pages

| Path | Description |
|------|-------------|
| `GET /` | Overview |
| `GET /backups` | Backup jobs list |
| `GET /backups/{id}` | Backup job detail |
| `GET /agents` | Agents |
| `GET /agents/{name}` | Agent detail |
| `GET /snapshots/{id}` | Snapshot detail (browse + restore) |
| `GET /storage` | Repository management |
| `GET /storage/{name}` | Repository detail |
| `GET /restores` | Restore sessions |
| `GET /settings` | Server settings (General, Security/TLS, Network, Integrations, Notifications) |

`GET /snapshots` redirects to `/storage`.

## Agents

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/agents` | List agents with status and schedule info |
| `POST` | `/api/agents` | Register agent (manual entry: `name`, `agent_addr`, optional `api_key`) |
| `DELETE` | `/api/agents/{name}` | Remove agent |
| `POST` | `/api/agents/{name}/schedule` | Set schedule (interval, cron, timezone, jobs) |
| `POST` | `/api/agents/{name}/repo` | Set target repository |
| `POST` | `/api/agents/{name}/browse` | Browse agent filesystem |
| `POST` | `/api/agents/{name}/configure` | Set backup paths and excludes |
| `GET` | `/api/agents/{name}/info` | Agent info |
| `POST` | `/api/agents/{name}/suspend` | Suspend agent backups |
| `POST` | `/api/agents/{name}/unsuspend` | Unsuspend agent |

## Agent Discovery

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/discover/subnets` | List local subnets for scanning |
| `POST` | `/api/discover/scan` | Scan subnet for agents |
| `POST` | `/api/discover/probe` | Probe a specific host:port |

## Backup Jobs

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/backups` | List backup jobs |
| `POST` | `/api/backups` | Create job (`job_type` is always `files`) |
| `GET` | `/api/backups/{id}` | Get job |
| `PUT` | `/api/backups/{id}` | Update job |
| `DELETE` | `/api/backups/{id}` | Delete job |
| `POST` | `/api/backups/{id}/run` | Run job now |
| `POST` | `/api/backups/{id}/cancel` | Cancel running backup |
| `GET` | `/api/backups/validate-cron` | Validate cron expression |
| `GET` | `/api/timezones` | List supported timezones |

## Repositories & Storage

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/repos` | List repositories |
| `POST` | `/api/repos` | Create repository (local or S3) |
| `DELETE` | `/api/repos/{name}` | Disconnect repository |
| `GET` | `/api/disk-space?path=` | Disk space for a path |
| `POST` | `/api/repos/test-s3` | Test S3 credentials |
| `POST` | `/api/repos/detect` | Detect existing repo at path |
| `GET` | `/api/repos/{name}/growth` | Growth chart data (`?range=`, `?granularity=`) |
| `POST` | `/api/repos/{name}/check` | Start integrity check |
| `POST` | `/api/repos/{name}/prune` | Start prune |
| `GET` | `/api/repos/{name}/sessions` | Active sessions for repo |

## Snapshots & Restore

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/snapshots?host=&limit=` | List snapshots (newest first) |
| `GET` | `/api/snapshots/{id}/tree?path=` | Browse snapshot file tree |
| `POST` | `/api/snapshots/{id}/restore` | Start agent restore (`agent_name`, `target_path`, optional `includes`) |
| `GET` | `/api/restores` | List restore sessions |
| `GET` | `/api/restore/{id}/events` | SSE restore progress |

## Progress & Operations

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/stats` | Repository statistics |
| `GET` | `/api/active-backups` | Active backup sessions |
| `GET` | `/api/backup-progress/events` | SSE backup progress (all sessions) |
| `GET` | `/api/operations/{id}/events` | SSE for check/prune operations |

## Settings

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/settings` | Effective server settings (secrets masked) |
| `PUT` | `/api/settings/general` | `update_check`, `update_url`, `parallel`, `sync_writes`, `min_free_reserve_bytes` |
| `PUT` | `/api/settings/security` | `dashboard_password` (empty = keep current); may set `restart_required` |
| `PUT` | `/api/settings/network` | `serve_port`, `serve_dashboard_port`, `serve_advertise_addr`; may set `restart_required` |
| `PUT` | `/api/settings/notifications` | `alert_webhook_url`, `alert_stale_threshold`, `alert_notifier_plugins` |
| `PUT` | `/api/settings/integrations` | Gemini key/model (`gemini_api_key` empty = keep current) |
| `POST` | `/api/settings/integrations/import-env` | Migrate `GEMINI_API_KEY` into encrypted YAML |
| `GET` | `/api/settings/tls/status` | TLS certificate metadata (source, expiry, SANs) |
| `POST` | `/api/settings/tls/upload` | Multipart `cert` + `key` PEM files; sets `restart_required` |
| `POST` | `/api/settings/test/gemini` | Validate Gemini API key |
| `POST` | `/api/settings/test/webhook` | POST test payload to configured webhook URL |

## Alerts (Sentinel)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/alerts` | List sentinel/security alerts |
| `POST` | `/api/alerts/{id}/resolve` | Resolve alert |

## Metrics (unauthenticated)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/metrics` | Prometheus text exposition |

## Authentication

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/login` | Login page |
| `POST` | `/login` | Submit password |
| `POST` | `/logout` | End session |

## See Also

- [gRPC API](grpc-api.md) — agent backup protocol
- [Dashboard](../admin-guide/dashboard.md) — UI overview
- [Backup Jobs](../admin-guide/backup-jobs.md) — job types and workflows