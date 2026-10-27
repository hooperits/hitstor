# Paquetes para Linux (deb/rpm)

Paquetes precompilados para Debian/Ubuntu (`.deb`) y RHEL/Fedora/AlmaLinux (`.rpm`) en arquitectura amd64.

!!! tip "Prefiera el instalador .run para una configuración más sencilla"
    La [instalación rápida para Linux (.run)](linux-installer.md) ofrece un asistente interactivo que configura todo automáticamente: contraseñas, puertos, inicialización del repositorio e inicio del servicio. Los paquetes requieren configuración manual (vea más abajo).

## Descarga { #download }

Descargue los paquetes más recientes desde la página de [GitHub Releases](https://github.com/hooperits/hitstor/releases). Cada versión incluye cuatro paquetes para Linux:

| Paquete | Descripción |
|---------|-------------|
| `hitstor_x.y.z_amd64.deb` | Servidor -- Debian/Ubuntu |
| `hitstor-x.y.z.x86_64.rpm` | Servidor -- RHEL/Fedora/AlmaLinux |
| `hitstor-agent_x.y.z_amd64.deb` | Agente -- Debian/Ubuntu |
| `hitstor-agent-x.y.z.x86_64.rpm` | Agente -- RHEL/Fedora/AlmaLinux |

## Instalar el servidor { #install-the-server }

=== "Debian / Ubuntu"

    ```bash
    sudo dpkg -i hitstor_x.y.z_amd64.deb
    ```

=== "RHEL / Fedora"

    ```bash
    sudo rpm -i hitstor-x.y.z.x86_64.rpm
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
    sudo rpm -i hitstor-agent-x.y.z.x86_64.rpm
    ```

El paquete del agente instala:

| Ruta | Descripción |
|------|-------------|
| `/usr/bin/hitstor-agent` | Binario del agente |
| `/usr/lib/systemd/system/hitstor-agent.service` | Unidad de servicio de systemd |
| `/usr/lib/systemd/system/hitstor-agent.timer` | Temporizador de systemd para respaldos programados |
| `/etc/hitstor/agent.env.example` | Plantilla del archivo de entorno |
| `/etc/hitstor/agent-config.yaml.example` | Plantilla del archivo de configuración |

## Configuración { #configuration }

Después de la instalación, cree sus archivos de configuración a partir de los ejemplos incluidos.

### Servidor { #server }

```bash
sudo cp /etc/hitstor/server.env.example /etc/hitstor/server.env
sudo cp /etc/hitstor/config.yaml.example /etc/hitstor/config.yaml
```

Edite `/etc/hitstor/server.env` para definir la ruta y la contraseña del repositorio:

```bash
sudo editor /etc/hitstor/server.env
```

Edite `/etc/hitstor/config.yaml` para configurar el servidor (dirección de escucha, certificados TLS, configuración del dashboard, etc.):

```bash
sudo editor /etc/hitstor/config.yaml
```

### Agente { #agent }

```bash
sudo cp /etc/hitstor/agent.env.example /etc/hitstor/agent.env
sudo cp /etc/hitstor/agent-config.yaml.example /etc/hitstor/agent-config.yaml
```

Edite `/etc/hitstor/agent.env` para definir la dirección del servidor y la clave de API:

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
