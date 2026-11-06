# hitstor-agent service

Administra el agente de HitStor como servicio del sistema: un servicio de Windows en Windows y la unidad de systemd en Linux.

## Sinopsis { #synopsis }

Windows:

```
hitstor-agent service install [--name HitStorAgent] [--display "HitStor Backup Agent"] [--config <archivo>]
hitstor-agent service start|stop|uninstall [--name HitStorAgent]
```

Linux:

```
hitstor-agent service install|uninstall|start|stop|restart|status|enable|disable [--name hitstor-agent]
```

## Descripción { #description }

En Windows, el comando registra y controla el servicio de Windows `HitStorAgent`. El servicio ejecuta el agente en [modo daemon](daemon.md): escucha en el puerto de escucha del agente las conexiones del servidor y consulta periódicamente al servidor sus tareas de respaldo.

En Linux, el comando controla mediante `systemctl` la unidad de systemd que instalan el paquete o el instalador del agente (`hitstor-agent.service`, que ejecuta `hitstor-agent daemon`). No crea el archivo de la unidad.

## Subcomandos { #subcommands }

### `service install` { #service-install }

Windows: registra el agente como servicio de Windows con inicio automático. Requiere privilegios de administrador. `--config`, `--server`, `--api-key` y `--ca-cert` indicados aquí se pasan al servicio.

Linux: ejecuta `systemctl enable --now` sobre la unidad.

### `service start` / `service stop` { #service-start-service-stop }

Inicia o detiene el servicio.

### `service uninstall` { #service-uninstall }

Windows: elimina el registro del servicio de Windows. Linux: detiene y deshabilita la unidad.

### `service restart`, `service status`, `service enable`, `service disable` (Linux) { #service-restart-service-status-service-enable-service-disable-linux }

Reinicia la unidad, muestra su estado o la habilita o deshabilita en el arranque.

## Flags { #flags }

| Flag | Predeterminado | Descripción |
|------|----------------|-------------|
| `--name` | `HitStorAgent` (Windows), `hitstor-agent` (Linux) | Nombre del servicio o de la unidad |
| `--display` | `HitStor Backup Agent` | Solo Windows, `install`: nombre visible en la consola de Servicios |

## Ejemplos { #examples }

Instalar e iniciar el servicio en Windows (consola con privilegios elevados):

```powershell
hitstor-agent service install --config "$env:ProgramData\HitStor\agent-config.yaml"
hitstor-agent service start
Get-Service HitStorAgent
```

Detener y desinstalar:

```powershell
hitstor-agent service stop
hitstor-agent service uninstall
```

En Linux:

```bash
sudo hitstor-agent service status
sudo hitstor-agent service restart
```

## Notas { #notes }

- El servicio de Windows se ejecuta como `LocalSystem`. Lee `%ProgramData%\HitStor\agent.env` y el archivo indicado con `--config` al instalarlo; el instalador de Windows indica `%ProgramData%\HitStor\agent-config.yaml`.
- El agente escribe su registro en `%ProgramData%\HitStor\logs\agent.log` en Windows y en `/var/log/hitstor/agent.log` en Linux (consulte [daemon](daemon.md)).
- El instalador de Windows instala e inicia el servicio por usted; consulte [Agente de Windows](../../installation/windows-agent.md).
