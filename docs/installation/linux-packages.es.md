# Paquetes para Linux (deb/rpm)

Paquetes precompilados para Debian/Ubuntu (`.deb`) y RHEL/Fedora/AlmaLinux (`.rpm`) en arquitectura amd64.

!!! tip "Prefiera el instalador .run para una configuración más sencilla"
    La [instalación rápida para Linux (.run)](linux-installer.md) ofrece un asistente interactivo que configura por usted la contraseña del dashboard, los puertos, la dirección anunciada y el inicio del servicio. Los paquetes requieren configuración manual (vea más abajo).

## Descarga { #download }

Descargue los paquetes más recientes desde la página de [GitHub Releases](https://github.com/hooperits/hitstor/releases). Cada versión incluye cuatro paquetes para Linux:

| Paquete | Descripción |
|---------|-------------|
| `hitstor_x.y.z_amd64.deb` | Servidor -- Debian/Ubuntu |
| `hitstor-x.y.z-1.x86_64.rpm` | Servidor -- RHEL/Fedora/AlmaLinux |
| `hitstor-agent_x.y.z_amd64.deb` | Agente -- Debian/Ubuntu |
| `hitstor-agent-x.y.z-1.x86_64.rpm` | Agente -- RHEL/Fedora/AlmaLinux |

## Instalar el servidor { #install-the-server }

=== "Debian / Ubuntu"

    ```bash
    sudo dpkg -i hitstor_x.y.z_amd64.deb
    ```

=== "RHEL / Fedora"

    ```bash
    sudo rpm -i hitstor-x.y.z-1.x86_64.rpm
    ```

El paquete instala:

| Ruta | Descripción |
|------|-------------|
| `/usr/bin/hitstor` | Binario del servidor |
| `/usr/lib/systemd/system/hitstor-server.service` | Unidad de servicio de systemd |
| `/etc/hitstor/server.env.example` | Plantilla del archivo de entorno |
| `/etc/hitstor/config.yaml.example` | Plantilla del archivo de configuración |

## Instalar el agente { #install-the-agent }

=== "Debian / Ubuntu"

    ```bash
    sudo dpkg -i hitstor-agent_x.y.z_amd64.deb
    ```

=== "RHEL / Fedora"

    ```bash
    sudo rpm -i hitstor-agent-x.y.z-1.x86_64.rpm
    ```

El paquete del agente instala:

| Ruta | Descripción |
|------|-------------|
| `/usr/bin/hitstor-agent` | Binario del agente |
| `/usr/lib/systemd/system/hitstor-agent.service` | Unidad de servicio de systemd |
| `/usr/lib/systemd/system/hitstor-agent.timer` | Temporizador de systemd para respaldos programados |
| `/etc/hitstor/agent.env.example` | Plantilla del archivo de entorno |
| `/etc/hitstor/agent-config.yaml.example` | Plantilla del archivo de configuración |
| `/usr/lib/hitstor/plugins/hitstor-scanner-builtin` | Plugin de escáner de sistema de archivos integrado |
| `/var/lib/hitstor/plugins/` | Almacén de [plugins de carga de trabajo](../user-guide/plugins.md#workload-plugins) (modo 0700) |

## Configuración { #configuration }

Los scripts de posinstalación de los paquetes copian los ejemplos a los archivos de trabajo (`server.env`, `config.yaml`, `agent.env`, `agent-config.yaml`) cuando todavía no existen, y nunca los sobrescriben al actualizar. El paquete del servidor también crea el usuario del sistema `hitstor`.

### Servidor { #server }

Edite `/etc/hitstor/config.yaml` para configurar el servidor (puertos, dirección anunciada, certificados TLS, configuración del dashboard, etc.):

```bash
sudo editor /etc/hitstor/config.yaml
```

Normalmente los repositorios se crean desde la página **Almacenamiento** del dashboard después de iniciar el servicio. `/etc/hitstor/server.env` solo necesita `HITSTOR_REPO` y `HITSTOR_PASSWORD` para el modo heredado de un solo repositorio. El primer inicio de sesión funciona como se describe en [Contraseña del dashboard y primer administrador](quickstart.md#dashboard-password).

### Agente { #agent }

Para un agente administrado desde el dashboard (recomendado), deje los archivos copiados como están: inicie el servicio (más abajo), lea el token con `sudo hitstor-agent show-id` y agregue el agente desde el dashboard (**Agentes → Agregar agente**) con su dirección IP, su puerto de escucha (predeterminado: 8422) y el token.

Edite `/etc/hitstor/agent.env` solo para un agente registrado manualmente, para definir `HITSTOR_AGENT_SERVER` y `HITSTOR_AGENT_API_KEY` (la clave que muestra `hitstor serve agent add <nombre>`):

```bash
sudo editor /etc/hitstor/agent.env
```

## Iniciar los servicios { #start-the-services }

### Servidor { #server_1 }

Habilite e inicie el servidor de HitStor:

```bash
sudo systemctl enable --now hitstor-server
```

Verifique el estado del servicio:

```bash
sudo systemctl status hitstor-server
```

Vea los registros:

```bash
journalctl -u hitstor-server -f
```

### Agente { #agent_1 }

Habilite e inicie el agente de HitStor:

```bash
sudo systemctl enable --now hitstor-agent
```

Para habilitar el temporizador de respaldos programados en lugar de ejecutar el agente de forma continua:

```bash
sudo systemctl enable --now hitstor-agent.timer
```

## Verificar la instalación { #verify-the-installation }

```bash
hitstor version
```

Esto muestra la versión, el hash del commit y la fecha de compilación. Si no se encuentra el comando, asegúrese de que `/usr/bin` esté en su `PATH`.

## Desinstalar { #uninstall }

=== "Debian / Ubuntu"

    ```bash
    sudo systemctl disable --now hitstor-server
    sudo dpkg -r hitstor
    ```

=== "RHEL / Fedora"

    ```bash
    sudo systemctl disable --now hitstor-server
    sudo rpm -e hitstor
    ```

Los archivos de configuración en `/etc/hitstor/` se conservan durante la desinstalación. Elimínelos manualmente si ya no los necesita.
