# Windows Agent Installation

The HitStor agent is a lightweight binary that sends encrypted, deduplicated backups from a Windows machine to a remote HitStor server over gRPC. The NSIS installer provides an optional checkbox to register the agent as a Windows Service for unattended backups.

## Requirements

- Windows 10 (1809+) or Windows Server 2016+
- Architecture: amd64 or arm64
- Network access to the HitStor server (default port 8420)
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
    - **Install as Windows Service** (optional) -- registers and starts the agent as a Windows Service for unattended scheduled backups.
5. Click **Install**.

The installer performs the following steps:

- Copies `hitstor-agent.exe` to the installation directory.
- Adds the installation directory to the system `PATH`.
- Creates a configuration directory at `%ProgramData%\HitStor\`.
- Copies `agent-config.yaml.example` to the configuration directory.
- Creates a Start Menu shortcut for the uninstaller.
- If the service component is selected, registers and starts the `HitStorAgent` Windows Service.

## Configuration

The agent reads configuration from one of these locations:

| Location | Scope |
|----------|-------|
| `%AppData%\hitstor\config.yaml` | Per-user configuration |
| `%ProgramData%\HitStor\agent-config.yaml` | System-wide configuration |

Copy the example configuration and edit it:

```powershell
copy "%ProgramData%\HitStor\agent-config.yaml.example" "%ProgramData%\HitStor\agent-config.yaml"
notepad "%ProgramData%\HitStor\agent-config.yaml"
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

Open a new PowerShell window and run:

```powershell
hitstor-agent version
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
hitstor-agent service install --server backup-server:8420 --api-key <key>
```

Options:

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | `HitStorAgent` | Windows Service name |
| `--display` | `HitStor Backup Agent` | Display name in the Services console |
| `--config` | (auto-detected) | Path to the configuration file |

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

When running as a service, the agent reads all settings from the configuration file (`agent_server`, `agent_api_key`, `agent_paths`, etc.) and performs a single backup pass. To schedule recurring backups, configure the service start type or use Windows Task Scheduler.

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

The uninstaller stops and removes the Windows Service (if installed), removes the binary, PATH entry, Start Menu shortcuts, and registry entries.

## Next Steps

- [Getting Started](../user-guide/getting-started.md) -- learn the full backup lifecycle.
- [Configuration](../user-guide/configuration.md) -- config file schema and environment variables.
