# Instalación rápida en Linux

La forma más rápida de instalar HitStor en un servidor Linux. Descargue un único archivo `.run`, ejecútelo y el asistente interactivo lo configura todo: sin gestor de paquetes, sin herramientas de compilación y sin dependencias.

!!! info "Requisitos"
    - Linux (amd64 o arm64)
    - systemd
    - Privilegios de root (`sudo`)
    - Probado en Ubuntu 22.04+, Debian 12+, RHEL 9+, Fedora 39+, AlmaLinux 9+

## Instalar el servidor { #install-the-server }

Descargue el instalador desde la página de [GitHub Releases](https://github.com/hooperits/hitstor/releases) y ejecútelo:

```bash
wget https://github.com/hooperits/hitstor/releases/download/vX.Y.Z/hitstor-server-X.Y.Z-linux-amd64.run
chmod +x hitstor-server-X.Y.Z-linux-amd64.run
sudo ./hitstor-server-X.Y.Z-linux-amd64.run
```

El asistente interactivo le solicitará:

1. **Directorio de datos**: dónde HitStor almacena los datos (predeterminado: `/var/lib/hitstor`)
2. **Contraseña del dashboard**: contraseña de inicio de sesión de la interfaz web (opcional; si se deja vacía, el servidor genera una en el primer inicio; consulte [Contraseña del dashboard](quickstart.md#dashboard-password))
3. **Puerto gRPC**: para las conexiones de los agentes (predeterminado: 8420)
4. **Puerto del dashboard**: para la interfaz web (predeterminado: 8421)

Tras confirmar, el instalador crea el usuario del sistema, inicia el servicio y verifica que todo esté en ejecución. Los repositorios se crean desde el dashboard después de la instalación.

### Instalación no interactiva { #non-interactive-install }

Para automatización, use `--no-wizard` con variables de entorno:

```bash
sudo HITSTOR_DATA_DIR=/var/lib/hitstor \
     HITSTOR_DASHBOARD_PASSWORD=dashpass \
     HITSTOR_GRPC_PORT=8420 \
     HITSTOR_DASHBOARD_PORT=8421 \
     ./hitstor-server-X.Y.Z-linux-amd64.run --no-wizard
```

### Archivos instalados { #installed-files }

| Ruta | Descripción |
|------|-------------|
| `/usr/bin/hitstor` | Binario del servidor |
| `/usr/lib/systemd/system/hitstor-server.service` | Unidad de servicio de systemd |
| `/etc/hitstor/config.yaml` | Configuración del servidor |
| `/etc/hitstor/server.env` | Variables de entorno |
| `/var/lib/hitstor/` | Directorio de datos (ruta predeterminada) |
| `/var/log/hitstor/` | Directorio de registros |

## Instalar el agente { #install-the-agent }

Descargue el instalador del agente y ejecútelo:

```bash
wget https://github.com/hooperits/hitstor/releases/download/vX.Y.Z/hitstor-agent-X.Y.Z-linux-amd64.run
chmod +x hitstor-agent-X.Y.Z-linux-amd64.run
sudo ./hitstor-agent-X.Y.Z-linux-amd64.run
```

El asistente ofrece dos modos de configuración:

1. **Administrado desde el dashboard (recomendado)**: el agente genera un token y espera a que se registre desde el dashboard del servidor. Solicita el puerto de escucha (predeterminado: 8422) y, opcionalmente, las rutas de respaldo.
2. **Heredado (manual)**: indique la dirección del servidor y la clave de API obtenida con `hitstor serve agent add <name>`. Incluye una prueba de conexión.

Después de la instalación, los agentes administrados desde el dashboard muestran el **token del agente** y las instrucciones para agregarlo desde el dashboard.

### Instalación no interactiva { #non-interactive-install_1 }

**Modo administrado desde el dashboard** (predeterminado):

```bash
sudo HITSTOR_AGENT_LISTEN_PORT=8422 \
     ./hitstor-agent-X.Y.Z-linux-amd64.run --no-wizard
```

**Modo heredado** (se activa al definir el servidor y la clave de API):

```bash
sudo HITSTOR_AGENT_SERVER=backup-server.internal:8420 \
     HITSTOR_AGENT_API_KEY=your-64-char-hex-key \
     HITSTOR_AGENT_PATHS=/etc,/home \
     ./hitstor-agent-X.Y.Z-linux-amd64.run --no-wizard
```

### Archivos instalados { #installed-files_1 }

| Ruta | Descripción |
|------|-------------|
| `/usr/bin/hitstor-agent` | Binario del agente |
| `/usr/lib/systemd/system/hitstor-agent.service` | Unidad de servicio de systemd |
| `/usr/lib/systemd/system/hitstor-agent.timer` | Temporizador de respaldo diario |
| `/etc/hitstor/agent-config.yaml` | Configuración del agente |
| `/etc/hitstor/agent.env` | Variables de entorno (puerto de escucha, dirección del servidor) |
| `/etc/hitstor/agent-token` | Token del agente para el registro en el dashboard (modo 0600) |

## Actualización { #upgrading }

Al ejecutar el instalador sobre una instalación existente, se detecta automáticamente la actualización:

```bash
sudo ./hitstor-server-X.Y.Z-linux-amd64.run
```

El proceso de actualización:

1. Muestra la versión actual y la nueva para que las confirme
2. Respalda el binario actual en `/usr/bin/hitstor.rollback`
3. Detiene el servicio (si está en ejecución)
4. Instala el nuevo binario y las unidades de systemd
5. Actualiza los archivos de configuración de ejemplo (sus configuraciones en uso **nunca** se modifican)
6. Reinicia el servicio y verifica que esté en buen estado
7. Si la verificación falla: **revierte automáticamente** a la versión anterior

Use `--no-wizard` para omitir la confirmación (para automatización).

## Desinstalación { #uninstall }

El instalador v2 incluye un desinstalador integrado:

### Servidor { #server }

```bash
sudo ./hitstor-server-X.Y.Z-linux-amd64.run --uninstall
```

Esto elimina el binario, el servicio de systemd, la configuración, los registros y el usuario del sistema. Se le preguntará por separado si desea eliminar el directorio de datos (`/var/lib/hitstor`); la respuesta predeterminada es **no**, para proteger sus respaldos.

### Agente { #agent }

```bash
sudo ./hitstor-agent-X.Y.Z-linux-amd64.run --uninstall
```

Esto elimina el binario, el servicio y el temporizador de systemd, y la configuración.

## Comprobación de versión { #version-check }

HitStor puede buscar versiones más recientes mediante la API de GitHub Releases.

### CLI { #cli }

```bash
hitstor version --check
```

### Automática (servidor) { #automatic-server }

Cuando `update_check: true` está definido en `/etc/hitstor/config.yaml` (valor predeterminado), el servidor busca actualizaciones cada 24 horas. Si hay una versión más reciente, aparece un banner en la parte superior del dashboard web.

## Soporte para ARM64 { #arm64-support }

También hay instaladores para ARM64 (aarch64). Descargue la variante `arm64`:

```bash
sudo ./hitstor-server-X.Y.Z-linux-arm64.run
sudo ./hitstor-agent-X.Y.Z-linux-arm64.run
```

## Flags del instalador { #installer-flags }

| Flag | Descripción |
|------|-------------|
| `--no-wizard` | Modo no interactivo (lee variables de entorno) |
| `--uninstall` | Elimina HitStor y todos los archivos asociados |
| `--yes` | Omite las confirmaciones |
| `--version` | Imprime la versión del instalador y termina |
| `--help` | Muestra la ayuda |

## Solución de problemas { #troubleshooting }

### Permiso denegado { #permission-denied }

El instalador debe ejecutarse como root:

```bash
sudo ./hitstor-server-X.Y.Z-linux-amd64.run
```

### No se encuentra systemd { #systemd-not-found }

El instalador `.run` requiere systemd. Si su sistema no tiene systemd, use el método [Compilar desde el código fuente](linux-source.md) y administre el proceso manualmente.

### Status 226/NAMESPACE { #status-226namespace }

En algunos entornos virtualizados (VM de Hyper-V, contenedores, WSL), el endurecimiento de seguridad de systemd puede hacer que el servicio falle con el código de salida 226. Para corregirlo, edite la unidad de servicio y desactive las directivas de endurecimiento:

```bash
sudo systemctl edit hitstor-server
```

Agregue las siguientes sobrescrituras:

```ini
[Service]
ProtectSystem=
ProtectHome=
ProtectKernelTunables=
ProtectKernelModules=
ProtectControlGroups=
PrivateTmp=
NoNewPrivileges=
```

Luego reinicie:

```bash
sudo systemctl restart hitstor-server
```

Consulte [Solución de problemas](../admin-guide/troubleshooting.md) para más detalles.
