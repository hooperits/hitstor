# Server Setup

This guide covers deploying the HitStor server, including repository initialization, server startup, systemd integration, and firewall configuration.

## Create a Repository

Repositories can be created in two ways:

1. **From the dashboard** (recommended): Navigate to **Storage** and click **New Repository**. The dashboard guides you through creating a local or S3-compatible repository.
2. **From the CLI**:

```bash
hitstor init --repo /var/lib/hitstor/repo --password "REPO_PASSWORD"
```

For production systems, use a password file or environment variable instead of passing the password on the command line:

```bash
export HITSTOR_PASSWORD="REPO_PASSWORD"
hitstor init --repo /var/lib/hitstor/repo
```

## Start the Server

Start the HitStor server with gRPC and dashboard ports:

```bash
hitstor serve --repo /var/lib/hitstor/repo --port 8420 --dashboard-port 8421
```

The server opens two listeners:

| Port | Protocol | Purpose |
|------|----------|---------|
| 8420 | gRPC (TLS) | Agent communication -- backups, restores, scheduling |
| 8421 | HTTP | Web dashboard and JSON API |

The server runs in the foreground. Use systemd (see below) for production deployments.

## Repository Password

The server needs the repository password to decrypt and encrypt data. There are three ways to provide it:

1. **Environment variable** (recommended for production):

    ```bash
    export HITSTOR_PASSWORD="REPO_PASSWORD"
    hitstor serve --repo /var/lib/hitstor/repo
    ```

2. **Password file**:

    ```bash
    hitstor serve --repo /var/lib/hitstor/repo --password-file /etc/hitstor/password.txt
    ```

    The file should contain the password on a single line with no trailing newline. Set permissions to `0600`.

3. **Command-line flag** (not recommended for production):

    ```bash
    hitstor serve --repo /var/lib/hitstor/repo --password "REPO_PASSWORD"
    ```

## Configuration File

The server reads configuration from `/etc/hitstor/config.yaml` when the `--config` flag is set. A typical server configuration:

```yaml
# /etc/hitstor/config.yaml
repo: /var/lib/hitstor/repo

# TLS certificates (optional, auto-generated if omitted)
serve_tls_cert: /etc/hitstor/tls/server.crt
serve_tls_key: /etc/hitstor/tls/server.key

# Dashboard
serve_dashboard_password: "DASHBOARD_PASSWORD"

# Alerting
alert_webhook_url: "https://hooks.example.com/hitstor"
alert_stale_threshold: 25
```

Start the server with the config file:

```bash
hitstor serve --config /etc/hitstor/config.yaml
```

See [Configuration](../user-guide/configuration.md) for the full schema and precedence rules.

## Multi-Repository Configuration

HitStor supports managing multiple repositories from a single server instance. This is useful for separating backups by team, environment, or retention policy.

### Defining Repositories

Add a `repositories` section to the config file:

```yaml
# /etc/hitstor/config.yaml
repositories:
  - name: production
    type: local
    path: /var/lib/hitstor/prod-repo
    password_file: /etc/hitstor/prod-password

  - name: staging
    type: local
    path: /var/lib/hitstor/staging-repo
    password_file: /etc/hitstor/staging-password

  - name: offsite
    type: s3
    endpoint: s3.amazonaws.com
    bucket: company-backups
    region: us-east-1
    access_key_encrypted: <encrypted by server>
    secret_key_encrypted: <encrypted by server>
    password_file: /etc/hitstor/s3-repo-password
```

S3 access keys are stored encrypted (`access_key_encrypted`, `secret_key_encrypted`). Use the dashboard Storage page to create S3 repositories without editing ciphertext manually.

Each repository must have a unique `name`. The `type` field must be `local` or `s3`.

### Backward Compatibility

If the `repositories` list is empty and a `repo` field is present, the server operates in single-repo legacy mode using the `repo` path and `HITSTOR_PASSWORD` environment variable.

### Assigning Agents to Repositories

Each agent targets a specific repository. Set the target repository from the Agent Detail page in the dashboard, or the agent will use the first configured repository by default.

