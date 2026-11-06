# Backup Scheduling

HitStor supports several approaches to scheduling automatic backups. Choose the method that best fits your infrastructure.

## Server-Managed Scheduling

The recommended approach is server-managed scheduling, where backup schedules are configured centrally and agents poll the server for their next run time.

### Configure from the Dashboard

1. Open the [Dashboard](dashboard.md) at `https://SERVER_IP:8421`.
2. Navigate to **Backups** and click **+ New Backup Job**. Step 2 of the wizard sets the
   schedule with the schedule picker (see [Schedule Picker](#plugin-jobs-schedule-picker)); a plugin
   source gets a [backup plan](backup-jobs.md#backup-plans), one schedule per backup kind.
3. To change the schedule of an existing job, open it and expand **Edit schedule** (or **Edit plan**
   for a workload job). It uses the same picker; **Edit as cron expression** switches to the raw expression.
4. The dashboard validates the expression and shows it in words with its next runs.
5. Click **Save Schedule** (or **Save plan**).

The server stores each job's schedule with the job, in the agent registry. Agents running in daemon mode pick up changes automatically.

### Schedule Modes

#### Interval Mode (legacy)

Jobs created before cron schedules (or through the API with `schedule_hours`) run every N hours:

- **How it works**: `next_run = last_backup + interval_hours`; after a failed run, the next try is 15 minutes later
- **Timezone**: Not applicable (relative to last backup time)

The dashboard no longer creates interval jobs. Saving a new schedule on such a job replaces the interval with a cron expression; the picker starts from the nearest equivalent.

#### Schedule Picker { #plugin-jobs-schedule-picker }

Backup jobs, of files or of a plugin, set their
schedule with a picker modeled on the schedule sets of enterprise backup
tools, and store the result as a cron expression:

| Option | Settings |
|--------|----------|
| **On demand only** | The job runs only with **Backup Now** |
| **Repeating → Every day** | Start time |
| **Repeating → Days of week** | Weekdays, and optionally weeks of the month (1st to 5th, Last); start time |
| **Repeating → Days of month** | Days 1 to 31 and **Last day**; start time. Days a month does not have are skipped |
| **Repeating → Every…** | Every 5 to 30 minutes, or every 1 to 12 hours at a given minute; optionally only between two hours and only Monday to Friday |

Each schedule takes a time zone (the browser's by default for a new job;
**Server local** uses the server's). The picker shows the schedule in words
and its next runs, and **Edit as cron expression** switches to the raw
expression. A new filesystem job starts from every day at 2:00 AM. In a backup
plan each kind starts from its usual schedule: fulls on Sundays at 1:00 AM,
differentials Monday to Saturday at 1:00 AM (both on), and log backups every
15 minutes (off until you switch them on). In a plan, a kind is either
repeating or off (on demand only).

#### Cron Expression Mode

Standard 5-field cron syntax for precise, timezone-aware scheduling:

```
┌───────────── minute (0-59)
│ ┌───────────── hour (0-23)
│ │ ┌───────────── day of month (1-31)
│ │ │ ┌───────────── month (1-12)
│ │ │ │ ┌───────────── day of week (0-6, Sun=0)
│ │ │ │ │
* * * * *
```

**Supported syntax**:

| Syntax | Example | Meaning |
|--------|---------|---------|
| Wildcard | `*` | Every value |
| Specific value | `5` | Only at 5 |
| List | `1,15` | At 1 and 15 |
| Range | `1-5` | 1 through 5 |
| Step | `*/15` | Every 15 units |
| Range with step | `0-30/10` | 0, 10, 20, 30 |
| Last day of month | `L` (day of month) | The last day of each month |
| Nth weekday | `0#1` (day of week) | The first Sunday of the month (`#1` to `#5`) |
| Last weekday | `5L` (day of week) | The last Friday of the month |

Names (`MON`, `JAN`), `7` for Sunday, and macros such as `@daily` are not accepted. When both day of month and day of week are restricted, a day matching **either** runs, as in standard cron (`0 2 1 * 1` runs on the 1st and on every Monday).

**Common examples**:

| Expression | Description |
|------------|-------------|
| `0 2 * * *` | Daily at 2:00 AM |
| `0 3 * * 1-5` | Weekdays at 3:00 AM |
| `30 1 * * 0` | Sundays at 1:30 AM |
| `0 */6 * * *` | Every 6 hours |
| `0 22 1 * *` | 1st of each month at 10:00 PM |
| `*/15 * * * *` | Every 15 minutes |
| `0 23 L * *` | Last day of each month at 11:00 PM |
| `0 2 * * 0#1` | First Sunday of each month at 2:00 AM |

**Timezone handling**:

- Select a timezone from the dropdown (IANA format, e.g. `America/New_York`).
- If no timezone is selected (**Server local**), the server's local timezone is used. An unknown timezone falls back to UTC.
- Cron expressions are evaluated in the selected timezone, handling DST transitions automatically:
    - **Spring forward**: If a scheduled time falls in the skipped hour, that day's run is skipped; the next run is the next matching day.
    - **Fall back**: If a scheduled time falls in the repeated hour, the backup runs on the first occurrence.

### How It Works

1. The agent calls the `GetSchedule` RPC on the server.
2. The server evaluates the schedule (cron expression, backup plan or interval) and responds with the next run time (`next_run`). For a backup plan it also says which backup kind is due.
3. The agent sleeps until `next_run`, then executes the backup.
4. After the backup completes, the agent calls `GetSchedule` again for the next run.

This approach allows administrators to change schedules from the dashboard without restarting or reconfiguring agents.

### Missed Window Recovery

If the server or agent is offline when a backup was due:

- On the next poll, the server detects that `now > next_run` and signals the agent to run immediately.
- After the catch-up backup, the schedule advances to the next future occurrence (no double-runs).
- This works for interval, cron and backup-plan schedules.

A failed run is not retried on every poll: a cron or plan schedule waits for its next slot, and an interval job retries after 15 minutes.

## Agent Daemon Mode

Daemon mode is the client-side component of server-managed scheduling. Start the agent in daemon mode:

```bash
hitstor-agent daemon
```

In daemon mode, the agent:

- Runs as a long-lived process
- Polls the server for its backup schedule via the `GetSchedule` RPC (default: every 5 minutes; `--poll-interval`, minimum 1 minute)
- Executes backups when `now >= next_run_unix` for any enabled job
- Reports backup results back to the server
- Handles transient connection failures with automatic retry

### Systemd Service (Linux)

Enable the daemon as a systemd service:

```bash
sudo systemctl enable hitstor-agent
sudo systemctl start hitstor-agent
```

The service unit runs `hitstor-agent daemon --config /etc/hitstor/agent-config.yaml` with restart-on-failure.

### Windows Service

Register the daemon as a Windows Service:

```powershell
hitstor-agent service install
hitstor-agent service start
```

The Windows installer already does this. The service starts automatically on boot. See [Agent Deployment](agent-deployment.md) for full Windows setup instructions.

## Systemd Timer (Alternative)

If you prefer cron-style scheduling managed at the OS level rather than server-managed scheduling, use a systemd timer that runs a one-shot `hitstor-agent backup`.

!!! note
    Versions before 0.2.1 installed a `hitstor-agent.timer` that only started the daemon again; current packages remove it. Use your own units, as below, for one-shot backups.

### Create the Units

The agent must already be able to reach the server: `/etc/hitstor/agent.env` holds `HITSTOR_AGENT_SERVER` and `HITSTOR_AGENT_API_KEY` once the agent is registered.

```ini
# /etc/systemd/system/hitstor-backup.service
[Unit]
Description=HitStor one-shot backup
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
EnvironmentFile=/etc/hitstor/agent.env
ExecStart=/usr/bin/hitstor-agent backup /etc /home
```

```ini
# /etc/systemd/system/hitstor-backup.timer
[Unit]
Description=Run a HitStor backup daily at 2 AM

[Timer]
OnCalendar=*-*-* 02:00:00
Persistent=true

[Install]
WantedBy=timers.target
```

### Enable the Timer

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now hitstor-backup.timer
```

### Customize the Schedule

Change `OnCalendar` in the timer, or override it:

```bash
sudo systemctl edit hitstor-backup.timer
```

```ini
[Timer]
OnCalendar=
OnCalendar=*-*-* 03:00:00
```

### OnCalendar Syntax

The `OnCalendar` directive uses systemd calendar event syntax. Common examples:

| Expression | Schedule |
|------------|----------|
| `*-*-* 02:00:00` | Daily at 2:00 AM |
| `*-*-* 00/6:00:00` | Every 6 hours (midnight, 6 AM, noon, 6 PM) |
| `*-*-* *:00:00` | Every hour |
| `Mon *-*-* 03:00:00` | Every Monday at 3:00 AM |
| `*-*-01 04:00:00` | First day of each month at 4:00 AM |
| `*-*-* 02:00:00,14:00:00` | Twice daily at 2:00 AM and 2:00 PM |

Verify the schedule with `systemd-analyze`:

```bash
systemd-analyze calendar "*-*-* 02:00:00"
```

### Timer vs. Daemon Mode

| Feature | Daemon Mode | Timer Mode |
|---------|-------------|------------|
| Schedule source | Server (dashboard) | Local systemd timer |
| Central management | Yes | No (per-machine) |
| Dynamic schedule changes | Yes (via dashboard) | Requires systemd edit |
| Process lifecycle | Long-running | Starts per backup |
| Workload (plugin) jobs and dashboard restores | Yes | No |
| Server connection | Persistent | On-demand |

Daemon mode is recommended for most deployments because it enables centralized schedule management.

## Windows Task Scheduler

On Windows systems where the agent is not registered as a Windows Service, use Task Scheduler as an alternative:

1. Open Task Scheduler (`taskschd.msc`).
2. Create a new basic task.
3. Set the trigger to the desired schedule (for example, daily at 2:00 AM).
4. Set the action to start a program:
    - Program: `hitstor-agent.exe` in the agent's installation folder
    - Arguments: `backup C:\Users C:\Data`
5. Save the task.

The command line does not read `agent.env` (only the daemon and the service do), so give the task the server and key: add `--server HOST:8420 --api-key KEY` to the arguments, or set `agent_server` and `agent_api_key` in a config file passed with `--config`.

For server-managed scheduling on Windows, use daemon mode via the Windows Service instead.

## Scheduling Best Practices

- **Use cron expressions** for production deployments. They provide precise, timezone-aware scheduling that handles DST transitions and server restarts correctly.
- **Stagger backup times** across agents to avoid overloading the server with simultaneous backups. With cron expressions, use different minutes (e.g., `0 2 * * *` for one agent, `15 2 * * *` for another).
- **Use off-peak hours** (for example, 01:00 to 05:00) to minimize impact on production workloads.
- **Always set a timezone** when using cron expressions to avoid ambiguity across servers in different locations.
- **Set the stale threshold** (`alert_stale_threshold`) to slightly more than the backup interval. For daily backups, 25 hours is a reasonable default.
- **Monitor scheduling** via the dashboard and Prometheus metrics to ensure backups run on time.

## Next Steps

- [Agent Deployment](agent-deployment.md) -- install agents and configure systemd units
- [Monitoring](monitoring.md) -- set up alerts for missed backups
- [Dashboard](dashboard.md) -- manage schedules from the web interface
