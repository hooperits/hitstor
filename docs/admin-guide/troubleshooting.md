# Troubleshooting

This page covers common HitStor issues, their causes, and solutions. For each problem, the error message is shown as it appears in logs or terminal output.

## Repository Errors

### "repository not initialized"

**Cause:** The repository directory does not exist or has not been initialized with `hitstor init`.

**Solution:** Initialize the repository before starting the server or running backups:

```bash
hitstor init --repo /var/lib/hitstor/repo --password "REPO_PASSWORD"
```

If the directory exists but appears empty or corrupted, check that the path is correct and that the `keys/`, `data/`, `index/`, and `snapshots/` subdirectories are present.

### "incorrect password"

**Cause:** The password provided does not match the password used when the repository was initialized.

**Solution:**

1. Check the `HITSTOR_PASSWORD` environment variable:

    ```bash
    echo $HITSTOR_PASSWORD
    ```

2. If using a password file (`--password-file`), verify the file contains the correct password with no trailing newline:

    ```bash
    wc -c /etc/hitstor/password.txt
    cat -A /etc/hitstor/password.txt
    ```

3. If using systemd, check the environment file:

    ```bash
    sudo cat /etc/hitstor/server.env
    ```

4. Ensure there are no extra spaces, quotes, or escape characters in the password value.

### "lock repository: repository is locked"

**Cause:** Another HitStor process is currently using the repository, or a previous process terminated without releasing the lock.

**Solution:**

1. Check for other running HitStor processes:

    ```bash
    ps aux | grep hitstor
    ```

2. If no other process is running, the lock is stale. Remove it:

    ```bash
    hitstor unlock --repo /var/lib/hitstor/repo --password "REPO_PASSWORD"
    ```

3. If using the server, ensure only one `hitstor serve` process runs against a given repository.

## TLS Errors

### "TLS connection error"

**Cause:** The agent cannot verify the server's TLS certificate. The most common causes are:

1. The agent does not have the server certificate (missing `--ca-cert`)
2. The certificate does not include the server IP in its SANs
3. The server binary is an old version that generates certificates for `127.0.0.1` only

**Diagnosis:** Check which IPs the certificate includes:

```bash
openssl x509 -in /var/lib/hitstor/repo/server.crt -noout -text | grep -A1 "Subject Alternative Name"
```

If it only lists `127.0.0.1` and `localhost`, the certificate does not contain the server's real IP.

**Solution:**

1. Make sure the server binary is up to date (`hitstor version`)
2. Delete the old certificate to force regeneration:

    ```bash
    rm -f /var/lib/hitstor/repo/server.crt /var/lib/hitstor/repo/server.key
    ```

3. Restart the server — it generates a new certificate with all local IPs:

    ```bash
    hitstor serve --repo /var/lib/hitstor/repo ...
    ```

4. Verify that the new certificate includes the correct IP:

    ```bash
    openssl x509 -in /var/lib/hitstor/repo/server.crt -noout -text | grep -A1 "Subject Alternative Name"
    # Expected: IP Address:192.168.x.x in the list
    ```

5. Copy the certificate to the agent and configure it:

    ```bash
    scp hits@SERVER_IP:/var/lib/hitstor/repo/server.crt /etc/hitstor/server.crt
    hitstor-agent --ca-cert /etc/hitstor/server.crt ping
    ```

    O via config:

    ```yaml
    agent_ca_cert: /etc/hitstor/server.crt
    ```

### "certificate signed by unknown authority"

**Cause:** The agent does not trust the TLS certificate presented by the server. This is common with self-signed certificates or private CAs.

**Solution:** Configure the agent to trust the server's certificate:

```bash
hitstor-agent --ca-cert /etc/hitstor/server.crt ping
```

Or permanently via config file:

```yaml
# ~/.config/hitstor/config.yaml
agent_ca_cert: /etc/hitstor/server.crt
```

For self-signed certificates (the default), copy the server's `server.crt` from the repo directory. See [TLS Certificates](tls-certificates.md) for details.

### "tls: bad certificate"

**Cause:** The server's TLS certificate does not match the hostname or IP address the agent is connecting to.

**Solution:** Verify the certificate's SANs include the server's IP:

```bash
openssl x509 -in /var/lib/hitstor/repo/server.crt -noout -text | grep -A1 "Subject Alternative Name"
```

If the IP is missing, delete the cert and restart the server to regenerate it with all current IPs.

### "tls: private key does not match public key"

**Cause:** The certificate file and key file do not correspond to each other.

**Solution:** Verify the files match:

```bash
openssl x509 -noout -modulus -in server.crt | md5sum
openssl rsa -noout -modulus -in server.key | md5sum
```

If the checksums differ, the files are mismatched. Regenerate or re-pair them.

## Connection Errors

### "connection refused"

**Cause:** The server is not running, or a firewall is blocking the port.

**Solution:**

1. Verify the server is running:

    ```bash
    sudo systemctl status hitstor-server
    ```

2. Check the server is listening on the expected port:

    ```bash
    ss -tlnp | grep 8420
    ```

3. Verify firewall rules allow traffic on ports 8420 and 8421:

    ```bash
    sudo firewall-cmd --list-ports
    # or
    sudo ufw status
    ```

