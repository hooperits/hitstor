# Instalación del agente para Windows

El agente de HitStor es un binario liviano que envía respaldos cifrados y deduplicados desde un equipo Windows a un servidor HitStor remoto por gRPC. El instalador NSIS ofrece una casilla opcional para registrar el agente como servicio de Windows y ejecutar respaldos desatendidos.

## Requisitos { #requirements }

- Windows 10 (1809+) o Windows Server 2016+
- Arquitectura: amd64 o arm64
- Acceso de red al servidor HitStor (puerto predeterminado 8420)
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
    - **Install as Windows Service** (opcional): registra e inicia el agente como servicio de Windows para respaldos programados desatendidos.
5. Haga clic en **Install**.

El instalador realiza los siguientes pasos:

- Copia `hitstor-agent.exe` al directorio de instalación.
- Agrega el directorio de instalación al `PATH` del sistema.
- Crea un directorio de configuración en `%ProgramData%\HitStor\`.
- Copia `agent-config.yaml.example` al directorio de configuración.
- Crea un acceso directo en el menú Inicio para el desinstalador.
- Si se seleccionó el componente de servicio, registra e inicia el servicio de Windows `HitStorAgent`.

## Configuración { #configuration }

El agente lee la configuración desde una de estas ubicaciones:

| Ubicación | Alcance |
|-----------|---------|
| `%AppData%\hitstor\config.yaml` | Configuración por usuario |
| `%ProgramData%\HitStor\agent-config.yaml` | Configuración para todo el sistema |

Copie la configuración de ejemplo y edítela:

```powershell
copy "%ProgramData%\HitStor\agent-config.yaml.example" "%ProgramData%\HitStor\agent-config.yaml"
notepad "%ProgramData%\HitStor\agent-config.yaml"
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

Abra una nueva ventana de PowerShell y ejecute:

```powershell
hitstor-agent version
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
hitstor-agent service install --server backup-server:8420 --api-key <key>
```

Opciones:

| Flag | Predeterminado | Descripción |
|------|----------------|-------------|
| `--name` | `HitStorAgent` | Nombre del servicio de Windows |
| `--display` | `HitStor Backup Agent` | Nombre visible en la consola de Servicios |
| `--config` | (detección automática) | Ruta al archivo de configuración |

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

Cuando se ejecuta como servicio, el agente lee toda la configuración desde el archivo de configuración (`agent_server`, `agent_api_key`, `agent_paths`, etc.) y realiza una sola pasada de respaldo. Para programar respaldos recurrentes, configure el tipo de inicio del servicio o use el Programador de tareas de Windows.

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

El desinstalador detiene y elimina el servicio de Windows (si está instalado), y elimina el binario, la entrada del PATH, los accesos directos del menú Inicio y las entradas del registro de Windows.

## Próximos pasos { #next-steps }

- [Primeros pasos](../user-guide/getting-started.md): conozca el ciclo de vida completo de los respaldos.
- [Configuración](../user-guide/configuration.md): esquema del archivo de configuración y variables de entorno.
