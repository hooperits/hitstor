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
| Authentication | Per instance: the agent's account (default), a Windows account, or a SQL login |

## Requirements

- The HitStor agent runs **on the SQL Server host**. VDI does not work across
  the network.
- The plugin package `hitstor-plugin-mssql-<version>-windows-amd64.hpk`.
- A login in the **`sysadmin`** role. SQL Server requires it for any `BACKUP`
  or `RESTORE` to a virtual device (error 3265 otherwise); narrower roles such
  as `db_backupoperator` are not enough for VDI.
- For log backups and point-in-time restore, databases in the **FULL** (or
  BULK_LOGGED) recovery model. Databases in SIMPLE support full and
  differential backups only.

## Install the plugin

From the dashboard: upload the package under **Settings → Plugins**, then open
the agent's page and choose **Install** in its **Plugins** section. The agent
verifies the package signature before installing it.

Or on the SQL Server host, in an elevated prompt:

```powershell
hitstor-agent plugin install hitstor-plugin-mssql-0.1.0-windows-amd64.hpk
hitstor-agent plugin browse mssql MSSQLSERVER
```

`browse` lists the databases on the instance. Once the agent's next schedule
poll reports the plugin, the server can create SQL Server jobs for it.

## Authentication

Like other enterprise backup tools, HitStor keeps the credentials of each SQL
Server instance **on the agent**, not in jobs: a job only names what to back
up. The settings are tested before they are saved, stored encrypted (DPAPI)
in the agent's plugin store, and the password is never shown again or sent to
the server.

In the dashboard, open the selection tree (in **New Backup Job**, expand the
machine and **Microsoft SQL Server**, or in a plugin restore) and choose
**Configure** next to the instance:

| Setting | Description |
|---------|-------------|
| **Authentication** | `integrated`: the agent service account (`LocalSystem`, which SQL Server sees as `NT AUTHORITY\SYSTEM`). `windows`: a Windows account. `sql`: a SQL Server login |
| **User name** | The SQL login, or `DOMAIN\user` for a Windows account (`.\user` for a local one) |
| **Password** | Saved encrypted on the agent; leave it empty to keep the saved one |

