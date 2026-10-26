# Security

This page documents the HitStor security model, covering encryption, key management, authentication, and system hardening.

## Encryption

### Data Encryption

All data stored in the repository is encrypted with **AES-256-GCM** (Galois/Counter Mode). Encryption is applied client-side before data is written to storage, ensuring that the repository contents are unreadable without the password.

Each blob (chunk of data) is encrypted with a unique nonce. The authenticated encryption provided by GCM ensures both confidentiality and integrity -- any tampering with encrypted data is detected on read.

### Key Derivation

The encryption key is derived from the repository password using **scrypt** with the following parameters:

| Parameter | Value | Purpose |
|-----------|-------|---------|
| N | 32768 | CPU/memory cost factor |
| r | 8 | Block size |
| p | 1 | Parallelization factor |

These parameters are designed to make brute-force attacks computationally expensive. The scrypt output produces two keys:

- **EncKey** -- used for AES-256-GCM encryption of data blobs
- **IDKey** -- used for HMAC-SHA256 blob identification

### Blob Identification

Blob IDs are computed as **HMAC-SHA256** using the IDKey derived from the password. This prevents content-guessing attacks: an attacker who knows the plaintext content of a file cannot determine whether it exists in the repository without knowing the password.

```
BlobID = HMAC-SHA256(IDKey, plaintext)
```

Without the IDKey, an attacker cannot correlate known plaintext with stored blobs.

## Transport Security

### TLS

All gRPC communication between agents and the server is encrypted with TLS. The server either auto-generates a self-signed certificate or uses custom certificates provided by the administrator.

| Configuration | Encryption | Certificate Verification |
|---------------|-----------|-------------------------|
| Auto-generated (default) | Yes | Agent must set `agent_ca_cert` |
| Custom certificate | Yes | Standard CA verification or `agent_ca_cert` |
| Let's Encrypt | Yes | Standard CA verification (no extra config) |

TLS is always enabled. There is no option to disable transport encryption.

See [TLS Certificates](tls-certificates.md) for detailed configuration.

## Authentication

### Agent Authentication

HitStor uses two authentication mechanisms for agents:

**Agent tokens** (dashboard-managed agents):

- 64-character hexadecimal string (256 bits of entropy)
- Generated automatically during agent installation
- Stored at `/etc/hitstor/agent-token` on the agent (mode 0600)
- Stored encrypted (AES-256-GCM) on the server in the agent registry
- Used by the server to authenticate gRPC calls to the agent (filesystem browsing, configuration)
- Retrievable any time with `hitstor-agent show-id`
- Verified using constant-time comparison to prevent timing attacks

**API keys** (all agents):

- 64-character hexadecimal string (256 bits of entropy)
- One key per agent
- Generated using a cryptographically secure random number generator
- For dashboard-managed agents: sent to the agent automatically during registration
- For CLI-registered agents: displayed once at registration time and cannot be retrieved again
- Stored as a hash in the encrypted agent registry

An agent must present a valid API key with every backup gRPC request. The server verifies the key against the hashed value in the registry.

### Repository Password

The repository password is the root secret for all encryption operations. It is:

- **Never stored on disk** by the server (provided at runtime via environment variable or password file)
- Used to derive EncKey and IDKey through scrypt
- Required to read or write any data in the repository

If the password is lost, the repository data is irrecoverable.

### Dashboard Authentication

The web dashboard always requires login. The password comes from `serve_dashboard_password`; when it is not set, the server generates a random password at startup, stores its scrypt hash in the config file and logs the plaintext once so the operator can sign in and change it.

Session cookies have the following security properties:

| Property | Value | Purpose |
|----------|-------|---------|
| `HttpOnly` | `true` | Prevents JavaScript access to the cookie |
| `SameSite` | `Strict` | Prevents cross-site request forgery (CSRF) |

The `/metrics` endpoint is excluded from authentication to allow Prometheus scraping. Restrict access to this endpoint at the network level if needed.

## Systemd Hardening

The HitStor server systemd unit includes security hardening directives that restrict the server process:

```ini
[Service]
# Filesystem protection
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/hitstor

# Privilege restrictions
NoNewPrivileges=true
PrivateTmp=true

# Capability restrictions
CapabilityBoundingSet=
AmbientCapabilities=

# Network
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX

# System call filtering
SystemCallFilter=@system-service
SystemCallArchitectures=native

# Namespace isolation
PrivateDevices=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
```

These directives:

- Prevent the server from writing outside its data directory
- Block privilege escalation
- Restrict system calls to the minimum required set
- Isolate the process from kernel tunables and device access

### Verifying Hardening

Check the security score of the unit:

```bash
systemd-analyze security hitstor-server
```

A score below 5.0 indicates good hardening. Review any items marked as "UNSAFE" and assess whether they are required for your deployment.

## Security Checklist

Use this checklist when deploying HitStor in production:

- [ ] Use a strong repository password (at least 16 characters, randomly generated)
- [ ] Store the password in a file with `0600` permissions, not on the command line
- [ ] Use custom TLS certificates from a trusted CA
- [ ] Set a dashboard password (`serve_dashboard_password`)
- [ ] Restrict dashboard port (8421) to management networks
- [ ] Restrict the `/metrics` endpoint to Prometheus scraper IPs
- [ ] Enable systemd hardening (included by default in packaged units)
- [ ] Ensure agent token files (`/etc/hitstor/agent-token`) have `0600` permissions
- [ ] Restrict agent gRPC port (8422) to the server's IP address
- [ ] Rotate API keys if compromise is suspected (remove and re-register the agent)
- [ ] Run `hitstor check --read-data` periodically to verify repository integrity
- [ ] Keep HitStor updated to receive security fixes

## Next Steps

- [TLS Certificates](tls-certificates.md) -- configure production TLS
- [Agent Management](agent-management.md) -- API key registration and rotation
- [Troubleshooting](troubleshooting.md) -- diagnose security-related issues
