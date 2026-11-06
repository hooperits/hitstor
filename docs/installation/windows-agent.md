# Windows Agent Installation

The HitStor agent is a lightweight binary that sends encrypted, deduplicated backups from a Windows machine to a remote HitStor server over gRPC. The NSIS installer registers the agent as a Windows Service (selected by default) that is then managed from the HitStor dashboard.

## Requirements

- Windows 10 (1809+) or Windows Server 2016+
- Architecture: amd64 (the installer is amd64 only; an arm64 binary can be built from source with `make agent-windows-arm64`)
- Network access to the HitStor server (default port 8420), and from the server to the agent's listen port (default 8422)
- Administrator privileges (required for installation and service management)

## Download

Download the agent installer from the [GitHub Releases](https://github.com/hooperits/hitstor/releases) page:

- `hitstor-agent-x.y.z-windows-amd64-setup.exe`

## Install

1. Run the installer:

    ```
    hitstor-agent-x.y.z-windows-amd64-setup.exe
    ```

2. Accept the license agreement.
3. Choose the installation directory (default: `C:\Program Files\HitStor Agent\`).
4. On the **Components** page, select the components to install:
    - **HitStor Agent** (required) -- installs the agent binary and configuration.
    - **Install as Windows Service** (selected by default) -- registers and starts the agent as a Windows Service, and opens its listen port in Windows Firewall.
5. On the **Agent Configuration** page, keep the listen port (default `8422`) unless another service uses it. The server connects to this port to register and manage the agent.
6. Click **Install**. The last page shows the listen port, the **agent token**, and the next steps.

The installer performs the following steps:

- Copies `hitstor-agent.exe` to the installation directory and adds the directory to the system `PATH`.
- Creates `%ProgramData%\HitStor\`, restricted to SYSTEM and Administrators, with `agent-config.yaml.example`, `agent.env` (`HITSTOR_AGENT_LISTEN_PORT`), and an `agent-config.yaml` without backup paths (an existing one is preserved). On upgrade, the credentials in `agent.env` are kept.
- Creates a Start Menu shortcut for the uninstaller.
- If the service component is selected, registers the `HitStorAgent` Windows Service with `--config %ProgramData%\HitStor\agent-config.yaml`, adds the firewall rule `HitStor Agent` for the listen port, and starts the service. The service generates the agent token (`%ProgramData%\HitStor\agent-token`) on first start.

## Register the Agent

In the HitStor dashboard, go to **Agents → Add Agent** and enter this machine's IP address, the listen port, and the agent token shown at the end of the installation (`hitstor-agent show-id` prints it again from an elevated prompt). The server sends the agent its address and API key, which the agent stores in `agent.env`. Then create backup jobs under **Backups**; backup paths and schedules are managed from the dashboard.

## Manual Configuration

A dashboard-managed agent needs no configuration file edits. To run the agent CLI against a server without dashboard registration (for example with an API key from `hitstor serve agent add <name>`), use a configuration file. The service reads `%ProgramData%\HitStor\agent-config.yaml` (passed with `--config`) and `%ProgramData%\HitStor\agent.env`; CLI commands read `%AppData%\hitstor\config.yaml` unless you pass `--config`.

Edit the system-wide file (`agent-config.yaml.example` in the same folder lists every option):

```powershell
notepad "$env:ProgramData\HitStor\agent-config.yaml"
```

### Required Settings

At minimum, configure the server address and API key:

```yaml
agent_server: "backup-server.internal:8420"
agent_api_key: "your-64-char-hex-api-key"
```

### Full Example

```yaml
agent_server: "backup-server.internal:8420"
agent_api_key: "your-64-char-hex-api-key"
agent_ca_cert: "C:\\certs\\ca.crt"
agent_hostname: "win-workstation-01"
agent_paths:
  - "C:\\Users"
  - "C:\\Data"
agent_excludes:
  - "*.tmp"
  - "Thumbs.db"
  - "desktop.ini"
```

## Verify the Installation

Open a new elevated PowerShell window and run:

```powershell
Get-Service HitStorAgent
hitstor-agent show-id
```

Test connectivity to the server:

```powershell
hitstor-agent ping --server backup-server.internal:8420 --ca-cert C:\certs\ca.crt
```

## Running a Manual Backup

```powershell
hitstor-agent backup C:\Users C:\Data
```

Or with explicit flags:

```powershell
hitstor-agent backup --server backup-server:8420 --api-key <key> --ca-cert ca.crt C:\Users C:\Data
```

## Windows Service Management

The agent can run as a Windows Service for unattended scheduled backups. Administrator privileges are required for all service operations.

### Install the Service

If you did not select the service component during installation, you can install it manually:

```powershell
hitstor-agent service install --config "$env:ProgramData\HitStor\agent-config.yaml"
```

Options:

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | `HitStorAgent` | Windows Service name |
| `--display` | `HitStor Backup Agent` | Display name in the Services console |
| `--config` | (none) | Configuration file the service is started with; without it the service account's default (`%AppData%\hitstor\config.yaml`) is used |

`--server`, `--api-key`, and `--ca-cert` given to `service install` are also passed to the service.

### Start and Stop

```powershell
hitstor-agent service start
hitstor-agent service stop
```

You can also manage the service through the Windows Services console (`services.msc`).

### Uninstall the Service

```powershell
hitstor-agent service stop
hitstor-agent service uninstall
```

### Service Behavior

When running as a service, the agent runs in [daemon mode](../reference/agent-commands/daemon.md): it listens on the listen port (`HITSTOR_AGENT_LISTEN_PORT` in `agent.env`, default 8422) for the server, and polls the server every 5 minutes for its backup jobs, which run on the schedules set in the dashboard. It reads `agent.env` from `%ProgramData%\HitStor\` and logs to `%ProgramData%\HitStor\logs\agent.log`.

## Known Limitations

| Area | Limitation |
|------|-----------|
| **UID / GID** | Always stored as 0. Windows has no Unix ownership model. |
| **File mode** | Simplified: `0666` for normal files, `0444` for read-only. Not Unix permissions. |
| **CTime** | Represents file creation time on Windows (not change time as on Unix). |
| **Symlinks** | Require Developer Mode or admin privileges. Skipped if unavailable. |
| **NTFS Alternate Data Streams** | Not backed up. |
| **Windows ACLs** | Not backed up. Permissions are not preserved during restore. |
| **Long paths (>260 chars)** | Supported via Go 1.24 long path handling. |

## Uninstall

Use **Add/Remove Programs** in Windows Settings, or run the uninstaller directly:

```powershell
& "$env:ProgramFiles\HitStor Agent\uninstall.exe"
```

The uninstaller stops and removes the Windows Service (if installed) and the `HitStor Agent` firewall rule, removes the binary, PATH entry, Start Menu shortcuts, and registry entries, and deletes the agent's files in `%ProgramData%\HitStor\` (`agent.env`, `agent-config.yaml`, the agent token, and its certificates). Register the agent again after a reinstall.

## Next Steps

- [Getting Started](../user-guide/getting-started.md) -- learn the full backup lifecycle.
- [Configuration](../user-guide/configuration.md) -- config file schema and environment variables.