The account must be in the **`sysadmin`** role (see [Requirements](#requirements)).
**Test and save** connects to the instance and lists its databases; the
settings are saved only if that works. **Use the agent account** removes
them. An instance without saved settings uses the agent's account, so either
add `NT AUTHORITY\SYSTEM` to `sysadmin` or configure the instance.

On the host, the same with the agent CLI (the password is read from standard
input):

```powershell
hitstor-agent plugin configure mssql MSSQLSERVER --set auth=sql --set username=hitstor_backup --secret password
```

Other plugin settings, for the command line (`--setting`) and jobs created
before instance settings existed:

| Setting | Default | Description |
|---------|---------|-------------|
| `instance` | `MSSQLSERVER` | Instance listed when none is found in the registry |
| `server` | `localhost` | Host to connect to |
| `auth`, `username` | `integrated` | Credentials for instances without saved settings |
| `password_env` | `HITSTOR_MSSQL_PASSWORD` | Environment variable (in `agent.env`) with the password for those |
| `encrypt` | `true` | Connection encryption |
| `trust_server_certificate` | `true` | Trust the instance's certificate (the connection is local) |

## Back up

The object tree lists the SQL Server instances installed on the host (from the
registry), with their release, edition, and version. Expanding an instance
lists its databases with their recovery model and size; system databases come
last, and databases that are not online are shown but cannot be selected.

Objects are written `<instance>/<database>`, for example
`MSSQLSERVER/Sales`, or just `<instance>` to back up **the whole instance**:
every online database at each run, including databases created after the job.
Differential backups of a whole instance skip `master`, and log backups skip
databases in the SIMPLE recovery model.

### Backup plan

A job backs up one selection with a **backup plan**: each backup kind has its
own schedule, and differential and log backups build on the job's own fulls.
The wizard proposes a common plan:

| Kind | Default schedule | Notes |
|------|------------------|-------|
| Full | Sunday 01:00 | Starts the chain |
| Differential | Monday to Saturday 01:00 | Changes since the job's last full |
| Transaction log | Off; e.g. every 15 minutes | Needs the FULL recovery model |

Each kind can be switched off (on demand only). When two kinds fall at the
same time, the full runs and covers the differential. **Backup Now** asks which
kind to run, and each kind's row on the job page has its own **Run now**.

In the dashboard, choose **New Backup Job**, expand the machine and
**Microsoft SQL Server** in the selection tree, and check the instance or
individual databases. The next step sets the repository and the plan. See
[Backup jobs](../admin-guide/dashboard.md#backups).

The same job through the dashboard API:

```bash
curl -X POST https://<server>:8420/api/backups -H 'Content-Type: application/json' -d '{
  "name": "Sales", "agent_name": "sql01", "repo_name": "default",
  "job_type": "workload", "plugin": "mssql", "selection": ["MSSQLSERVER/Sales"],
  "plan": [
    {"kind": "full", "enabled": true, "cron_expression": "0 1 * * 0", "timezone": "America/Panama"},
    {"kind": "differential", "enabled": true, "cron_expression": "0 1 * * 1,2,3,4,5,6", "timezone": "America/Panama"},
    {"kind": "log", "enabled": true, "cron_expression": "*/15 * * * *", "timezone": "America/Panama"}
  ]
}'
```

`POST /api/backups/<id>/run` with `{"kind": "full"}` runs one kind now.

### A full backup is required

Before a differential or log backup, the plugin checks in `msdb` that the
backup will restore from HitStor. If it would not, the run fails **before
backing anything up** and the job page offers **Run full backup now**:

| Reason | Meaning |
|--------|---------|
| `no-full` | No full backup since the database was created or switched to FULL |
| `external-full` | The full it depends on was taken by another tool (the page shows when, by whom and where) |
| `external-log` | Another tool took the last log backup, leaving a gap in HitStor's chain |
| `unknown-base` | `msdb` has no record of the full it depends on (for example, a restored copy) |

When a selection holds several databases, all are checked first, and the
message lists each one that needs a full. SQL Server itself would accept many
of these backups (a differential on another tool's full, for instance), but
HitStor could not restore them from its own backups.

### Backup chain

The job page shows each database's **backup chain**: every full, the
differentials based on it (their base LSN is the full's checkpoint LSN) and the
log backups after it, with how far the chain restores. A gap in the log chain is
marked, and backups that build on a full HitStor does not hold are listed apart.
The restore page shows the same link in its **Based on** column.

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

Open the snapshot of a SQL Server backup (**Restores**, or the job's
snapshots) and choose **Restore with the plugin**, or open **Restores →
Restore from a Plugin Backup**. The restore works like SQL Server Management
Studio's:

1. **Restore tree.** The agent, its instances, and the databases backed up,
   each with its latest backup. Choose the database.
2. **Backup sets.** Every backup of that database. Restore to the selected
   backup, the latest point, or a point in time.
3. **Destination.** The agent and the instance to restore on (by default the
   original), and **Restore as**: a new name keeps the existing database
   untouched.
4. **Files.** Each database file with its original path and where it will be
   restored. A new name or another instance moves the files to the instance's
   default data and log folders, named after the new database; type a path to
   put a file elsewhere.
5. **Summary.** Before anything runs, the plugin checks the restore on the
   target instance: the backups it will apply and warnings, such as a database
   with that name that already exists (turn on **Replace** to overwrite it) or
   a file another database already uses.

The server collects the database's backups from the snapshots; the plugin
chooses the newest full backup finished by the target point, the newest
differential based on it, and the unbroken chain of log backups up to the
target, which it applies with `STOPAT`. **Restore as files** on the snapshot
page restores the `.bak` files themselves instead.

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
| `instance` | Original instance | Instance to restore on |
| `database` | Original name | Name to restore as |
| `replace` | `false` | Overwrite an existing database with that name |
| `recovery` | `true` | Bring the database online; `false` leaves it RESTORING for further manual restores |
| `relocate` | `true` when renaming or changing instance | Move the files to the instance's default data and log folders, named after the new database |
| `data_path`, `log_path` | Instance defaults | Folders for relocated files |
| `file:<logical name>` | | Full path for one file, e.g. `file:Sales_log` = `L:\Logs\Sales_copy_log.ldf` |

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

## See also

- [Workload Plugin Architecture](../dev-guide/plugin-architecture.md)
- [hitstor-agent plugin](../reference/agent-commands/plugin.md)
- [Dashboard HTTP API](../reference/dashboard-api.md)
