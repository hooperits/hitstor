# Retention

As you take regular backups, the number of snapshots grows steadily. The `hitstor forget` command removes snapshots that no longer match a retention policy, and `hitstor prune` reclaims the disk space occupied by data that is no longer referenced by any remaining snapshot.

## Removing Individual Snapshots

You can remove specific snapshots by passing their IDs directly:

```console
hitstor forget --repo /path/to/repo --password "secret" abc12345 def67890
removed snapshot abc12345
removed snapshot def67890
```

This is useful for one-off cleanup, but for routine maintenance a policy-based approach is more practical.

## Policy-Based Retention

Instead of specifying individual IDs, define rules that describe **which snapshots to keep**. Everything that does not match any rule is removed.

### Available Policy Flags

| Flag | Keeps |
|------|-------|
| `--keep-last N` | The N most recent snapshots |
| `--keep-hourly N` | The most recent snapshot for each of the last N hours that have a snapshot |
| `--keep-daily N` | The most recent snapshot for each of the last N days that have a snapshot |
| `--keep-weekly N` | The most recent snapshot for each of the last N weeks that have a snapshot |
| `--keep-monthly N` | The most recent snapshot for each of the last N months that have a snapshot |
| `--keep-yearly N` | The most recent snapshot for each of the last N years that have a snapshot |
| `--keep-tag TAG` | Every snapshot that carries the specified tag |

### Examples

Keep the 7 most recent snapshots and one daily snapshot for the last 30 days:

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --keep-last 7 \
    --keep-daily 30
```

Keep one snapshot per month for one year, plus all snapshots tagged `important`:

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --keep-monthly 12 \
    --keep-tag important
```

## Union Semantics

Retention policies use **union semantics**: a snapshot is kept if it matches **any** of the active rules. Rules are not applied sequentially or exclusively; they are evaluated independently and their results are combined.

For example, with `--keep-last 3 --keep-daily 7`:

- A snapshot is kept if it is one of the 3 most recent, **or** if it is the latest snapshot for one of the last 7 calendar days.
- A snapshot that satisfies both rules is still kept (the rules do not cancel each other out).

## Workload Backup Chains

Workload snapshots (for example from [SQL Server](sql-server.md)) carry
`plugin:<id>` and `kind:<kind>` tags and one `obj:<object>` tag per object
(database) they hold. The policy is applied to the snapshots as a whole and
also to each object's own backups, so every database keeps its last N
backups, its dailies, and so on, whatever the other databases did.

Differential and log backups are only restorable together with the backups
they build on. When a policy keeps one, HitStor also keeps, for the same host,
plugin, and object, the backups it needs, even if the policy alone would
forget them: a differential keeps its base full backup; a log backup keeps the
earlier log backups back to its base full. Copy-only full backups are never
used as a base.

## Host and Path Filtering

By default, `forget` applies the retention policy to **all** snapshots in the repository as one set: snapshots are not grouped by host or path, so `--keep-last 7` keeps the 7 most recent snapshots overall. To apply a policy to one host or path, narrow the scope with filters:

### Filter by Host

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --host webserver \
    --keep-daily 14
```

Only snapshots recorded from `webserver` are evaluated. Snapshots from other hosts are untouched.

### Filter by Path

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --path /var/www \
    --keep-last 5
```

Only snapshots whose backup path is `/var/www` are evaluated.

Both flags can be combined to target a specific host-and-path combination.

## Dry Run

Before committing to a retention policy, preview its effect with `--dry-run`:

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --keep-last 7 \
    --keep-daily 30 \
    --dry-run
```

The command prints each snapshot with `keep` or `forget`, without actually deleting anything. Always run `--dry-run` first when tuning a new policy.

Example dry-run output:

```
keep   abc12345 2026-02-22 08:00:11
keep   bcd23456 2026-02-21 08:00:09
...
keep   cde34567 2026-01-24 08:00:12
forget def45678 2026-01-23 08:00:10
forget ef567890 2026-01-22 08:00:07
snapshots: 30 kept, 2 forgotten
```

## Automatic Prune

You can combine `forget` and `prune` into a single step by adding the `--prune` flag:

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --keep-last 7 \
    --keep-daily 30 \
    --prune
```

This removes the snapshots that fall outside the retention policy and immediately reclaims the disk space for any unreferenced data. It is equivalent to running `forget` followed by `prune` as separate commands.

## Separate Prune Step

If you prefer to decouple snapshot removal from space reclamation, run `prune` as a standalone command:

```console
hitstor prune --repo /path/to/repo --password "secret"
```

`prune` scans the repository for data chunks that are no longer referenced by any snapshot and deletes them. It also repacks partially-used pack files to reduce fragmentation.

### Limiting Repack Size

For very large repositories, repacking can involve significant I/O. Use `--max-repack-size` to cap the amount of data repacked in a single run:

```console
hitstor prune --repo /path/to/repo --password "secret" \
    --max-repack-size 5GiB
```

This processes up to 5 GiB of pack data per invocation. Run `prune` again later to continue where it left off. This approach is useful for spreading the I/O cost over multiple maintenance windows.

## Example Retention Strategies

### Development Workstation

Keep recent history for quick rollbacks, but do not accumulate months of snapshots:

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --keep-last 3 \
    --keep-daily 7 \
    --keep-weekly 4 \
    --prune
```

- Always have the last 3 snapshots available.
- One snapshot per day for the past week.
- One snapshot per week for the past month.

### Production Server

Maintain a longer history with monthly granularity for auditing:

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --host production-db \
    --keep-last 7 \
    --keep-daily 30 \
    --keep-weekly 12 \
    --keep-monthly 24 \
    --prune
```

- Last 7 snapshots for immediate recovery.
- Daily snapshots for the past month.
- Weekly snapshots for the past 3 months.
- Monthly snapshots for the past 2 years.

### Compliance / Long-Term Archival

Regulations may require retaining data for several years:

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --keep-daily 90 \
    --keep-monthly 36 \
    --keep-yearly 7 \
    --keep-tag compliance \
    --prune
```

- Daily snapshots for the past 90 days.
- Monthly snapshots for the past 3 years.
- Yearly snapshots for the past 7 years.
- Any snapshot tagged `compliance` is kept indefinitely.

!!! tip
    Schedule `forget --prune` as a cron job or systemd timer so that retention policies are applied automatically. See the [Admin Guide](../admin-guide/index.md) for automation recipes.
