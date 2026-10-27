# hitstor-agent daemon

Ejecuta el agente como daemon de larga duración, con servidor de gestión y consulta periódica de respaldos.

## Sinopsis { #synopsis }

```
hitstor-agent daemon [flags]
```

## Descripción { #description }

Ejecuta el agente como un proceso de larga duración con dos funciones:

1. **Servidor de gestión**: escucha en un puerto gRPC (predeterminado: 8422) los comandos del servidor HitStor, incluidos el registro, la exploración del sistema de archivos y la configuración
2. **Consulta de respaldos**: consulta periódicamente al servidor la programación de respaldos y ejecuta los respaldos cuando corresponde

El daemon se inicia en uno de dos modos:

- **Sin configurar** (instalación nueva): solo ejecuta el servidor de gestión. Registra el mensaje "waiting for registration from dashboard." Cuando el servidor registra el agente desde el dashboard, el daemon pasa automáticamente al modo configurado.
- **Configurado** (después del registro o en una instalación heredada): ejecuta tanto el servidor de gestión como el ciclo de consulta de respaldos.

Se detiene de forma ordenada al recibir SIGINT/SIGTERM (Linux) o al detener el servicio (Windows).

## Flags { #flags }

| Flag | Tipo | Valor predeterminado | Descripción |
|------|------|---------|-------------|
| `--listen-port` | int | 8422 | Puerto gRPC de escucha para las conexiones de gestión del servidor |
| `--poll-interval` | duration | 5m | Frecuencia con la que se consulta al servidor si hay cambios en la programación |

## Ejemplos { #examples }

Ejecutar en primer plano (gestionado desde el dashboard):

```bash
hitstor-agent daemon --listen-port 8422
```

Ejecutar en primer plano (modo heredado):

```bash
hitstor-agent daemon --server backup.internal:8420 --api-key YOUR_KEY
```

Ejecutar mediante systemd:

```bash
sudo systemctl enable --now hitstor-agent
```

## Variables de entorno { #environment-variables }

| Variable | Descripción |
|----------|-------------|
| `HITSTOR_AGENT_LISTEN_PORT` | Sobrescribe el puerto de escucha predeterminado (8422) |

## Notas { #notes }

- En el primer inicio, el daemon genera un token de agente en `/etc/hitstor/agent-token` si no existe uno
- El servidor de gestión usa TLS autofirmado (generado automáticamente)
- Si no hay ninguna programación configurada, el daemon consulta cada 5 minutos y registra un mensaje
- Alternativa al modo daemon: use temporizadores de systemd con `hitstor-agent backup` para una programación de estilo cron
