# Linux Packages (deb/rpm)

Pre-built packages for Debian/Ubuntu (`.deb`) and RHEL/Fedora/AlmaLinux (`.rpm`) in amd64 architecture.

!!! tip "Prefer the .run installer for easier setup"
    The [Linux Quick Install (.run)](linux-installer.md) provides an interactive wizard that configures everything automatically — passwords, ports, repository initialization, and service startup. Packages require manual configuration (see below).

## Download

Download the latest packages from the [GitHub Releases](https://github.com/hooperits/hitstor/releases) page. Each release includes four Linux packages:

| Package | Description |
|---------|-------------|
| `hitstor_x.y.z_amd64.deb` | Server -- Debian/Ubuntu |
| `hitstor-x.y.z.x86_64.rpm` | Server -- RHEL/Fedora/AlmaLinux |
| `hitstor-agent_x.y.z_amd64.deb` | Agent -- Debian/Ubuntu |
| `hitstor-agent-x.y.z.x86_64.rpm` | Agent -- RHEL/Fedora/AlmaLinux |

## Install the Server

=== "Debian / Ubuntu"

    ```bash
    sudo dpkg -i hitstor_x.y.z_amd64.deb
    ```

=== "RHEL / Fedora"

    ```bash
    sudo rpm -i hitstor-x.y.z.x86_64.rpm
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
    sudo rpm -i hitstor-agent-x.y.z.x86_64.rpm
    ```

The agent package installs:

| Path | Description |
|------|-------------|
| `/usr/bin/hitstor-agent` | Agent binary |
| `/usr/lib/systemd/system/hitstor-agent.service` | systemd service unit |
| `/usr/lib/systemd/system/hitstor-agent.timer` | systemd timer for scheduled backups |
| `/etc/hitstor/agent.env.example` | Environment file template |
| `/etc/hitstor/agent-config.yaml.example` | Configuration file template |

## Configuration

After installation, create your configuration files from the provided examples.

### Server

```bash
sudo cp /etc/hitstor/server.env.example /etc/hitstor/server.env
sudo cp /etc/hitstor/config.yaml.example /etc/hitstor/config.yaml
```

Edit `/etc/hitstor/server.env` to set the repository path and password:

```bash
sudo editor /etc/hitstor/server.env
```

Edit `/etc/hitstor/config.yaml` to configure the server (listen address, TLS certificates, dashboard settings, etc.):

```bash
sudo editor /etc/hitstor/config.yaml
```

### Agent

```bash
sudo cp /etc/hitstor/agent.env.example /etc/hitstor/agent.env
sudo cp /etc/hitstor/agent-config.yaml.example /etc/hitstor/agent-config.yaml
```

Edit `/etc/hitstor/agent.env` to set the server address and API key:

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
