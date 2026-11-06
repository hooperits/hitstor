# Comandos del agente

Referencia de todos los comandos de la CLI `hitstor-agent`.

## Comandos { #commands }

| Comando | Descripción |
|---------|-------------|
| [backup](backup.md) | Envía un respaldo al servidor remoto |
| [ping](ping.md) | Prueba la conectividad con el servidor |
| [daemon](daemon.md) | Se ejecuta como daemon de larga duración con servidor de administración y consulta periódica de respaldos |
| [show-id](show-id.md) | Muestra el token del agente para registrarlo en el dashboard |
| [plugin](plugin.md) | Instala y administra plugins de carga de trabajo |
| [service](service.md) | Administra el servicio del agente (servicio de Windows, o la unidad de systemd en Linux) |
| [bmr](bmr.md) | Crea e inspecciona medios de recuperación bare-metal (Linux) |
| [recover](recover.md) | Se ejecuta en modo de recuperación bare-metal (lo invoca el medio de rescate) |

## Flags globales { #global-flags }

| Flag | Tipo | Predeterminado | Descripción |
|------|------|---------|-------------|
| `--server` | string | | Dirección del servidor remoto (`host:port`) |
| `--api-key` | string | | Clave de API del agente para la autenticación |
| `--ca-cert` | string | | Ruta al certificado de la CA para la verificación TLS |
| `--config` | string | `~/.config/hitstor/config.yaml` | Ruta al archivo de configuración (el servicio del agente usa `/etc/hitstor/agent-config.yaml`, o `%ProgramData%\HitStor\agent-config.yaml` en Windows) |
| `--json` | bool | false | Salida en formato JSON |
| `-q`, `--quiet` | bool | false | Suprime la salida que no sea de error |
| `-v`, `--verbose` | count | | Aumenta el detalle de los registros (repita para más: `-vv`) |

El nombre de host que se envía al servidor se reemplaza con `backup --host` o con la clave de configuración `agent_hostname`.
