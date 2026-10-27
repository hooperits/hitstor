# Despliegue de agentes

Esta guía explica cómo instalar y configurar el binario `hitstor-agent` en equipos Linux y Windows.

## Agente para Linux { #linux-agent }

### Instalar con el instalador .run (recomendado) { #install-with-the-run-installer-recommended }

El instalador `.run` incluye un asistente interactivo que configura el agente e inicia el servicio automáticamente:

```bash
sudo ./hitstor-agent-X.Y.Z-linux-amd64.run
```

El asistente ofrece dos modos de configuración:

#### Administrado desde el dashboard (recomendado) { #dashboard-managed-recommended }

El agente genera un token y espera el registro desde el dashboard del servidor. No se necesita la dirección del servidor ni una clave de API al momento de la instalación.

El asistente solicita:

1. **Modo de configuración**: seleccione `1` para administrado desde el dashboard
2. **Puerto de escucha**: puerto gRPC para las conexiones del servidor (predeterminado: 8422)
3. **Rutas de respaldo**: opcionales; pueden configurarse más adelante desde el dashboard

Al terminar la instalación, el asistente muestra el **token del agente** e instrucciones para agregar el agente desde el dashboard.

#### Heredado (manual) { #legacy-manual }

El agente se configura con la dirección del servidor y una clave de API al momento de la instalación. Use este modo si registró el agente con `hitstor serve agent add`.

El asistente solicita:

1. **Modo de configuración**: seleccione `2` para heredado
2. **Dirección del servidor**: `host:port` del servidor HitStor
3. **Clave de API**: clave hexadecimal de 64 caracteres obtenida con `hitstor serve agent add <name>` (entrada oculta)
4. **Prueba de conexión**: verifica la conectividad antes de continuar
5. **Rutas de respaldo**: directorios a respaldar, separados por comas (predeterminado: `/etc,/home`)

### Instalación no interactiva { #non-interactive-install }

Para automatizar, use `--no-wizard` con variables de entorno.

**Modo administrado desde el dashboard** (predeterminado cuando no se definen las variables de servidor y clave):

```bash
sudo HITSTOR_AGENT_LISTEN_PORT=8422 \
     ./hitstor-agent-X.Y.Z-linux-amd64.run --no-wizard
```

**Modo heredado** (se activa al definir el servidor y la clave de API):

```bash
sudo HITSTOR_AGENT_SERVER=backup.example.com:8420 \
     HITSTOR_AGENT_API_KEY=<64-char-hex-key> \
     HITSTOR_AGENT_PATHS=/etc,/home \
     ./hitstor-agent-X.Y.Z-linux-amd64.run --no-wizard
```

Consulte [Instalación rápida en Linux](../installation/linux-installer.md) para ver todos los detalles.

### Instalar desde un paquete { #install-from-package }

Como alternativa, instale el agente desde un paquete deb o rpm:

```bash
# Debian / Ubuntu
sudo apt install ./hitstor-agent_1.0.0_amd64.deb

# RHEL / Fedora / AlmaLinux
sudo rpm -i hitstor-agent-1.0.0.x86_64.rpm
```

Consulte [Instalación](../installation/index.md) para conocer otros métodos de instalación.

### Configurar el agente { #configure-the-agent }

La configuración depende del modo de configuración elegido.

#### Agente administrado desde el dashboard { #dashboard-managed-agent }

El agente lee su puerto de escucha desde `/etc/hitstor/agent.env`:

```bash
# /etc/hitstor/agent.env
HITSTOR_AGENT_LISTEN_PORT=8422
```

Cuando el servidor registra el agente desde el dashboard, escribe automáticamente la dirección del servidor y la clave de API en este archivo. El token del agente se guarda en `/etc/hitstor/agent-token`.

Para ver el token del agente:

```bash
sudo hitstor-agent show-id
```

#### Agente heredado { #legacy-agent }

El agente lee la dirección del servidor y la clave de API desde `/etc/hitstor/agent.env`:

```bash
# /etc/hitstor/agent.env
HITSTOR_AGENT_SERVER=backup.example.com:8420
HITSTOR_AGENT_API_KEY=a3f8c1d9e4b7...6f2a0e5d
```

Ajuste los permisos del archivo para que solo root pueda leer la clave:

```bash
sudo chmod 0600 /etc/hitstor/agent.env
sudo chown root:root /etc/hitstor/agent.env
```

### Variables de entorno { #environment-variables }

| Variable | Descripción |
|----------|-------------|
| `HITSTOR_AGENT_LISTEN_PORT` | Puerto de escucha gRPC para la administración desde el dashboard (predeterminado: 8422) |
| `HITSTOR_AGENT_SERVER` | Dirección del servidor en formato `host:port` (modo heredado) |
| `HITSTOR_AGENT_API_KEY` | Clave de API hexadecimal de 64 caracteres (modo heredado) |
| `HITSTOR_AGENT_CA_CERT` | Ruta al certificado de la CA para la verificación TLS |

### Obtener el token del agente { #retrieve-the-agent-token }

Los agentes administrados desde el dashboard generan un token durante la instalación. Para mostrarlo:

```bash
sudo hitstor-agent show-id
```

El token se guarda en `/etc/hitstor/agent-token` con modo `0600`.

