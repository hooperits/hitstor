# Changelog

All notable changes to HitStor are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/). Before 1.0, minor versions may
include breaking changes; they are listed under **Upgrade notes**.

## [0.2.0] — 2026-11-06

### Upgrade notes

- **The shared dashboard password is replaced by user accounts.** After
  upgrading, the dashboard opens the *Create the first administrator* page.
  Confirm the current dashboard password, choose an administrator name and
  password, and set up two-step verification with an authenticator app. Then
  invite the rest of your team from **Users and access**. See
  [Users and access](https://hooperits.github.io/hitstor/admin-guide/users/).
- With several repositories configured, the server keeps agents and users in
  the first repository by name. `hitstor serve agent …` and
  `hitstor serve user …` now use that repository by default.

### Added

- **Workload plugins**: signed, installable `.hpk` packages (ed25519) managed
  per agent with `hitstor-agent plugin` or from the dashboard
  (**Settings → Plugins**). Plugins back up and restore application objects
  through the agent, and retention keeps their backup chains whole.
- **SQL Server plugin** (`mssql`, Windows): native full, differential and log
  backups streamed through VDI, restore to the latest point or to a point in
  time, with SQL or Windows authentication kept on the agent per instance.
- **Backup plans** for plugin jobs: one selection with its own schedule for
  each backup kind (full, differential, log), results per database, partial
  runs that keep what finished, and a visible backup chain per job. A
  differential or log backup without a HitStor full backup fails with a clear
  message and offers to run the full backup now.
- **User accounts and access control**: roles (administrator, backup operator,
  restore operator, viewer) scoped to all or some agents, mandatory two-step
  verification (TOTP and recovery codes), session timeouts with a warning, a
  *My account* page, and a tamper-evident **audit log**. Users can also be
  managed from the console with `hitstor serve user`.
- **Email**: invitations and password reset links, sent through an SMTP relay
  (none, STARTTLS or TLS; anonymous or with a password) or Microsoft 365
  (OAuth 2.0 client credentials).
- **PDF reports**: a *protection summary* (week or month) and a *daily backup
  status*, in English or Spanish, downloadable from **Reports** or emailed on a
  cron schedule.
- **New backup job wizard**: choose what to protect from one tree of machines,
  file systems and plugin objects; a schedule picker with presets and cron,
  including `L`, `DL` and `D#N` (last day, last weekday, n-th weekday).
- **New brand**: padlock-and-layers icon, horizontal logo, and new artwork in
  both Windows installers, which now share a welcome page and header.

### Fixed

- Restore verification failed after every backup.
- The agent generated a new TLS certificate at every start, so the server's
  pinned certificate stopped matching until the agent was registered again.
- The Windows agent service did not load workload plugins, did not report
  failed jobs to the server, and its configuration files could be read by
  non-administrators.
- `hitstor repo create … --password` ignored the password and failed.
- `hitstor prune --max-repack-size` misread size suffixes such as `500MiB` at
  random.
- The Docker health check reported the container as unhealthy once the
  dashboard switched to HTTPS.
- `hitstor serve agent …` and `hitstor serve user …` failed with
  "Password required" when several repositories were configured.
- Without Sentinel credentials, every backup logged an error; the server now
  warns once.
- The server could stop while post-backup checks were still writing.
- Backup chains made only of full backups failed to show in the dashboard.
- The sign-in page claimed client-side encryption for agent backups; agents
  send compressed chunks over TLS and the server encrypts them.
- The Linux installer and configuration examples described an empty dashboard
  password as "open access".

## [0.1.0] — 2026-10-27

First public release: encrypted, deduplicated backup engine; server and agents
over gRPC/TLS; embedded web dashboard in English and Spanish; Windows service
and installers; Linux packages and installers; Docker image.

[0.2.0]: https://github.com/hooperits/hitstor/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/hooperits/hitstor/releases/tag/v0.1.0
