# User Guide

The HitStor User Guide covers everything you need to know to protect your data with encrypted, deduplicated backups. Whether you are backing up a single laptop or managing retention policies across multiple servers, this guide walks you through every step.

## Topics

| Guide | Description |
|-------|-------------|
| [Getting Started](getting-started.md) | End-to-end tutorial that takes you from initializing a repository to restoring files. |
| [Backup](backup.md) | Create backups, exclude files, use tags, tune parallelism, and understand deduplication. |
| [Restore](restore.md) | Restore full snapshots or individual files, resolve snapshot IDs, and verify restored data. |
| [Snapshots](snapshots.md) | List, filter, compare, and inspect snapshots stored in your repository. |
| [Retention](retention.md) | Define retention policies, remove old snapshots with `forget`, and reclaim space with `prune`. |
| [Configuration](configuration.md) | Manage the config file, environment variables, and interactive setup wizard. |
| [Plugins](plugins.md) | Discover, install, and configure storage backends, notifiers, hooks, and scanner plugins. |

## Prerequisites

Before following the guides in this section, make sure you have HitStor installed. See the [Installation Guide](../installation/index.md) for platform-specific instructions.

## Conventions

Throughout the User Guide the following conventions are used:

- Commands prefixed with `$` are run in a terminal.
- Text in `UPPER_CASE` (such as `SNAPSHOT_ID`) represents a value you must replace with your own.
- Optional flags are shown in square brackets: `[--flag VALUE]`.
- Long commands are split across multiple lines with `\` for readability.
