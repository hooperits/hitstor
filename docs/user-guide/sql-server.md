# SQL Server

The `mssql` workload plugin backs up and restores Microsoft SQL Server
databases through the **Virtual Device Interface (VDI)**. SQL Server streams the
backup straight into HitStor's pipeline: nothing is staged on disk, and every
backup is chunked, deduplicated, compressed, and encrypted like any other
snapshot.

| Capability | Details |
|------------|---------|
| Backup kinds | Full (optionally `COPY_ONLY`), differential, transaction log |
| Restore | Latest point or a point in time; under the original or a new name |
| Platforms | Windows (VDI is Windows-only); SQL Server 2016 or later |
| Authentication | Windows integrated (default) or a SQL login |

## Requirements

- The HitStor agent runs **on the SQL Server host**. VDI does not work across
  the network.
- The plugin package `hitstor-plugin-mssql-<version>-windows-amd64.hpk`.
- An account that can back up and restore: `sysadmin`, or `db_backupoperator`
  on each database plus `dbcreator` for restores.
- For log backups and point-in-time restore, databases in the **FULL** (or
  BULK_LOGGED) recovery model. Databases in SIMPLE support full and
  differential backups only.

## Install the plugin

On the SQL Server host, in an elevated prompt:

```powershell
hitstor-agent plugin install hitstor-plugin-mssql-0.1.0-windows-amd64.hpk
hitstor-agent plugin browse mssql MSSQLSERVER
```

`browse` lists the databases on the instance. Once the agent's next schedule
poll reports the plugin, the server can create SQL Server jobs for it.

## Authentication

The agent service runs as `LocalSystem`, which SQL Server sees as the login
`NT AUTHORITY\SYSTEM`. Choose one of:

- **Integrated (default).** Grant `NT AUTHORITY\SYSTEM` the required roles, or
  run the agent service under a dedicated Windows account that has them.
- **SQL login.** Create a login for HitStor and set the plugin settings
  `auth=sql` and `username=<login>`. Put the password in the agent's
  environment file (`C:\ProgramData\HitStor\agent.env`):

  ```
  HITSTOR_MSSQL_PASSWORD=<password>
  ```

  The variable name can be changed with `password_env`. Passwords are never
  accepted in job settings.

Plugin settings:

| Setting | Default | Description |
|---------|---------|-------------|
| `instance` | `MSSQLSERVER` | Instance name (`MSSQLSERVER` is the default instance) |
| `server` | `localhost` | Host to connect to |
| `auth` | `integrated` | `integrated` or `sql` |
| `username` | | SQL login, with `auth=sql` |
| `password_env` | `HITSTOR_MSSQL_PASSWORD` | Environment variable that holds the password |
| `encrypt` | `true` | Connection encryption |
| `trust_server_certificate` | `true` | Trust the instance's certificate (the connection is local) |

## Back up

Objects are written `<instance>/<database>`, for example
`MSSQLSERVER/Sales`. A job backs up one or more databases with one backup
kind; schedule separate jobs per kind. A common plan:

| Job | Kind | Schedule |
|-----|------|----------|
| Weekly full | `full` | Sunday 01:00 |
| Daily differential | `differential` | Daily 01:00 |
| Log | `log` | Every 15 minutes |

Create the jobs through the dashboard API (the dashboard pages arrive in a
later release):

```bash
curl -X POST https://<server>:8420/api/backups -H 'Content-Type: application/json' -d '{
  "name": "Sales log", "agent_name": "sql01", "repo_name": "default",
  "job_type": "workload", "plugin": "mssql",
  "selection": ["MSSQLSERVER/Sales"], "backup_kind": "log",
  "schedule_enabled": true, "cron_expression": "*/15 * * * *"
}'
```

Each backup is stored in the snapshot as
`mssql/<instance>/<database>/<kind>-<time>.bak` (`.trn` for logs), next to a
`.meta.json` file with its LSNs, times, and database files.

Backups are taken **uncompressed** by default so consecutive fulls
deduplicate; HitStor compresses the chunks anyway.

!!! warning "Another backup tool on the same database"
    Full and log backups taken by HitStor become part of the database's backup
    chain. If another tool also backs up the database, its differentials and
    log backups will depend on HitStor's backups and the other way around. In
    that case let one tool own the chain and set `copy_only` on the other.

## Restore

The server collects the database's backups from the snapshots; the plugin
chooses the newest full backup finished by the target point, the newest
differential based on it, and the unbroken chain of log backups up to the
target, which it applies with `STOPAT`.

List the available backups:

```bash
curl 'https://<server>:8420/api/workload/points?agent=sql01&plugin=mssql&object=MSSQLSERVER/Sales'
```

Restore a copy as of a point in time, under a new name:

```bash
curl -X POST https://<server>:8420/api/workload/restores -H 'Content-Type: application/json' -d '{
  "agent_name": "sql01", "plugin": "mssql", "object": "MSSQLSERVER/Sales",
  "target": "2026-03-01T14:30:00Z",
  "options": {"database": "Sales_restored"}
}'
```

Omit `target` to restore the latest point. Set `source_agent` to restore
another server's backups onto `agent_name`.

| Option | Default | Description |
|--------|---------|-------------|
| `database` | Original name | Name to restore as |
| `replace` | `false` | Overwrite an existing database with that name |
| `recovery` | `true` | Bring the database online; `false` leaves it RESTORING for further manual restores |
| `relocate` | `true` when renaming | Move the files to the instance's default data and log folders, named after the new database |
| `data_path`, `log_path` | Instance defaults | Folders for relocated files |

A restore under the original name without `replace` fails if the database
exists, so an existing database is never overwritten by accident.

## Try it on the host

Without a server, run a backup into a folder and restore from it:

```powershell
hitstor-agent plugin test-backup mssql --select MSSQLSERVER/Sales --kind full `
    --option copy_only=true --out C:\Temp\sales-backup
hitstor-agent plugin test-restore mssql --from C:\Temp\sales-backup `
    --object MSSQLSERVER/Sales --option database=Sales_check
```

## Limitations

- [Retention](retention.md#workload-backup-chains) keeps each kept
  differential or log together with the backups back to the previous full.
  It treats every full as a chain start, so do not mix `copy_only` fulls into
  a job plan that also takes differentials.
- One database per VDI stream; databases in a job are backed up one after
  another.
- Dashboard pages for SQL Server jobs and restores are not available yet; use
  the API.

## See also

- [Workload Plugin Architecture](../dev-guide/plugin-architecture.md)
- [hitstor-agent plugin](../reference/agent-commands/plugin.md)
- [Dashboard HTTP API](../reference/dashboard-api.md)