### Managing Repositories from the Dashboard

Repositories can also be created and removed from the dashboard Storage page. See [Dashboard](dashboard.md) for details.

## TLS Certificates

All gRPC communication between server and agents is encrypted with TLS.

### Auto-Generated (Default)

If no `--tls-cert` / `--tls-key` flags are provided, the server automatically generates a self-signed certificate on first startup and saves it to:

- `<repo>/server.crt` — certificate (distribute to agents)
- `<repo>/server.key` — private key (keep on server only)

The certificate includes all of the server's IP addresses and hostname in its Subject Alternative Names (SANs), so agents can connect by IP or hostname. The certificate is reused across server restarts.

To distribute the certificate to agents:

```bash
scp /var/lib/hitstor/repo/server.crt agent-host:/etc/hitstor/server.crt
```

Then configure each agent to trust it:

```yaml
# Agent config.yaml
agent_ca_cert: /etc/hitstor/server.crt
```

Or via CLI flag:

```bash
hitstor-agent --ca-cert /etc/hitstor/server.crt ping
```

### Custom Certificates (Production)

For production, provide your own certificates:

```bash
hitstor serve --repo /var/lib/hitstor/repo \
  --tls-cert /etc/hitstor/tls/server.crt \
  --tls-key /etc/hitstor/tls/server.key
```

Or via config file:

```yaml
serve_tls_cert: /etc/hitstor/tls/server.crt
serve_tls_key: /etc/hitstor/tls/server.key
```

If you use a certificate signed by a public CA, agents do not need the `--ca-cert` flag.

## Systemd Service

On Linux systems installed via deb or rpm packages, a systemd unit is provided.

### Enable and Start

```bash
sudo systemctl enable hitstor-server
sudo systemctl start hitstor-server
```

### Environment File

The systemd unit reads environment variables from `/etc/hitstor/server.env`. Configure the repository password and any other settings here:

```bash
# /etc/hitstor/server.env
# Repository passwords are managed via the dashboard.
# For legacy single-repo mode:
# HITSTOR_PASSWORD=REPO_PASSWORD
# HITSTOR_REPO=/var/lib/hitstor/repo
```

Set permissions so only root can read the file:

```bash
sudo chmod 0600 /etc/hitstor/server.env
sudo chown root:root /etc/hitstor/server.env
```

### Check Status

```bash
sudo systemctl status hitstor-server
sudo journalctl -u hitstor-server -f
```

## Firewall Rules

Open the gRPC and dashboard ports on the server. For `firewalld`:

```bash
sudo firewall-cmd --permanent --add-port=8420/tcp
sudo firewall-cmd --permanent --add-port=8421/tcp
sudo firewall-cmd --reload
```

For `ufw` (Ubuntu/Debian):

```bash
sudo apt install ufw
sudo ufw allow 22/tcp comment "SSH"
sudo ufw allow 8420/tcp comment "HitStor gRPC"
sudo ufw allow 8421/tcp comment "HitStor Dashboard"
sudo ufw enable
```

!!! warning
    Always allow SSH (port 22) **before** running `ufw enable`, or you may lose remote access to the server.

Verify the rules:

```bash
sudo ufw status verbose
```

If the dashboard should only be accessible from a management network, restrict port 8421 accordingly:

```bash
sudo ufw allow from 10.0.0.0/24 to any port 8421 proto tcp
```

## Log Output

HitStor uses Go's `slog` package for structured logging. Log output is JSON by default when running as a systemd service, making it easy to parse with tools like `jq`:

```bash
sudo journalctl -u hitstor-server -o cat | jq .
```

Increase verbosity with the `--verbose` flag:

| Flag | Level | Description |
|------|-------|-------------|
| (none) | warn | Warnings and errors only |
| `-v` | info | Operational messages (backup started, agent connected) |
| `-vv` | debug | Detailed internal state (chunk processing, pack writes) |

## Next Steps

- [TLS Certificates](tls-certificates.md) -- configure custom certificates for production
- [Agent Management](agent-management.md) -- register agents to connect to this server
- [Dashboard](dashboard.md) -- access the web dashboard
