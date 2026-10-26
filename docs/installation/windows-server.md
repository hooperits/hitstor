# Windows Server Installation

HitStor provides an NSIS installer for deploying the backup server on Windows. The installer adds `hitstor.exe` to the system PATH, creates configuration directories, and registers an uninstaller in Add/Remove Programs.

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
4. Click **Install**.

The installer performs the following steps:

- Copies `hitstor.exe` to the installation directory.
- Adds the installation directory to the system `PATH`.
- Creates a configuration directory at `%ProgramData%\HitStor\`.
- Copies `config.yaml.example` to the configuration directory.
- Creates a Start Menu shortcut for the uninstaller.
- Registers the application in Add/Remove Programs.

## Verify the Installation

Open a new PowerShell or Command Prompt window (required so the updated PATH takes effect) and run:

```powershell
hitstor version
```

This prints the version, commit hash, and build date.

## Configuration

Copy the example configuration and edit it:

```powershell
copy "%ProgramData%\HitStor\config.yaml.example" "%ProgramData%\HitStor\config.yaml"
notepad "%ProgramData%\HitStor\config.yaml"
```

## Initialize a Repository

Create a new encrypted repository:

```powershell
hitstor init --repo C:\HitStorRepo --password "my-secret-password"
```

This creates the repository directory structure and derives an encryption key from the provided password.

## Start the Server

Start the HitStor server with the gRPC listener and web dashboard:

```powershell
hitstor serve --repo C:\HitStorRepo --password "my-secret-password" --port 8420
```

By default, the server listens on:

- Port **8420** -- gRPC protocol (agent connections)
- Port **8421** -- Web dashboard

To run the server in the background, consider using [NSSM](https://nssm.cc/) or Windows Task Scheduler to register `hitstor serve` as a Windows Service.

## Firewall Configuration

If agents on other machines need to connect, allow inbound traffic on the HitStor ports:

```powershell
New-NetFirewallRule -DisplayName "HitStor gRPC" -Direction Inbound -LocalPort 8420 -Protocol TCP -Action Allow
New-NetFirewallRule -DisplayName "HitStor Dashboard" -Direction Inbound -LocalPort 8421 -Protocol TCP -Action Allow
```

## Uninstall

Use **Add/Remove Programs** in Windows Settings, or run the uninstaller directly:

```powershell
& "$env:ProgramFiles\HitStor\uninstall.exe"
```

The uninstaller removes the binary, PATH entry, Start Menu shortcuts, and registry entries. Configuration files in `%ProgramData%\HitStor\` and repository data are not deleted.

## Next Steps

- [Windows Agent Installation](windows-agent.md) -- install the agent on Windows workstations.
- [Getting Started](../user-guide/getting-started.md) -- create your first backup.
- [Configuration](../user-guide/configuration.md) -- config file options and environment variables.
