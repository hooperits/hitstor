# `hitstor jobs`

Administra tareas de respaldo programadas (compatible con múltiples repositorios).

> **Nota:** La gestión completa de tareas (CRUD), su programación y ejecución se realizan principalmente desde el dashboard. Por ahora la CLI ofrece un soporte básico que se ampliará.

## Uso { #usage }

```bash
hitstor jobs [command]
```

## Subcomandos { #subcommands }

| Comando | Descripción |
|---------|-------------|
| `list` | Lista las tareas de respaldo configuradas |
| `create` | Define una nueva tarea de respaldo (por ahora imprime la definición; la persistencia completa llegará más adelante) |

## Flags de `create` { #flags-for-create }

- `--name` (obligatorio)
- `--repo-name` (obligatorio)
- `--job-type` (solo se admite `files`)
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

Para la gestión de tareas en producción, use las páginas Almacenamiento y Tareas del dashboard.