4. If connecting across networks, ensure there are no intermediate firewalls or security groups blocking the port.

### Agent Not Appearing in Dashboard

**Cause:** The agent has not successfully connected to the server or completed a backup.

**Solution:**

1. Check the agent can reach the server:

    ```bash
    hitstor-agent ping
    ```

2. Verify the API key is correct in `/etc/hitstor/agent.env`.

3. Check agent logs for errors:

    ```bash
    sudo journalctl -u hitstor-agent -n 50
    ```

4. Ensure the agent name matches what was registered with `hitstor serve agent add`.

## Alert Issues

### Stale Agent Alerts

**Cause:** An agent has not completed a backup within the configured stale threshold (default: 25 hours).

**Solution:**

1. Check the agent service is running:

    ```bash
    sudo systemctl status hitstor-agent
    ```

2. Review agent logs for errors:

    ```bash
    sudo journalctl -u hitstor-agent --since "24 hours ago"
    ```

3. Verify network connectivity between the agent and server.

4. If the agent is intentionally offline (maintenance, decommissioned), remove it:

    ```bash
    hitstor serve agent remove AGENT_NAME
    ```

5. Adjust the stale threshold if the backup schedule is less frequent than every 25 hours:

    ```yaml
    # /etc/hitstor/config.yaml
    alert_stale_threshold: 49  # hours
    ```

## Maintenance Lock Errors

### "repository is locked for maintenance"

**Cause:** A check or prune operation is currently running on the repository. Only one maintenance operation can run at a time per repository.

**Solution:**

1. Wait for the current operation to finish. Check the Storage page in the dashboard for operation progress.

2. If the operation appears stuck, restart the server. The in-memory maintenance lock is released when the process exits.

3. If you need to run a maintenance operation urgently, use the "Force" option from the dashboard, which aborts active backup sessions and acquires the lock.

### "acquire lock: ..."

**Cause:** A race condition where another maintenance request acquired the lock between the initial check and the lock attempt.

**Solution:** Retry the operation after the current maintenance task completes.

## Performance Issues

### Slow Backups

**Cause:** Network bandwidth limitations, disk I/O bottlenecks, or suboptimal parallelism settings.

**Solution:**

1. Check the `--parallel` setting. The default is suitable for most systems, but adjust based on available CPU cores and disk speed:

    ```bash
    hitstor-agent backup --parallel 4 /data
    ```

2. Monitor network throughput during backup:

    ```bash
    # On the server
    iftop -i eth0 -f "port 8420"
    ```

3. Check disk I/O on both the agent and server:

    ```bash
    iostat -x 1
    ```

4. For large initial backups, consider running the first backup on the local network rather than over a WAN link.

5. Review the backup output for deduplication statistics. Low deduplication ratios on subsequent backups may indicate data churn.

## Repository Integrity

### Verifying Repository Health

Run a structural check:

```bash
hitstor check --repo /var/lib/hitstor/repo --password "REPO_PASSWORD"
```

For a thorough check that reads and decrypts every blob:

```bash
hitstor check --repo /var/lib/hitstor/repo --password "REPO_PASSWORD" --read-data
```

The `--read-data` flag significantly increases check duration but verifies that all data can be decrypted and matches its expected checksums.

### Check Output

A healthy repository produces:

```
check: no errors found
```

If errors are found, the output lists affected pack files or snapshots. Contact support or consult the repository recovery documentation.

## Log Analysis

HitStor uses Go's `slog` package for structured JSON logging. Log entries include fields for timestamp, level, message, and contextual data.

### Viewing Logs

```bash
# Follow server logs
sudo journalctl -u hitstor-server -f

# Follow agent logs
sudo journalctl -u hitstor-agent -f
```

### Filtering with jq

Parse and filter structured JSON logs:

```bash
# Show only errors
sudo journalctl -u hitstor-server -o cat | jq 'select(.level == "ERROR")'

# Show backup-related messages
sudo journalctl -u hitstor-server -o cat | jq 'select(.msg | contains("backup"))'

# Show messages for a specific agent
sudo journalctl -u hitstor-server -o cat | jq 'select(.agent == "web-server-01")'

# Count errors in the last 24 hours
sudo journalctl -u hitstor-server --since "24 hours ago" -o cat | jq 'select(.level == "ERROR")' | wc -l
```

### Common Log Fields

| Field | Description |
|-------|-------------|
| `time` | ISO 8601 timestamp |
| `level` | Log level (`DEBUG`, `INFO`, `WARN`, `ERROR`) |
| `msg` | Human-readable message |
| `agent` | Agent name (for agent-related events) |
| `snapshot` | Snapshot ID (for snapshot-related events) |
| `duration` | Operation duration (for timed operations) |
| `bytes` | Data size (for transfer operations) |
| `error` | Error message (for error-level entries) |

## Getting Help

If the troubleshooting steps above do not resolve the issue:

1. Collect server and agent logs covering the time of the problem.
2. Note the HitStor version (`hitstor version`).
3. Note the operating system and version.
4. Open an issue on the HitStor repository with the collected information.
