# Comandos del agente

Referencia de todos los comandos de la CLI `hitstor-agent`.

## Comandos { #commands }

| Comando | Descripción |
|---------|-------------|
| [backup](backup.md) | Envía un respaldo al servidor remoto |
| [ping](ping.md) | Prueba la conectividad con el servidor |
| [daemon](daemon.md) | Se ejecuta como daemon de larga duración con servidor de administración y consulta periódica de respaldos |
| [show-id](show-id.md) | Muestra el token del agente para registrarlo en el dashboard |
| [service](service.md) | Administra el servicio de Windows (solo Windows) |

## Flags globales { #global-flags }

| Flag | Tipo | Predeterminado | Descripción |
|------|------|---------|-------------|
| `--server` | string | | Dirección del servidor remoto (`host:port`) |
| `--api-key` | string | | Clave de API del agente para la autenticación |
| `--ca-cert` | string | | Ruta al certificado de la CA para la verificación TLS |
| `--hostname` | string | nombre de host del sistema | Reemplaza el nombre de host que se envía al servidor |
| `--config` | string | | Ruta al archivo de configuración |
