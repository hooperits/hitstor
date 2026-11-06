# Linux Packages (deb/rpm)

Pre-built packages for Debian/Ubuntu (`.deb`) and RHEL/Fedora/AlmaLinux (`.rpm`) in amd64 architecture.

!!! tip "Prefer the .run installer for easier setup"
    The [Linux Quick Install (.run)](linux-installer.md) provides an interactive wizard that configures the dashboard password, ports, advertise address, and service startup for you. Packages require manual configuration (see below).

## Download

Download the latest packages from the [GitHub Releases](https://github.com/hooperits/hitstor/releases) page. Each release includes four Linux packages:

| Package | Description |
|---------|-------------|
| `hitstor_x.y.z_amd64.deb` | Server -- Debian/Ubuntu |
| `hitstor-x.y.z-1.x86_64.rpm` | Server -- RHEL/Fedora/AlmaLinux |
| `hitstor-agent_x.y.z_amd64.deb` | Agent -- Debian/Ubuntu |
| `hitstor-agent-x.y.z-1.x86_64.rpm` | Agent -- RHEL/Fedora/AlmaLinux |

## Install the Server

=== "Debian / Ubuntu"

    ```bash
    sudo dpkg -i hitstor_x.y.z_amd64.deb
    ```

=== "RHEL / Fedora"

    ```bash
    sudo rpm -i hitstor-x.y.z-1.x86_64.rpm
    ```

The package installs:

| Path | Description |
|------|-------------|
| `/usr/bin/hitstor` | Server binary |
| `/usr/lib/systemd/system/hitstor-server.service` | systemd service unit |
| `/etc/hitstor/server.env.example` | Environment file template |
| `/etc/hitstor/config.yaml.example` | Configuration file template |

## Install the Agent

=== "Debian / Ubuntu"

    ```bash
    sudo dpkg -i hitstor-agent_x.y.z_amd64.deb
    ```

=== "RHEL / Fedora"

    ```bash
    sudo rpm -i hitstor-agent-x.y.z-1.x86_64.rpm
    ```

The agent package installs:

| Path | Description |
|------|-------------|
| `/usr/bin/hitstor-agent` | Agent binary |
| `/usr/lib/systemd/system/hitstor-agent.service` | systemd service unit |
| `/usr/lib/systemd/system/hitstor-agent.timer` | systemd timer for scheduled backups |
| `/etc/hitstor/agent.env.example` | Environment file template |
| `/etc/hitstor/agent-config.yaml.example` | Configuration file template |
| `/usr/lib/hitstor/plugins/hitstor-scanner-builtin` | Built-in filesystem scanner plugin |
| `/var/lib/hitstor/plugins/` | Store for [workload plugins](../user-guide/plugins.md#workload-plugins) (mode 0700) |

## Configuration

The packages' post-install scripts copy the examples to the working files (`server.env`, `config.yaml`, `agent.env`, `agent-config.yaml`) when those do not exist yet, and never overwrite them on upgrade. The server package also creates the `hitstor` system user.

### Server

Edit `/etc/hitstor/config.yaml` to configure the server (ports, advertise address, TLS certificates, dashboard settings, etc.):

```bash
sudo editor /etc/hitstor/config.yaml
```

Repositories are normally created from the dashboard **Storage** page after the service starts. `/etc/hitstor/server.env` only needs `HITSTOR_REPO` and `HITSTOR_PASSWORD` for the legacy single-repository mode. The first sign-in works as described in [Dashboard Password and First Administrator](quickstart.md#dashboard-password).

### Agent

For a dashboard-managed agent (recommended), keep the copied files as they are: start the service (below), read the token with `sudo hitstor-agent show-id`, and add the agent from the dashboard (**Agents → Add Agent**) with its IP address, listen port (default 8422), and token.

Edit `/etc/hitstor/agent.env` only for a manually registered agent, to set `HITSTOR_AGENT_SERVER` and `HITSTOR_AGENT_API_KEY` (the key printed by `hitstor serve agent add <name>`):

```bash
sudo editor /etc/hitstor/agent.env
```

## Start the Services

### Server

Enable and start the HitStor server:

```bash
sudo systemctl enable --now hitstor-server
```

Check the service status:

```bash
sudo systemctl status hitstor-server
```

View logs:

```bash
journalctl -u hitstor-server -f
```

### Agent

Enable and start the HitStor agent:

```bash
sudo systemctl enable --now hitstor-agent
```

To enable the timer for scheduled backups instead of running the agent continuously:

```bash
sudo systemctl enable --now hitstor-agent.timer
```

## Verify the Installation

```bash
hitstor version
```

This prints the version, commit hash, and build date. If the command is not found, ensure `/usr/bin` is in your `PATH`.

## Uninstall

=== "Debian / Ubuntu"

    ```bash
    sudo systemctl disable --now hitstor-server
    sudo dpkg -r hitstor
    ```

=== "RHEL / Fedora"

    ```bash
    sudo systemctl disable --now hitstor-server
    sudo rpm -e hitstor
    ```

Configuration files in `/etc/hitstor/` are preserved during removal. Delete them manually if no longer needed.
