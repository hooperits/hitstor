# Upgrading

This page covers how to upgrade an existing HitStor installation. Repositories are forward-compatible: newer versions of HitStor can read and write repositories created by older versions without any migration steps.

## Before You Upgrade

1. **Check your current version and available updates:**

    ```bash
    hitstor version
    hitstor version --check
    ```

    !!! tip
        The server automatically checks for updates every 24 hours and displays a banner in the web dashboard when a newer version is available.

2. **Back up your configuration files.** While upgrades preserve configuration, it is good practice to save a copy before proceeding:

    ```bash
    # Linux
    sudo cp -r /etc/hitstor /etc/hitstor.bak

    # Windows (PowerShell)
    Copy-Item -Recurse "$env:ProgramData\HitStor" "$env:ProgramData\HitStor.bak"
    ```

3. **Review the release notes** on the [GitHub Releases](https://github.com/hooperits/hitstor/releases) page for any breaking changes or required manual steps.

## Linux Quick Install (.run)

Run the new `.run` installer on an existing installation. The installer automatically detects the upgrade, displays the version comparison, and handles the entire process:

```bash
sudo ./hitstor-server-X.Y.Z-linux-amd64.run
```

The upgrade process:

1. Displays current and new version for confirmation
2. Backs up the current binary
3. Stops the service, installs the new binary and systemd units
4. **Never overwrites** your working configuration files (only updates example files)
5. Restarts the service and verifies it is healthy
6. On verification failure: **automatically rolls back** to the previous version

Use `--no-wizard` to skip the confirmation prompt for automated upgrades.

The same process works for the agent:

```bash
sudo ./hitstor-agent-X.Y.Z-linux-amd64.run
```

## Linux Packages

Download the new package from [GitHub Releases](https://github.com/hooperits/hitstor/releases) and install it over the existing one.

=== "Debian / Ubuntu"

    ```bash
    sudo dpkg -i hitstor_x.y.z_amd64.deb
    ```

=== "RHEL / Fedora"

    ```bash
    sudo rpm -U hitstor-x.y.z.x86_64.rpm
    ```

The same applies to the agent package:

=== "Debian / Ubuntu"

    ```bash
    sudo dpkg -i hitstor-agent_x.y.z_amd64.deb
    ```

=== "RHEL / Fedora"

    ```bash
    sudo rpm -U hitstor-agent-x.y.z.x86_64.rpm
    ```

Restart the services after upgrading:

```bash
sudo systemctl restart hitstor-server
sudo systemctl restart hitstor-agent
```

Verify the new version:

```bash
hitstor version
```

## Docker

Rebuild the image and restart the container:

```bash
# Pull latest source (if building from source)
git pull

# Rebuild the image
docker build -t hitstor:latest .

# Restart with the new image
docker compose -f docker-compose.prod.yml down
docker compose -f docker-compose.prod.yml up -d
```

Or, if using a specific version tag:

```bash
docker build -t hitstor:1.1.0 \
  --build-arg VERSION=1.1.0 \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg DATE=$(date -u +%Y-%m-%d) .
```

Update the `HITSTOR_VERSION` variable in your `.env` file, then:

```bash
docker compose -f docker-compose.prod.yml down
docker compose -f docker-compose.prod.yml up -d
```

Verify:

```bash
docker compose -f docker-compose.prod.yml exec hitstor hitstor version
```

Repository data in the `/var/lib/hitstor` volume is preserved across container restarts.

## Windows

Run the new installer over the existing installation. The NSIS installer overwrites the previous binary and updates the registry entries while preserving your configuration files in `%ProgramData%\HitStor\`.

1. Download the new installer from [GitHub Releases](https://github.com/hooperits/hitstor/releases).
2. Run the installer:

    ```
    hitstor-x.y.z-windows-amd64-setup.exe
    ```

3. Follow the installation wizard. Use the same installation directory as the previous version.
4. Open a new PowerShell window and verify:

    ```powershell
    hitstor version
    ```

For the agent, download and run the agent installer:

```
hitstor-agent-x.y.z-windows-amd64-setup.exe
```

If the agent is running as a Windows Service, it is automatically stopped during the upgrade and restarted afterward.

## Build from Source

Pull the latest changes and rebuild:

```bash
cd /path/to/hitstor
git pull
make build
make agent
```

Replace the installed binaries:

```bash
sudo cp bin/hitstor /usr/local/bin/
sudo cp bin/hitstor-agent /usr/local/bin/
```

Verify:

```bash
hitstor version
```

If running as a systemd service, restart it:

```bash
sudo systemctl restart hitstor-server
```

## Repository Compatibility

HitStor repositories are forward-compatible. A repository created with an older version can be read and written by any newer version without migration. There is no separate migration command or repository upgrade step.

If a future release ever requires a repository format change, it will be documented in the release notes with explicit migration instructions.

## Rollback

**Linux Quick Install (.run):** The `.run` installer automatically rolls back if post-upgrade verification fails. The previous binary is restored and the service is restarted with the old version — no manual intervention needed.

**Other methods:** If you need to revert to a previous version, install the older package or binary using the same installation method and restart the service.

Repositories are forward-compatible, so rolling back to an older version is safe as long as no forward-incompatible repository format changes were introduced (which would be documented in the release notes).
