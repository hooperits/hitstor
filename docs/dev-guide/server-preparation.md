# Server Preparation

This page covers what to do on a fresh Ubuntu server **before** installing HitStor. It covers OS-level prerequisites only. For post-installation steps (init, serve, systemd), see [Server Setup](../admin-guide/server-setup.md).

!!! tip "Using the .run installer?"
    The [Linux Quick Install (.run)](../installation/linux-installer.md) handles user creation, directory setup, configuration, and service startup automatically via an interactive wizard; the repository is then created from the dashboard. You only need the **System Updates**, **Firewall**, and **Storage** sections from this page.

## System Updates

Start with a fully updated system:

```bash
sudo apt update && sudo apt upgrade -y
```

## Required Packages

Ubuntu ships with `systemd` by default. Install the additional packages HitStor depends on:

```bash
sudo apt install -y ca-certificates openssl
```

| Package | Purpose |
|---------|---------|
| `systemd` | Service management (pre-installed on Ubuntu) |
| `ca-certificates` | TLS certificate validation for agent connections |
| `openssl` | TLS certificate generation |

!!! note
    The `.deb` post-install script automatically creates the `hitstor` system user, group, and all required directories. No manual user creation is needed.

## Firewall Configuration

HitStor uses two TCP ports:

| Port | Protocol | Service |
|------|----------|---------|
| 8420 | TCP | gRPC — agent communication |
| 8421 | TCP | Web dashboard and Prometheus metrics |

### Install and Configure ufw

```bash
sudo apt install ufw
```

### Open Both Ports

!!! warning
    Always allow SSH (port 22) **before** running `ufw enable`, or you may lose remote access to the server.

```bash
sudo ufw allow 22/tcp comment "SSH"
sudo ufw allow 8420/tcp comment "HitStor gRPC"
sudo ufw allow 8421/tcp comment "HitStor Dashboard"
sudo ufw enable
```

Verify the rules:

```bash
sudo ufw status verbose
```

### Restrict Dashboard to LAN

If the dashboard should only be accessible from the local network:

```bash
sudo ufw allow from 10.0.0.0/24 to any port 8421 proto tcp comment "HitStor Dashboard (LAN)"
```

Replace `10.0.0.0/24` with your actual network range.

## Storage Considerations

Repository data lives at `/var/lib/hitstor/repo` by default.

- **Estimate storage**: raw data size divided by expected deduplication ratio. Typical dedup ratios range from 2x to 10x depending on how similar the backed-up data is across snapshots.
- **Dedicated volume**: mount a separate partition or volume at `/var/lib/hitstor` to isolate backup storage from the OS disk.
- **Filesystem**: ext4 or XFS. Both work well; XFS performs slightly better with large files.
- **Permissions**: the post-install script sets `/var/lib/hitstor` to mode `750`, owned by `hitstor:hitstor`.

```bash
# Example: mount a dedicated volume
sudo mkfs.xfs /dev/sdb1
sudo mkdir -p /var/lib/hitstor
echo '/dev/sdb1 /var/lib/hitstor xfs defaults 0 2' | sudo tee -a /etc/fstab
sudo mount /var/lib/hitstor
```

## TLS Certificates

When no certificate is configured, HitStor generates a self-signed certificate on startup and saves it in the repository directory (`server.crt`, `server.key`) so it survives restarts; before the server has a repository, the certificate is kept in memory only. This is fine for testing, but production deployments should use proper certificates.

### Self-Signed with OpenSSL

```bash
sudo mkdir -p /etc/hitstor/tls
sudo openssl req -x509 -newkey rsa:4096 -nodes \
    -keyout /etc/hitstor/tls/server.key \
    -out /etc/hitstor/tls/server.crt \
    -days 365 -subj "/CN=HOSTNAME"
sudo chmod 600 /etc/hitstor/tls/server.key
sudo chown hitstor:hitstor /etc/hitstor/tls/server.key /etc/hitstor/tls/server.crt
```

Replace `HOSTNAME` with the server's FQDN or IP address.

### Let's Encrypt

```bash
sudo apt install -y certbot
sudo certbot certonly --standalone -d backup.example.com
```

For detailed TLS configuration, see [TLS Certificates](../admin-guide/tls-certificates.md).

## What the Installers Do

### `.run` Installer

The `.run` installer with the interactive wizard handles everything:

1. Creates system group and user `hitstor`, and lets it read the systemd journal
2. Creates directories: the data directory (default `/var/lib/hitstor`), `/var/log/hitstor`, `/etc/hitstor`
3. Installs the binary and systemd service
4. Writes configuration files from wizard answers (mode `600`)
5. Enables and starts the service
6. Verifies the service is running and ports are listening

It does not create a repository: open the dashboard to create or connect one.

### `.deb` Package

When you install the `.deb` package with `dpkg -i`, the post-install script handles:

1. Creates system group `hitstor` (if it does not exist)
2. Creates system user `hitstor` with home `/var/lib/hitstor` and shell `/usr/sbin/nologin`
3. Adds the `hitstor` user to the `systemd-journal` group (for the dashboard **Logs** page)
4. Creates directories: `/var/lib/hitstor`, `/var/log/hitstor`, `/etc/hitstor`, `/usr/lib/hitstor/plugins`
5. Sets ownership of `/var/lib/hitstor` and `/var/log/hitstor` to `hitstor:hitstor` and mode `750`
6. Copies `server.env.example` to `server.env` and `config.yaml.example` to `config.yaml` (mode `600`) if they do not exist
7. Runs `systemctl daemon-reload` to pick up the new service file

After installing the package, review the configuration files and create a repository, from the dashboard or with `hitstor init` (see [Server Setup](../admin-guide/server-setup.md)).

## Next Steps

1. **Recommended**: Install with the interactive wizard: [Linux Quick Install (.run)](../installation/linux-installer.md)
2. Or install the package: [Linux Packages](../installation/linux-packages.md)
3. Initialize the repository and start the server: [Server Setup](../admin-guide/server-setup.md)
