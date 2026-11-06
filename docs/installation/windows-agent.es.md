# Instalación del agente para Windows

El agente de HitStor es un binario liviano que envía respaldos cifrados y deduplicados desde un equipo Windows a un servidor HitStor remoto por gRPC. El instalador NSIS registra el agente como servicio de Windows (opción seleccionada de forma predeterminada), que luego se administra desde el dashboard de HitStor.

## Requisitos { #requirements }

- Windows 10 (1809+) o Windows Server 2016+
- Arquitectura: amd64 (el instalador es solo para amd64; un binario arm64 puede compilarse desde el código fuente con `make agent-windows-arm64`)
- Acceso de red al servidor HitStor (puerto predeterminado 8420), y desde el servidor al puerto de escucha del agente (predeterminado 8422)
- Privilegios de administrador (necesarios para la instalación y la administración del servicio)

## Descarga { #download }

Descargue el instalador del agente desde la página de [GitHub Releases](https://github.com/hooperits/hitstor/releases):

- `hitstor-agent-x.y.z-windows-amd64-setup.exe`

## Instalación { #install }

1. Ejecute el instalador:

    ```
    hitstor-agent-x.y.z-windows-amd64-setup.exe
    ```

2. Acepte el acuerdo de licencia.
3. Elija el directorio de instalación (predeterminado: `C:\Program Files\HitStor Agent\`).
4. En la página **Components**, seleccione los componentes que desea instalar:
    - **HitStor Agent** (obligatorio): instala el binario del agente y la configuración.
    - **Install as Windows Service** (seleccionado de forma predeterminada): registra e inicia el agente como servicio de Windows y abre su puerto de escucha en el Firewall de Windows.
5. En la página **Agent Configuration**, conserve el puerto de escucha (predeterminado `8422`) salvo que otro servicio lo use. El servidor se conecta a este puerto para registrar y administrar el agente.
6. Haga clic en **Install**. La última página muestra el puerto de escucha, el **token del agente** y los pasos siguientes.

El instalador realiza los siguientes pasos:

- Copia `hitstor-agent.exe` al directorio de instalación y agrega el directorio al `PATH` del sistema.
- Crea `%ProgramData%\HitStor\`, restringido a SYSTEM y Administradores, con `agent-config.yaml.example`, `agent.env` (`HITSTOR_AGENT_LISTEN_PORT`) y un `agent-config.yaml` sin rutas de respaldo (si ya existe, se conserva). Al actualizar, se conservan las credenciales de `agent.env`.
- Crea un acceso directo en el menú Inicio para el desinstalador.
- Si se seleccionó el componente de servicio, registra el servicio de Windows `HitStorAgent` con `--config %ProgramData%\HitStor\agent-config.yaml`, agrega la regla de firewall `HitStor Agent` para el puerto de escucha e inicia el servicio. El servicio genera el token del agente (`%ProgramData%\HitStor\agent-token`) en el primer inicio.

## Registrar el agente { #register-the-agent }

En el dashboard de HitStor, vaya a **Agentes → Agregar agente** e ingrese la dirección IP de este equipo, el puerto de escucha y el token del agente que se mostró al final de la instalación (`hitstor-agent show-id` lo vuelve a mostrar desde una consola con privilegios elevados). El servidor envía al agente su dirección y una clave de API, que el agente guarda en `agent.env`. Luego cree tareas de respaldo en **Respaldos**; las rutas de respaldo y las programaciones se administran desde el dashboard.

## Configuración manual { #manual-configuration }

Un agente administrado desde el dashboard no necesita editar archivos de configuración. Para usar la CLI del agente con un servidor sin registro desde el dashboard (por ejemplo, con una clave de API de `hitstor serve agent add <nombre>`), use un archivo de configuración. El servicio lee `%ProgramData%\HitStor\agent-config.yaml` (indicado con `--config`) y `%ProgramData%\HitStor\agent.env`; los comandos de la CLI leen `%AppData%\hitstor\config.yaml` salvo que indique `--config`.

Edite el archivo para todo el sistema (`agent-config.yaml.example`, en la misma carpeta, enumera todas las opciones):

```powershell
notepad "$env:ProgramData\HitStor\agent-config.yaml"
```

### Configuración obligatoria { #required-settings }

Como mínimo, configure la dirección del servidor y la clave de API:

```yaml
agent_server: "backup-server.internal:8420"
agent_api_key: "your-64-char-hex-api-key"
```

### Ejemplo completo { #full-example }

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

## Verificar la instalación { #verify-the-installation }

Abra una nueva ventana de PowerShell con privilegios elevados y ejecute:

```powershell
Get-Service HitStorAgent
hitstor-agent show-id
```

Pruebe la conectividad con el servidor:

```powershell
hitstor-agent ping --server backup-server.internal:8420 --ca-cert C:\certs\ca.crt
```

## Ejecutar un respaldo manual { #running-a-manual-backup }

```powershell
hitstor-agent backup C:\Users C:\Data
```

O con flags explícitos:

```powershell
hitstor-agent backup --server backup-server:8420 --api-key <key> --ca-cert ca.crt C:\Users C:\Data
```

## Administración del servicio de Windows { #windows-service-management }

El agente puede ejecutarse como servicio de Windows para respaldos programados desatendidos. Todas las operaciones del servicio requieren privilegios de administrador.

### Instalar el servicio { #install-the-service }

Si no seleccionó el componente de servicio durante la instalación, puede instalarlo manualmente:

```powershell
hitstor-agent service install --config "$env:ProgramData\HitStor\agent-config.yaml"
```

Opciones:

| Flag | Predeterminado | Descripción |
|------|----------------|-------------|
| `--name` | `HitStorAgent` | Nombre del servicio de Windows |
| `--display` | `HitStor Backup Agent` | Nombre visible en la consola de Servicios |
| `--config` | (ninguno) | Archivo de configuración con el que se inicia el servicio; sin él se usa el predeterminado de la cuenta del servicio (`%AppData%\hitstor\config.yaml`) |

`--server`, `--api-key` y `--ca-cert` indicados en `service install` también se pasan al servicio.

### Iniciar y detener { #start-and-stop }

```powershell
hitstor-agent service start
hitstor-agent service stop
```

También puede administrar el servicio desde la consola de Servicios de Windows (`services.msc`).

### Desinstalar el servicio { #uninstall-the-service }

```powershell
hitstor-agent service stop
hitstor-agent service uninstall
```

### Comportamiento del servicio { #service-behavior }

Cuando se ejecuta como servicio, el agente funciona en [modo daemon](../reference/agent-commands/daemon.md): escucha en el puerto de escucha (`HITSTOR_AGENT_LISTEN_PORT` en `agent.env`, predeterminado 8422) las conexiones del servidor y consulta al servidor cada 5 minutos sus tareas de respaldo, que se ejecutan según las programaciones definidas en el dashboard. Lee `agent.env` desde `%ProgramData%\HitStor\` y escribe su registro en `%ProgramData%\HitStor\logs\agent.log`.

## Limitaciones conocidas { #known-limitations }

| Área | Limitación |
|------|-----------|
| **UID / GID** | Siempre se almacenan como 0. Windows no tiene el modelo de propiedad de Unix. |
| **Modo de archivo** | Simplificado: `0666` para archivos normales, `0444` para archivos de solo lectura. No son permisos de Unix. |
| **CTime** | En Windows representa la fecha de creación del archivo (no la fecha de cambio, como en Unix). |
| **Enlaces simbólicos** | Requieren el Modo de desarrollador o privilegios de administrador. Se omiten si no están disponibles. |
| **Flujos de datos alternativos de NTFS** | No se respaldan. |
| **ACL de Windows** | No se respaldan. Los permisos no se conservan durante la restauración. |
| **Rutas largas (>260 caracteres)** | Compatibles gracias al manejo de rutas largas de Go 1.24. |

## Desinstalación { #uninstall }

Use **Agregar o quitar programas** en la Configuración de Windows, o ejecute el desinstalador directamente:

```powershell
& "$env:ProgramFiles\HitStor Agent\uninstall.exe"
```

El desinstalador detiene y elimina el servicio de Windows (si está instalado) y la regla de firewall `HitStor Agent`; elimina el binario, la entrada del PATH, los accesos directos del menú Inicio y las entradas del registro de Windows, y borra los archivos del agente en `%ProgramData%\HitStor\` (`agent.env`, `agent-config.yaml`, el token del agente y sus certificados). Vuelva a registrar el agente después de reinstalarlo.

## Próximos pasos { #next-steps }

- [Primeros pasos](../user-guide/getting-started.md): conozca el ciclo de vida completo de los respaldos.
- [Configuración](../user-guide/configuration.md): esquema del archivo de configuración y variables de entorno.