### Probar la conexión (modo heredado) { #test-the-connection-legacy-mode }

En los agentes heredados, verifique que el agente pueda comunicarse con el servidor:

```bash
hitstor-agent ping
```

Salida esperada:

```
Server: backup.example.com:8420
Status: connected
Agent:  web-server-01
```

Si falla, consulte [Solución de problemas](troubleshooting.md) para ver los problemas de conexión más comunes.

### Ejecutar un respaldo manual { #run-a-manual-backup }

Cree un respaldo de uno o más directorios:

```bash
hitstor-agent backup /etc /home /var/lib
```

El agente envía los datos al servidor por gRPC. El progreso se muestra en la terminal.

### Modo daemon { #daemon-mode }

En modo daemon, el agente se ejecuta de forma continua y cumple dos funciones:

1. **Servidor de administración**: escucha en el puerto 8422 los comandos gRPC del servidor HitStor (registro, exploración del sistema de archivos, configuración)
2. **Consulta de respaldos**: consulta periódicamente al servidor su programación de respaldos y ejecuta los respaldos automáticamente

```bash
hitstor-agent daemon
```

Si el agente aún no está registrado (modo administrado desde el dashboard), el daemon espera el registro desde el dashboard y comienza a respaldar una vez registrado.

### Integración con systemd { #systemd-integration }

El paquete del agente incluye dos unidades de systemd:

#### Modo daemon (recomendado) { #daemon-mode-recommended }

Habilite el servicio daemon persistente:

```bash
sudo systemctl enable hitstor-agent
sudo systemctl start hitstor-agent
```

Esto inicia `hitstor-agent daemon` como un servicio de larga duración. El agente escucha comandos de administración y consulta al servidor para obtener actualizaciones de la programación.

#### Modo timer { #timer-mode }

Para una ejecución al estilo cron, use en su lugar el timer de systemd:

```bash
sudo systemctl enable hitstor-agent.timer
sudo systemctl start hitstor-agent.timer
```

El timer ejecuta `hitstor-agent backup` en el intervalo configurado. Edite el timer para personalizar la programación:

```bash
sudo systemctl edit hitstor-agent.timer
```

```ini
[Timer]
OnCalendar=*-*-* 02:00:00
```

Consulte [Programación](scheduling.md) para obtener más detalles sobre la configuración del timer.

#### Verificar el estado del agente { #check-agent-status }

```bash
sudo systemctl status hitstor-agent
sudo journalctl -u hitstor-agent -f
```

## Agente para Windows { #windows-agent }

### Instalar el agente { #install-the-agent }

Descargue el instalador para Windows desde la página de versiones (releases) de HitStor y ejecútelo. El instalador coloca el binario `hitstor-agent.exe` en `C:\Program Files\HitStor\` y lo agrega al PATH del sistema.

### Configurar el agente { #configure-the-agent_1 }

Cree o edite el archivo de configuración en `C:\ProgramData\HitStor\config.yaml`:

```yaml
# C:\ProgramData\HitStor\config.yaml
agent_server: "backup.example.com:8420"
agent_api_key: "a3f8c1d9e4b7...6f2a0e5d"

# Opcional: certificado de la CA para la verificación TLS
# agent_ca_cert: "C:\\ProgramData\\HitStor\\ca.crt"
```

### Probar la conexión { #test-the-connection }

Abra una ventana de Símbolo del sistema o de PowerShell:

```powershell
hitstor-agent ping
```

### Ejecutar un respaldo manual { #run-a-manual-backup_1 }

```powershell
hitstor-agent backup C:\Users C:\Data
```

### Registrar como servicio de Windows { #register-as-a-windows-service }

Para ejecutar el agente en segundo plano como servicio de Windows:

```powershell
hitstor-agent service install
hitstor-agent service start
```

Esto registra `hitstor-agent daemon` como un servicio de Windows que se inicia automáticamente al arrancar el equipo.

Administre el servicio:

```powershell
hitstor-agent service stop
hitstor-agent service uninstall
```

También puede administrar el servicio desde la consola de Servicios de Windows (`services.msc`).

## Despliegue en múltiples equipos { #deploying-to-multiple-machines }

Para despliegues a gran escala, use herramientas de gestión de configuración (Ansible, Puppet, Chef o similares) para:

1. Instalar el paquete `hitstor-agent`
2. El agente genera automáticamente un token en su primer inicio
3. Recopilar los tokens de los agentes (con `hitstor-agent show-id`) y agregar los agentes desde el dashboard
4. Configurar las rutas de respaldo desde el explorador del sistema de archivos del dashboard

Como alternativa, para despliegues heredados:

1. Registre cada agente en el servidor con `hitstor serve agent add`
2. Despliegue `/etc/hitstor/agent.env` con la clave de API específica de cada equipo
3. Despliegue el archivo del certificado de la CA si usa certificados autofirmados
4. Habilite e inicie el servicio systemd `hitstor-agent`

## Próximos pasos { #next-steps }

- [Administración de agentes](agent-management.md): agregue agentes desde el dashboard o la CLI
- [Programación](scheduling.md): configure programaciones de respaldo para los agentes desplegados
- [Monitoreo](monitoring.md): configure alertas sobre el estado de los agentes
- [Solución de problemas](troubleshooting.md): resuelva problemas de conexión y de respaldo
