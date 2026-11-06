# Dashboard HTTP API

The embedded dashboard served by `hitstor serve` exposes a JSON API on the HTTP port (default **8421**). Routes are defined in `internal/dashboard/dashboard.go`.

Unless noted, endpoints require a signed-in session (see [Authentication](#authentication)), and each route needs a role: reads need the viewer role, changes to jobs the backup operator role, restores the restore operator role, and users, storage, settings, and other server-wide changes the administrator. Routes about one agent accept a role on that agent; routes that span agents need the role on every agent. Sensitive changes also need a recent sign-in. See [Users and Access](../admin-guide/users.md#roles).

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
| `GET /settings` | Server settings (General, Security/TLS, Network, Integrations, Notifications, Email, Plugins) |
| `GET /disaster` | Disaster recovery |
| `GET /logs` | Server and agent logs |
| `GET /reports` | Scheduled reports |
| `GET /users`, `GET /users/{name}` | Users and access (administrators) |
| `GET /audit` | Audit log (administrators) |
| `GET /account`, `GET /account/password` | The signed-in user's account and password change |

`GET /snapshots` redirects to `/storage`.

## Agents

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/agents` | List agents with status, schedule info, and installed workload plugins (`plugins`) |
| `POST` | `/api/agents` | Register agent (manual entry: `name`, `agent_addr`, optional `api_key`) |
| `DELETE` | `/api/agents/{name}` | Remove agent |
| `POST` | `/api/agents/{name}/schedule` | Set schedule (interval, cron, timezone, jobs) |
| `POST` | `/api/agents/{name}/repo` | Set target repository |
| `POST` | `/api/agents/{name}/browse` | Browse agent filesystem |
| `POST` | `/api/agents/{name}/configure` | Set backup paths and excludes |
| `GET` | `/api/agents/{name}/info` | Agent info |
| `GET` | `/api/agents/{name}/snapshots` | Snapshots of one agent |
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
| `POST` | `/api/backups` | Create job. `job_type` is `files` (with `paths`) or `workload` (with `plugin`, `selection`, optional `plan` (one `{kind, enabled, cron_expression, timezone}` per backup kind), `backup_kind` and `plugin_settings`; the agent must have reported the plugin) |
| `GET` | `/api/backups/{id}` | Get job |
| `PUT` | `/api/backups/{id}` | Update job |
| `DELETE` | `/api/backups/{id}` | Delete job |
| `POST` | `/api/backups/{id}/run` | Run job now; for workload jobs, `{"kind": "full"}` picks the plan's kind |
| `GET` | `/api/backups/{id}/chains` | Backup chains of each database of a workload job |
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
| `GET` | `/api/repos/{name}`, `/api/repos/{name}/health`, `/api/repos/{name}/snapshots` | One repository's metrics, operational health, and snapshots grouped by agent |
| `GET` | `/api/repos/growth` | Growth data for several repositories in one request |
| `POST` | `/api/repos/{name}/forget` | Apply a retention policy (administrators, recent sign-in) |
| `POST` | `/api/repos/detect-s3`, `/api/repos/validate-path` | Detect an existing repository in a bucket; validate a local path |

## Snapshots & Restore

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/snapshots?host=&limit=` | List snapshots (newest first) |
| `GET` | `/api/snapshots/{id}/tree?path=` | Browse snapshot file tree |
| `POST` | `/api/snapshots/{id}/restore` | Start agent restore (`agent_name`, `target_path`, optional `includes`) |
| `GET` | `/api/restores` | List restore sessions |
| `GET` | `/api/restores/sources` | Sources that can be restored, as listed on the Restores page |
| `GET` | `/api/restore/{id}/events` | SSE restore progress |
| `GET` | `/api/agents/{name}/plugins/{plugin}/browse?path=` | List the objects a workload plugin on the agent can back up, with `detail`, `status`, and whether each takes object settings (`configurable`) and has them saved (`configured`) |
| `GET` | `/api/agents/{name}/plugins/{plugin}/object-config?path=` | Settings saved on the agent for one object (`configured`, `values` without passwords, `secrets_set`) |
| `PUT` | `/api/agents/{name}/plugins/{plugin}/object-config` | Test and save an object's settings on the agent (`path`, `values`; an empty password keeps the saved one), or remove them (`path`, `remove: true`). The server stores nothing |
| `GET` | `/api/workload/points?agent=&plugin=&object=` | Items available to restore a workload object, oldest first |
| `GET` | `/api/workload/catalog?agent=&plugin=&snapshot=` | Every item a plugin backed up on an agent, newest first, with its `object`; `snapshot` (an ID prefix) keeps one snapshot's |
| `POST` | `/api/workload/restore-plan` | Preview a workload restore without running it (same body as `POST /api/workload/restores`): the items it applies, the files it writes (`key`, `kind`, `source`, `target`), `warnings`, and a `summary` |
| `POST` | `/api/workload/restores` | Start a workload restore (`agent_name`, `plugin`, `object`; optional `source_agent` to restore another agent's backups, `target` time, `options`, `plugin_settings`) |
| `GET` | `/api/workload/restores/{id}` | Workload restore status: plan, bytes sent, result |
| `GET` | `/api/plugin-packages` | List uploaded plugin packages |
| `POST` | `/api/plugin-packages` | Upload a plugin package (the `.hpk` file as an `application/octet-stream` body); rejected unless signed by a trusted key |
| `DELETE` | `/api/plugin-packages/{file}` | Delete an uploaded package |
| `POST` | `/api/agents/{name}/plugins` | Install an uploaded package on a managed agent (`package`, optional `upgrade`, `allow_downgrade`); the agent verifies it again |
| `DELETE` | `/api/agents/{name}/plugins/{id}` | Uninstall a plugin from a managed agent; refused while a job uses it |

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
| `PUT` | `/api/settings/email` | Outgoing mail server for invitations, password reset links, and reports (recent sign-in) |
| `POST` | `/api/settings/email/test` | Send a test email with the settings in the request |
| `PUT` | `/api/settings/integrations` | Gemini key/model (`gemini_api_key` empty = keep current) |
| `POST` | `/api/settings/integrations/import-env` | Migrate `GEMINI_API_KEY` into encrypted YAML |
| `GET` | `/api/settings/tls/status` | TLS certificate metadata (source, expiry, SANs) |
| `POST` | `/api/settings/tls/upload` | Multipart `cert` + `key` PEM files; sets `restart_required` |
| `POST` | `/api/settings/test/gemini` | Validate Gemini API key |
| `POST` | `/api/settings/test/webhook` | POST test payload to configured webhook URL |

## Users, Account, and Audit

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/users` | List users (administrators) |
| `POST` | `/api/users` | Create a user: sends an invitation when email is set up, otherwise returns a temporary password |
| `GET` / `PUT` / `DELETE` | `/api/users/{name}` | Read, change (roles, status, profile), or delete a user |
| `POST` | `/api/users/{name}/password` | Reset a user's password (temporary password, or a reset link by email) |
| `DELETE` | `/api/users/{name}/second-factor` | Reset a user's two-step verification |
| `GET` | `/api/users/{name}/sessions` | A user's sessions |
| `DELETE` | `/api/users/{name}/sessions/{id}` | End one of a user's sessions |
| `POST` | `/api/account/password` | Change the signed-in user's password |
| `POST` | `/api/account/recovery-codes` | Generate new recovery codes (recent sign-in) |
| `PUT` | `/api/account/profile` | Change the signed-in user's display name |
| `GET` / `DELETE` | `/api/account/sessions` | List the signed-in user's sessions, or end all but the current one |
| `DELETE` | `/api/account/sessions/{id}` | End one of the signed-in user's sessions |
| `POST` | `/api/reauth` | Confirm the password and a code before a sensitive action |
| `GET` | `/api/session` | Remaining session time (does not count as activity) |
| `POST` | `/api/session/activity` | Record user activity so the session stays open |
| `GET` | `/api/audit`, `/api/audit/export` | Audit log, and its CSV export (administrators) |

All user changes require an administrator and a recent sign-in.

## Reports

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/reports` | List scheduled reports |
| `POST` | `/api/reports` | Create a scheduled report |
| `PUT` / `DELETE` | `/api/reports/{id}` | Change or delete a report |
| `POST` | `/api/reports/{id}/send` | Send a report now |
| `GET` | `/api/reports/pdf` | Download a report as PDF, as it would be sent now |

Reports are for administrators. See [Reports](../admin-guide/reports.md).

## Disaster Recovery and Bare Metal

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/disaster/plans`, `/api/disaster/plans/{agent}` | Recovery readiness of every system, or one system's plan |
| `POST` | `/api/disaster/recover` | Start a full-system recovery (administrators, recent sign-in) |
| `GET` | `/api/disaster/recoveries`, `/api/disaster/recoveries/{id}` | Orchestrated recoveries and their status |
| `GET` | `/api/bmr/hosts` | Agents with their recovery-media status |
| `POST` | `/api/bmr/media/{agent}/build` | Build recovery media on an agent |
| `GET` / `DELETE` | `/api/bmr/media/{agent}/download`, `/api/bmr/media/{agent}` | Download or discard a host's cached recovery image |
| `POST` | `/api/bmr/prepare/{agent}` | Issue a single-use recovery token (administrators, recent sign-in) |
| `GET` / `DELETE` | `/api/bmr/tokens`, `/api/bmr/tokens/{id}` | List unused recovery tokens, or revoke one |

See [Disaster Recovery](../user-guide/disaster-recovery.md) and [Bare-metal restore](../user-guide/bare-metal-restore.md).

## Logs

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/logs` | Server and agent log entries |
| `GET` | `/api/logs/export` | Export log entries |

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
| `GET` / `POST` | `/login` | Sign-in page; submit user name and password (or, before any account exists, the dashboard password) |
| `GET` / `POST` | `/login/code` | Second step: a code from the authenticator app, or a recovery code |
| `GET` / `POST` | `/login/enroll` | Set up the authenticator at the first sign-in |
| `GET` / `POST` | `/login/forgot` | Request a password reset link by email |
| `GET` / `POST` | `/login/reset` | Choose a new password from an emailed link |
| `GET` / `POST` | `/setup` | Create the first administrator with the dashboard password |
| `POST` | `/logout` | End session |

The session is the `hitstor_session` cookie, issued only after both sign-in steps. There are no API tokens: scripts call the API with the cookie of a signed-in user, and the session ends after the inactivity timeout like any other (see [Sessions and lockout](../admin-guide/users.md#sessions)). A request that needs a recent sign-in answers `403` with `{"error": "reauth_required"}` and the header `X-HitStor-Reauth: 1`; confirm with `POST /api/reauth` and retry. State-changing requests sent by a browser from another origin are rejected.

## See Also

- [gRPC API](grpc-api.md) — agent backup protocol
- [Dashboard](../admin-guide/dashboard.md) — UI overview
- [Backup Jobs](../admin-guide/backup-jobs.md) — job types and workflows