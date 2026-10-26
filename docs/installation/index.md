# Installation

HitStor provides two binaries:

- **`hitstor`** -- the backup server, CLI tool, and web dashboard.
- **`hitstor-agent`** -- a lightweight remote agent that sends encrypted, deduplicated backups to a HitStor server over gRPC.

Choose the installation method that best fits your environment.

**New to HitStor?** Start with the [Quick Start](quickstart.md) guide — Docker server + agent binaries in ~15 minutes.

## Installation Methods

| Method | Platforms | Best For |
|--------|-----------|----------|
| [Quick Start](quickstart.md) | Docker + Linux/Windows agents | Fastest path: Docker server, VM installer, or agent-only |
| [Linux Quick Install (.run)](linux-installer.md) | Ubuntu, Debian, RHEL, Fedora, AlmaLinux | Interactive wizard — configures, initializes, and starts everything. Built-in upgrade with rollback and uninstaller. |
| [Linux Packages (deb/rpm)](linux-packages.md) | Debian, Ubuntu, RHEL, Fedora, AlmaLinux | Production servers with package manager |
| [Build from Source](linux-source.md) | Any OS with Go 1.26+ | Development, custom builds, unsupported architectures |
| [Windows Server](windows-server.md) | Windows 10+, Windows Server 2016+ | Running the HitStor server on Windows |
| [Windows Agent](windows-agent.md) | Windows 10+, Windows Server 2016+ | Backing up Windows machines to a remote HitStor server |
| [Docker](docker.md) | Any Docker host | Containerized deployment, CI/CD pipelines |

## After Installation

Once HitStor is installed, continue with:

1. [Getting Started](../user-guide/getting-started.md) -- end-to-end tutorial covering init, backup, restore, verify, and prune.
2. [Configuration](../user-guide/configuration.md) -- config file schema, environment variables, and flag precedence.
3. [Command Reference](../reference/commands/index.md) -- detailed documentation for every command.

## Upgrading

See [Upgrading](upgrading.md) for instructions on upgrading an existing HitStor installation.
