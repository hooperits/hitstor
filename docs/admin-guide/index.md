# Administration Guide

The HitStor Administration Guide is for system administrators responsible for deploying, configuring, and maintaining HitStor backup infrastructure. It covers server setup, agent management, TLS configuration, monitoring, and security hardening.

If you are looking for instructions on creating and restoring backups, see the [User Guide](../user-guide/index.md) instead.

## Topics

| Guide | Description |
|-------|-------------|
| [Server Setup](server-setup.md) | Initialize a repository, start the server, configure systemd, and manage firewall rules. |
| [TLS Certificates](tls-certificates.md) | Configure auto-generated or custom TLS certificates for gRPC and dashboard communication. |
| [Backup Jobs](backup-jobs.md) | Filesystem backup jobs, scheduling, and retention. |
| [Agent Management](agent-management.md) | Register, configure, and remove backup agents. |
| [Agent Discovery](agent-discovery.md) | Network scan for unregistered agents. |
| [Agent Deployment](agent-deployment.md) | Install and configure the `hitstor-agent` binary on Linux and Windows machines. |
| [Dashboard](dashboard.md) | Access the web dashboard, manage schedules, and use the JSON API. |
| [Server Settings](settings.md) | Central settings hub: TLS, network, integrations (Gemini), notifications. |
| [Monitoring](monitoring.md) | Prometheus metrics, webhook alerting, and Grafana integration. |
| [Scheduling](scheduling.md) | Configure server-managed schedules, daemon mode, and systemd timers. |
| [Sentinel](sentinel.md) | Ransomware telemetry and alerts. |
| [Security](security.md) | Encryption model, key derivation, API key management, and systemd hardening. |
| [Troubleshooting](troubleshooting.md) | Common errors, diagnostic commands, and log analysis. |
| [Roadmap](roadmap.md) | Current priorities and out-of-scope surfaces. |

## Prerequisites

Before following the guides in this section, make sure you have HitStor installed. See the [Installation Guide](../installation/index.md) for platform-specific instructions.

## Conventions

Throughout the Administration Guide the following conventions are used:

- Commands prefixed with `$` are run in a terminal.
- Text in `UPPER_CASE` (such as `AGENT_NAME`) represents a value you must replace with your own.
- Optional flags are shown in square brackets: `[--flag VALUE]`.
- Long commands are split across multiple lines with `\` for readability.
- Configuration file paths assume a standard Linux installation. Adjust paths for other platforms as needed.
