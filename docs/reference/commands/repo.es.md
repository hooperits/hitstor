# `hitstor repo`

Comandos de administración de repositorios (compatibles con varios repositorios).

## Uso { #usage }

```bash
hitstor repo [command]
```

## Subcomandos { #subcommands }

| Comando | Descripción |
|---------|-------------|
| `list` | Lista los repositorios configurados en la configuración de varios repositorios |
| `create <name> <path>` | Crea e inicializa un nuevo repositorio local |
| `delete <name>` | Quita un repositorio de la configuración (no elimina los datos) |
| `health` | Muestra el estado operativo del repositorio seleccionado |

## Flags globales (relevantes) { #global-flags-relevant }

- `--repo-name <name>`: selecciona un repositorio con nombre de la lista `repositories:` de la configuración.
- `--config /etc/hitstor/config.yaml`: recomendado al ejecutar en el servidor.

## Ejemplos { #examples }

```bash
# Listar todos los repositorios conocidos por este servidor
hitstor --config /etc/hitstor/config.yaml repo list

# Crear un nuevo repositorio local (crea el archivo de contraseña junto a la configuración)
hitstor --config /etc/hitstor/config.yaml repo create MyRepo /data/myrepo

# Verificación de estado de un repositorio con nombre
hitstor --config /etc/hitstor/config.yaml --repo-name MyRepo repo health
```

Consulte también: `hitstor config show` (muestra la sección moderna de repositorios cuando está presente).
