# Comandos del servidor

Referencia de todos los comandos de la CLI `hitstor`.

## Comandos { #commands }

| Comando | Descripción |
|---------|-------------|
| [init](init.md) | Inicializa un nuevo repositorio de respaldos |
| [backup](backup.md) | Respalda archivos en el repositorio |
| [restore](restore.md) | Restaura archivos desde una instantánea (snapshot) |
| [snapshots](snapshots.md) | Lista todas las instantáneas (alias: `ls`) |
| [forget](forget.md) | Elimina instantáneas por ID o según una política de retención |
| [prune](prune.md) | Elimina del repositorio los datos sin referencias |
| [check](check.md) | Verifica la integridad del repositorio |
| [serve](serve.md) | Inicia el servidor de respaldos gRPC |
| [config](config.md) | Administra la configuración |
| [diff](diff.md) | Compara dos instantáneas |
| [du](du.md) | Muestra el uso de disco del repositorio |
| [repo](repo.md) | Operaciones de repositorio (compatible con varios repositorios) |
| [jobs](jobs.md) | Administra las tareas de respaldo programadas |
| [agents](agents.md) | Administra los agentes de respaldo registrados |
| [version](version.md) | Muestra la información de versión |

## Flags globales { #global-flags }

| Flag | Tipo | Predeterminado | Descripción |
|------|------|----------------|-------------|
| `--repo` | string | | Ubicación del repositorio (ruta o `s3:endpoint/bucket`); modo heredado de un solo repositorio |
| `--repo-name` | string | | Selecciona el repositorio por nombre en una configuración con varios repositorios (preferido) |
| `--config` | string | `~/.config/hitstor/config.yaml` | Ruta al archivo de configuración (en servidores suele ser `/etc/hitstor/config.yaml`) |
| `--password` | string | | Contraseña del repositorio |
| `--password-file` | string | | Ruta al archivo que contiene la contraseña del repositorio |
| `--parallel` | int | 0 | Número de workers en paralelo (0 = detección automática, máximo 8) |
| `--s3-access-key` | string | | Clave de acceso de S3 |
| `--s3-secret-key` | string | | Clave secreta de S3 |
| `--s3-region` | string | us-east-1 | Región de S3 |
| `--json` | bool | false | Salida en formato JSON (donde se admita) |
| `--no-color` | bool | false | Desactiva la salida con colores |
