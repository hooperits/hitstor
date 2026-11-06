# Despliegue de agentes

Esta guía explica cómo instalar y configurar el binario `hitstor-agent` en equipos Linux y Windows.

## Agente para Linux { #linux-agent }

### Instalar con el instalador .run (recomendado) { #install-with-the-run-installer-recommended }

El instalador `.run` incluye un asistente interactivo que configura el agente e inicia el servicio automáticamente:

```bash
sudo ./hitstor-agent-X.Y.Z-linux-amd64.run
```

El asistente pide un solo valor, el **puerto de escucha** (puerto gRPC que usa el servidor para administrar el agente; predeterminado `8422`). Luego instala el binario y las unidades de systemd, escribe `/etc/hitstor/agent.env`, inicia el servicio `hitstor-agent` y muestra el **token del agente** con las instrucciones para agregarlo desde el dashboard. No se necesita la dirección del servidor ni una clave de API al instalar: el servidor las envía cuando usted registra el agente.

### Instalación no interactiva { #non-interactive-install }

Para automatizar, use `--no-wizard`; el único ajuste es el puerto de escucha:

```bash
sudo HITSTOR_AGENT_LISTEN_PORT=8422 \
     ./hitstor-agent-X.Y.Z-linux-amd64.run --no-wizard
```

Consulte [Instalación rápida en Linux](../installation/linux-installer.md) para ver todos los detalles.

### Instalar desde un paquete { #install-from-package }

Como alternativa, instale el agente desde un paquete deb o rpm:

```bash
# Debian / Ubuntu
sudo apt install ./hitstor-agent_X.Y.Z_amd64.deb

# RHEL / Fedora / AlmaLinux
sudo rpm -i hitstor-agent-X.Y.Z-1.x86_64.rpm
```

El paquete no inicia el agente. Habilítelo con `sudo systemctl enable --now hitstor-agent`; el token se genera en el primer inicio. Consulte [Instalación](../installation/index.md) para conocer otros métodos de instalación.

### Configurar el agente { #configure-the-agent }

#### Agente administrado desde el dashboard (recomendado) { #dashboard-managed-agent-recommended }

El agente lee su puerto de escucha de `/etc/hitstor/agent.env`:

```bash
# /etc/hitstor/agent.env
HITSTOR_AGENT_LISTEN_PORT=8422
```

Cuando el servidor registra el agente desde el dashboard, el agente reescribe este archivo con la dirección del servidor y la clave de API (`HITSTOR_AGENT_SERVER`, `HITSTOR_AGENT_API_KEY`). El token del agente se guarda en `/etc/hitstor/agent-token`.

Para ver el token del agente:

```bash
sudo hitstor-agent show-id
```

#### Registro manual (CLI) { #manual-cli-registration }

Si registró el agente en el servidor con `hitstor serve agent add <name>`, escriba usted mismo la dirección del servidor y la clave de API impresa en `/etc/hitstor/agent.env` y luego reinicie el servicio:

```bash
# /etc/hitstor/agent.env
HITSTOR_AGENT_SERVER=backup.example.com:8420
HITSTOR_AGENT_API_KEY=a3f8c1d9e4b7...6f2a0e5d
```

Defina los permisos del archivo para que solo root pueda leer la clave:

```bash
sudo chmod 0600 /etc/hitstor/agent.env
sudo chown root:root /etc/hitstor/agent.env
```

### Variables de entorno { #environment-variables }

| Variable | Descripción |
|----------|-------------|
| `HITSTOR_AGENT_LISTEN_PORT` | Puerto de escucha gRPC para la administración desde el dashboard (predeterminado: 8422) |
| `HITSTOR_AGENT_SERVER` | Dirección del servidor en formato `host:port` (se escribe al registrar) |
| `HITSTOR_AGENT_API_KEY` | Clave de API hexadecimal de 64 caracteres (se escribe al registrar) |
| `HITSTOR_AGENT_CA_CERT` | Ruta al certificado de la CA para la verificación TLS (opcional; sin ella, el agente fija el certificado del servidor en el primer uso) |

El daemon y el servicio de Windows leen `agent.env` por su cuenta; los comandos puntuales como `hitstor-agent ping` o `backup` leen estas variables del entorno, de `--server`/`--api-key`/`--ca-cert`, o de `agent_server`/`agent_api_key`/`agent_ca_cert` en el archivo indicado con `--config`.

### Obtener el token del agente { #retrieve-the-agent-token }

Los agentes administrados desde el dashboard generan un token en su primer inicio. Para mostrarlo:

```bash
sudo hitstor-agent show-id
```

El token se guarda en `/etc/hitstor/agent-token` con modo `0600`.

### Probar la conexión { #test-the-connection }

Una vez registrado el agente, verifique que pueda conectarse al servidor (las variables provienen de `agent.env`):

```bash
sudo sh -c 'set -a; . /etc/hitstor/agent.env; hitstor-agent ping'
```

Salida esperada:

```
Server: backup.example.com:8420
Version: X.Y.Z
Repo ID: 03b9cb33...
```

Si falla, consulte [Solución de problemas](troubleshooting.md) para ver los problemas de conexión más comunes.

### Ejecutar un respaldo manual { #run-a-manual-backup }

Cree un respaldo de uno o varios directorios:

```bash
sudo sh -c 'set -a; . /etc/hitstor/agent.env; hitstor-agent backup /etc /home /var/lib'
```

El agente envía los datos al servidor por gRPC. El progreso se muestra en la terminal.

### Modo daemon { #daemon-mode }

En modo daemon, el agente se ejecuta de forma continua con dos funciones:

