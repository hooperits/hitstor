# Backup Scheduling

HitStor supports several approaches to scheduling automatic backups. Choose the method that best fits your infrastructure.

## Server-Managed Scheduling

The recommended approach is server-managed scheduling, where backup schedules are configured centrally and agents poll the server for their next run time.

### Configure from the Dashboard

1. Open the [Dashboard](dashboard.md) at `http://SERVER_IP:8421`.
2. Navigate to **Backups** and click **+ New Backup Job** (or open an existing job).
3. Choose a schedule mode:
    - **Interval**: Run every N hours (simple, no timezone awareness).
    - **Cron Expression**: Use standard 5-field cron syntax for precise scheduling with timezone support.
4. If using Cron mode, enter the expression and select a timezone. The dashboard validates the expression and shows a human-readable description with the next 3 run times.
5. Save the configuration.

The server stores schedule configuration in the agent registry. Agents running in daemon mode pick up changes automatically.

### Schedule Modes

#### Interval Mode

The simplest scheduling option. Set the number of hours between backups:

- **How it works**: `next_run = last_backup + interval_hours`
- **Timezone**: Not applicable (relative to last backup time)
- **Best for**: Simple setups where exact run times don't matter

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

**Common examples**:

| Expression | Description |
|------------|-------------|
| `0 2 * * *` | Daily at 2:00 AM |
| `0 3 * * 1-5` | Weekdays at 3:00 AM |
| `30 1 * * 0` | Sundays at 1:30 AM |
| `0 */6 * * *` | Every 6 hours |
| `0 22 1 * *` | 1st of each month at 10:00 PM |
| `*/15 * * * *` | Every 15 minutes |

**Timezone handling**:

- Select a timezone from the dropdown (IANA format, e.g. `America/New_York`).
- If no timezone is selected, the server's local timezone is used.
- Cron expressions are evaluated in the selected timezone, handling DST transitions automatically:
    - **Spring forward**: If a scheduled time falls in the skipped hour, the backup runs at the next valid time.
    - **Fall back**: If a scheduled time falls in the repeated hour, the backup runs on the first occurrence.

### How It Works

1. The agent calls the `GetSchedule` RPC on the server.
2. The server evaluates the schedule (cron expression or interval) and responds with the next run time (`next_run`).
3. The agent sleeps until `next_run`, then executes the backup.
4. After the backup completes, the agent calls `GetSchedule` again for the next run.

This approach allows administrators to change schedules from the dashboard without restarting or reconfiguring agents.

### Missed Window Recovery

If the server or agent is offline when a backup was due:

- On the next poll, the server detects that `now > next_run` and signals the agent to run immediately.
- After the catch-up backup, the schedule advances to the next future occurrence (no double-runs).
- This works for both interval and cron modes.

## Agent Daemon Mode

Daemon mode is the client-side component of server-managed scheduling. Start the agent in daemon mode:

```bash
hitstor-agent daemon
```

In daemon mode, the agent:

- Runs as a long-lived process
- Polls the server for its backup schedule via the `GetSchedule` RPC (default: every 5 minutes)
- Executes backups when `now >= next_run_unix` for any enabled job
- Reports backup results back to the server
- Handles transient connection failures with automatic retry

### Systemd Service (Linux)

Enable the daemon as a systemd service:

```bash
sudo systemctl enable hitstor-agent
sudo systemctl start hitstor-agent
```

The service unit runs `hitstor-agent daemon` with restart-on-failure.

### Windows Service

Register the daemon as a Windows Service:

```powershell
hitstor-agent service install
hitstor-agent service start
```

The service starts automatically on boot. See [Agent Deployment](agent-deployment.md) for full Windows setup instructions.

## Systemd Timer (Alternative)

If you prefer cron-style scheduling managed at the OS level rather than server-managed scheduling, use a systemd timer.

### Enable the Timer

```bash
sudo systemctl enable hitstor-agent.timer
sudo systemctl start hitstor-agent.timer
```

The timer triggers the `hitstor-agent` service unit, which runs a single backup and exits.

### Customize the Schedule

Override the default timer schedule:

```bash
sudo systemctl edit hitstor-agent.timer
```

Add an `[Timer]` section with your desired `OnCalendar` value:

```ini
[Timer]
OnCalendar=*-*-* 02:00:00
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
| Server connection | Persistent | On-demand |

Daemon mode is recommended for most deployments because it enables centralized schedule management.

## Windows Task Scheduler

On Windows systems where the agent is not registered as a Windows Service, use Task Scheduler as an alternative:

1. Open Task Scheduler (`taskschd.msc`).
2. Create a new basic task.
3. Set the trigger to the desired schedule (for example, daily at 2:00 AM).
4. Set the action to start a program:
    - Program: `C:\Program Files\HitStor\hitstor-agent.exe`
    - Arguments: `backup C:\Users C:\Data`
5. Save the task.

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
