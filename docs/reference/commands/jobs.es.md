# `hitstor jobs`

Administra tareas de respaldo programadas (compatible con múltiples repositorios).

> **Nota:** La gestión completa de tareas (edición, tareas de carga de trabajo con planes de respaldo, ejecución y cancelación) se realiza en el dashboard. La CLI permite listar, crear tareas de archivos y reparar tareas huérfanas.

## Uso { #usage }

```bash
hitstor jobs [command]
```

## Subcomandos { #subcommands }

| Comando | Descripción |
|---------|-------------|
| `list` | Lista las tareas de respaldo configuradas (`--agent <nombre>` filtra por agente; `--orphans` muestra solo las tareas huérfanas) |
| `create` | Crea una tarea de respaldo de archivos en el registro de agentes del repositorio indicado con `--repo-name` |
| `orphans` | Lista las tareas cuyo agente ya no está registrado |
| `reattach` | Vuelve a asociar tareas a un agente: `--agent <nombre>` (obligatorio) con `--all-orphans` o `--job-id <id>` (repetible) |

## Flags de `create` { #flags-for-create }

- `--name` (obligatorio)
- `--repo-name` (obligatorio)
- `--job-type` (solo se admite `files`; las tareas de carga de trabajo se crean en el dashboard)
- `--path` (repetible)
- `--interval-hours`, `--cron`, `--timezone`, `--agent-name`

## Ejemplos { #examples }

```bash
hitstor --config /etc/hitstor/config.yaml jobs list

hitstor --config /etc/hitstor/config.yaml jobs create \
  --name "Daily /home" \
  --repo-name Production \
  --job-type files \
  --path /home \
  --interval-hours 24
```

`create` guarda la tarea en el registro de agentes del repositorio `--repo-name`, que también es donde van los respaldos de la tarea. Un servidor en ejecución lee las tareas del registro de su primer repositorio (por nombre), así que cree desde el dashboard las tareas que respaldan en otro repositorio. Para la gestión de tareas en producción, use la página **Respaldos** del dashboard.
