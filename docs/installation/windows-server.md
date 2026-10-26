# Windows Server Installation

HitStor provides an NSIS installer for deploying the backup server on Windows. The installer adds `hitstor.exe` to the system PATH, creates the configuration directory, registers HitStor as a Windows Service, and registers an uninstaller in Add/Remove Programs.

## Requirements

- Windows 10 (1809+) or Windows Server 2016+
- Architecture: amd64
- Administrator privileges (required for installation)

## Download

Download the server installer from the [GitHub Releases](https://github.com/hooperits/hitstor/releases) page:

- `hitstor-x.y.z-windows-amd64-setup.exe`

## Install

1. Run the installer:

    ```
    hitstor-x.y.z-windows-amd64-setup.exe
    ```

2. Accept the license agreement.
3. Choose the installation directory (default: `C:\Program Files\HitStor\`).
4. Choose the components (both selected by default):
    - **Windows Firewall rules** -- allow inbound TCP 8420 (agents) and 8421 (dashboard) for `hitstor.exe`.
    - **Install as Windows Service** -- register and start the `HitStor` service.
5. Click **Install**.

The installer performs the following steps:

- Stops the `HitStor` service if it is already installed (upgrade).
- Copies `hitstor.exe` to the installation directory and adds it to the system `PATH`.
- Creates `%ProgramData%\HitStor\`, restricted to Administrators and SYSTEM, with `config.yaml.example`.
- Creates `%ProgramData%\HitStor\config.yaml` from the example if it does not exist (an existing one is preserved).
- Registers the `HitStor` Windows Service, which runs `hitstor serve --config %ProgramData%\HitStor\config.yaml` as LocalSystem with automatic start, and starts it.
- Registers the application in Add/Remove Programs.

### Unattended install and upgrade

Run the installer with `/S` from an elevated prompt. All components are installed, and running the same command with a newer installer upgrades in place (the service is stopped, the binary replaced, and the service re-registered and started):

```powershell
.\hitstor-x.y.z-windows-amd64-setup.exe /S
```

## Verify the Installation

Open a new PowerShell or Command Prompt window (required so the updated PATH takes effect) and run:

```powershell
hitstor version
Get-Service HitStor
```

`hitstor version` prints the version, commit hash, and build date.

## First Login

With no repositories configured, the server starts in setup mode (dashboard only). Open `https://<server>:8421`.

If `config.yaml` has no dashboard password, the server generates one on first start, saves its hash to `config.yaml`, and logs the plaintext once to `%ProgramData%\HitStor\logs\server.log`:

```powershell
Select-String -Path "$env:ProgramData\HitStor\logs\server.log" -Pattern "generated one"
```

Log in with it and change it under **Settings**. Then create a repository from the **Storage** page.

## Configuration

Edit `%ProgramData%\HitStor\config.yaml` (see `config.yaml.example` in the same folder for all options) and restart the service:

```powershell
notepad "$env:ProgramData\HitStor\config.yaml"
Restart-Service HitStor
```

## Managing the Service

The installer manages the service for you. To do it by hand (elevated prompt):

```powershell
hitstor service install --config "$env:ProgramData\HitStor\config.yaml"
hitstor service start
hitstor service stop
hitstor service uninstall
```

All subcommands accept `--name` (default `HitStor`). You can also use `Start-Service`/`Stop-Service HitStor` or the Services console. The service writes to `%ProgramData%\HitStor\logs\server.log`.

To run the server in the foreground instead (for troubleshooting), stop the service and run:

```powershell
hitstor serve --config "$env:ProgramData\HitStor\config.yaml"
```

## Firewall Configuration

The **Windows Firewall rules** component creates the rules `HitStor Server gRPC` (8420) and `HitStor Server Dashboard` (8421), open to any remote address. To restrict them, for example to an admin network:

```powershell
Set-NetFirewallRule -DisplayName "HitStor Server Dashboard" -RemoteAddress 10.0.0.0/24
```

If you skipped that component, create the rules yourself:

```powershell
New-NetFirewallRule -DisplayName "HitStor gRPC" -Direction Inbound -LocalPort 8420 -Protocol TCP -Action Allow
New-NetFirewallRule -DisplayName "HitStor Dashboard" -Direction Inbound -LocalPort 8421 -Protocol TCP -Action Allow
```

## Uninstall

Use **Add/Remove Programs** in Windows Settings, or run the uninstaller directly:

```powershell
& "$env:ProgramFiles\HitStor\uninstall.exe"
```

The uninstaller stops and removes the service and firewall rules, and removes the binary, PATH entry, Start Menu shortcuts, and registry entries. Configuration files in `%ProgramData%\HitStor\` and repository data are not deleted.

## Next Steps

- [Windows Agent Installation](windows-agent.md) -- install the agent on Windows workstations.
- [Getting Started](../user-guide/getting-started.md) -- create your first backup.
- [Configuration](../user-guide/configuration.md) -- config file options and environment variables.
