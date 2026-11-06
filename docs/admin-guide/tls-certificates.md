# TLS Certificates

All gRPC communication between the HitStor server and agents is encrypted with TLS, and the dashboard is served over HTTPS with the same certificate. This guide covers the default self-signed certificate behavior, custom certificate configuration, and certificate management for agents.

## Default Behavior: Self-Signed Certificates

When the server starts without explicit certificate configuration, it automatically generates a self-signed TLS certificate and private key. This provides encryption out of the box with no additional setup.

The auto-generated certificate is:

- Created at the first startup and saved as `server.crt` and `server.key` in the directory of the server's first repository (for example `/var/lib/hitstor/repo/server.crt`), then reused on every restart
- Valid for `localhost`, the server's hostname, `127.0.0.1` and the server's local IP addresses at the time it was generated
- An ECDSA P-256 certificate valid for 10 years
- Suitable for testing and internal networks

If the first repository is not on a local path (for example S3), the certificate is kept in memory and a new one is generated at each restart. To regenerate the saved certificate (for example after the server gets a new IP address), delete both files and restart the server; agents that pinned the old certificate must then forget it (see [below](#without-a-ca-certificate-certificate-pinning)).

## Custom Certificates

For production deployments, provide your own TLS certificate and private key.

### Using CLI Flags

```bash
hitstor serve \
  --repo /var/lib/hitstor/repo \
  --tls-cert /etc/hitstor/tls/server.crt \
  --tls-key /etc/hitstor/tls/server.key
```

### Using the Config File

```yaml
# /etc/hitstor/config.yaml
serve_tls_cert: /etc/hitstor/tls/server.crt
serve_tls_key: /etc/hitstor/tls/server.key
```

The certificate file should contain the full chain (server certificate followed by any intermediate certificates). The key file must be an unencrypted PEM-encoded private key.

You can also upload the certificate and key from **Settings → Security & TLS** in the dashboard; they are saved as `server.crt` and `server.key` in `/etc/hitstor/tls/` (Linux) or `%ProgramData%\HitStor\tls\` (Windows) and written to `serve_tls_cert` and `serve_tls_key`. Restart the server to use them.

### File Permissions

Restrict access to the private key, keeping it readable by the user the server runs as (`hitstor` with the packaged systemd unit):

```bash
sudo chown root:hitstor /etc/hitstor/tls/server.key
sudo chmod 0640 /etc/hitstor/tls/server.key
```

## Using Let's Encrypt

HitStor does not integrate directly with Let's Encrypt or ACME. Generate certificates externally using a tool like `certbot`, then point HitStor at the resulting files.

### Example with Certbot

```bash
sudo certbot certonly --standalone -d backup.example.com
```

This creates certificate files at:

- `/etc/letsencrypt/live/backup.example.com/fullchain.pem`
- `/etc/letsencrypt/live/backup.example.com/privkey.pem`

The private key under `/etc/letsencrypt` is readable only by root, and the packaged server runs as the `hitstor` user, so copy both files to `/etc/hitstor/tls/` (the deploy hook below does it on every renewal) and point HitStor at the copies:

```yaml
# /etc/hitstor/config.yaml
serve_tls_cert: /etc/hitstor/tls/server.crt
serve_tls_key: /etc/hitstor/tls/server.key
```

### Renewal

Let's Encrypt certificates are valid for 90 days. After certbot renews the certificate, restart the HitStor server to load the new files:

```bash
sudo systemctl restart hitstor-server
```

To automate this, add a post-renewal hook:

```bash
# /etc/letsencrypt/renewal-hooks/deploy/hitstor-restart.sh
#!/bin/bash
set -e
live=/etc/letsencrypt/live/backup.example.com
install -o root -g hitstor -m 0644 "$live/fullchain.pem" /etc/hitstor/tls/server.crt
install -o root -g hitstor -m 0640 "$live/privkey.pem" /etc/hitstor/tls/server.key
systemctl restart hitstor-server
```

```bash
sudo mkdir -p /etc/hitstor/tls
sudo chmod +x /etc/letsencrypt/renewal-hooks/deploy/hitstor-restart.sh
sudo /etc/letsencrypt/renewal-hooks/deploy/hitstor-restart.sh   # first copy
```

## Certificate Renewal

When replacing certificate files (from any CA, not just Let's Encrypt), restart the server to load the new certificate:

```bash
sudo systemctl restart hitstor-server
```

The server reads certificate files at startup. Changes to certificate files on disk are not detected at runtime.

## Agent CA Certificate

When the server uses a self-signed certificate or a certificate from a private CA, agents must be configured to trust that CA. Use the `agent_ca_cert` configuration key on each agent to point at the CA certificate file.

### Agent Configuration

In the agent config file:

```yaml
# /etc/hitstor/agent-config.yaml
agent_ca_cert: /etc/hitstor/tls/ca.crt
```

Or via environment variable:

```bash
# /etc/hitstor/agent.env
HITSTOR_AGENT_CA_CERT=/etc/hitstor/tls/ca.crt
```

The CA certificate file should be the PEM-encoded certificate of the authority that signed the server's certificate. For self-signed certificates, use the server certificate itself (`server.crt` from the repository directory).

### Without a CA Certificate (Certificate Pinning)

If `agent_ca_cert` is not set, the agent trusts the server certificate it sees on its first connection and pins its SHA-256 fingerprint in `/etc/hitstor/server-fingerprint` (`C:\ProgramData\HitStor\server-fingerprint` on Windows). Later connections are refused if the server presents a different certificate, unless that certificate is valid for the server hostname under the system's trusted CAs, in which case the pin is updated automatically.

Run the first connection (for example `hitstor-agent ping`) on a trusted network. If you replace the server's self-signed certificate on purpose, delete the fingerprint file on each agent so it pins the new one.

### Distributing the CA Certificate

Copy the CA certificate to each agent machine:

```bash
scp /etc/hitstor/tls/ca.crt agent-host:/etc/hitstor/tls/ca.crt
```

Verify the agent can connect:

```bash
hitstor-agent ping
```

## Troubleshooting TLS

| Symptom | Cause | Solution |
|---------|-------|----------|
| `certificate signed by unknown authority` | Agent does not trust the server CA | Set `agent_ca_cert` on the agent |
| `peer certificate fingerprint ... does not match pinned` | The server certificate changed since the agent pinned it | If the change was intended, delete the agent's `server-fingerprint` file; otherwise investigate a possible man-in-the-middle |
| `tls: bad certificate` | Certificate does not match hostname | Ensure the certificate's SAN includes the server hostname or IP |
| `tls: private key does not match public key` | Mismatched cert and key files | Regenerate or re-pair the certificate and key |
| `permission denied` reading key file | File permissions too restrictive | Ensure the HitStor process user can read the key file |

## Next Steps

- [Agent Management](agent-management.md) -- register agents that will connect over TLS
- [Security](security.md) -- full security model including encryption and key derivation