1. **Servidor de administración**: escucha en el puerto 8422 los comandos gRPC del servidor HitStor (registro, exploración del sistema de archivos y de los plugins, instalación de plugins, restauraciones, activación inmediata)
2. **Consulta de respaldos**: consulta al servidor su programación de respaldo y ejecuta los respaldos automáticamente

```bash
hitstor-agent daemon
```

Si el agente aún no está registrado, el daemon espera el registro desde el dashboard y luego ejecuta las tareas de respaldo creadas para él.

### Integración con systemd { #systemd-integration }

El paquete del agente incluye dos unidades de systemd:

#### Modo daemon (recomendado) { #daemon-mode-recommended }

Habilite el servicio daemon persistente:

```bash
sudo systemctl enable hitstor-agent
sudo systemctl start hitstor-agent
```

Esto inicia `hitstor-agent daemon` como un servicio de larga duración. El agente escucha los comandos de administración y consulta al servidor si hay cambios en la programación.

#### Unidad timer { #timer-unit }

El paquete también incluye `hitstor-agent.timer` (diario a las 02:00). Este activa `hitstor-agent.service`, el mismo daemon, en lugar de ejecutar un respaldo puntual, así que no aporta nada si el daemon está habilitado. Para respaldos puntuales programados por el sistema operativo, consulte [Programación → Temporizador de systemd](scheduling.md#systemd-timer-alternative).

#### Verificar el estado del agente { #check-agent-status }

```bash
sudo systemctl status hitstor-agent
sudo journalctl -u hitstor-agent -f
```

## Agente para Windows { #windows-agent }

### Instalar el agente { #install-the-agent }

Descargue el instalador del agente para Windows (`hitstor-agent-X.Y.Z-windows-amd64-setup.exe`) desde la página de versiones de HitStor y ejecútelo como administrador. El instalador:

- Pide el **puerto de escucha** (predeterminado `8422`)
- Instala `hitstor-agent.exe` y agrega su carpeta al PATH del sistema
- Escribe `C:\ProgramData\HitStor\agent.env` y `agent-config.yaml`, legibles solo por SYSTEM y Administradores
- Agrega una regla del Firewall de Windows para el puerto de escucha
- Registra e inicia el servicio de Windows `HitStorAgent` ("HitStor Backup Agent"), que ejecuta el daemon
- Muestra el **token del agente**

Luego agregue el agente desde el dashboard (**Agentes → + Agregar agente**) con su dirección IP y su token. Consulte [Instalación del agente en Windows](../installation/windows-agent.md).

### Configurar el agente { #configure-the-agent_1 }

El registro desde el dashboard escribe la dirección del servidor y la clave de API en `C:\ProgramData\HitStor\agent.env`, que el servicio lee. Para un agente registrado con `hitstor serve agent add`, escriba usted mismo ese archivo:

```ini
# C:\ProgramData\HitStor\agent.env
HITSTOR_AGENT_SERVER=backup.example.com:8420
HITSTOR_AGENT_API_KEY=a3f8c1d9e4b7...6f2a0e5d
# Opcional: certificado de la CA para la verificación TLS
# HITSTOR_AGENT_CA_CERT=C:\ProgramData\HitStor\ca.crt
```

Luego reinicie el servicio (`hitstor-agent service stop`, `hitstor-agent service start`).

### Probar la conexión { #test-the-connection_1 }

Abra una ventana de PowerShell con privilegios elevados e indique el servidor y la clave (la línea de comandos no lee `agent.env`):

```powershell
hitstor-agent ping --server backup.example.com:8420 --api-key KEY
```

### Ejecutar un respaldo manual { #run-a-manual-backup_1 }

```powershell
hitstor-agent backup --server backup.example.com:8420 --api-key KEY C:\Users C:\Data
```

### Administrar el servicio de Windows { #manage-the-windows-service }

El instalador registra el servicio. Para administrarlo manualmente:

```powershell
hitstor-agent service install
hitstor-agent service start
hitstor-agent service stop
hitstor-agent service uninstall
```

También puede administrar el servicio desde la consola de Servicios de Windows (`services.msc`).

## Despliegue en múltiples equipos { #deploying-to-multiple-machines }

Para despliegues a gran escala, use herramientas de gestión de configuración (Ansible, Puppet, Chef o similares) para:

1. Instalar el paquete `hitstor-agent` y habilitar el servicio `hitstor-agent`
2. El agente genera automáticamente un token en su primer inicio
3. Recopilar los tokens de los agentes (con `hitstor-agent show-id`) y agregar los agentes desde el dashboard
4. Crear tareas de respaldo para ellos en el dashboard (**Respaldos → + Nueva tarea de respaldo**)

Como alternativa, para despliegues heredados:

1. Registre cada agente en el servidor con `hitstor serve agent add`
2. Despliegue `/etc/hitstor/agent.env` con la dirección del servidor y la clave de API específica de cada equipo
3. Despliegue el archivo del certificado de la CA si no quiere que el agente fije el certificado del servidor en el primer uso (consulte [Certificados TLS](tls-certificates.md#agent-ca-certificate))
4. Habilite e inicie el servicio systemd `hitstor-agent`

## Próximos pasos { #next-steps }

- [Administración de agentes](agent-management.md): agregue agentes desde el dashboard o la CLI
- [Programación](scheduling.md): configure programaciones de respaldo para los agentes desplegados
- [Monitoreo](monitoring.md): configure alertas sobre el estado de los agentes
- [Solución de problemas](troubleshooting.md): resuelva problemas de conexión y de respaldo
