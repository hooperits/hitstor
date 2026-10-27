# hitstor snapshots

Lista todas las instantáneas del repositorio.

## Sinopsis { #synopsis }

```
hitstor snapshots [flags]
```

## Descripción { #description }

Lista todas las instantáneas (snapshots) almacenadas en el repositorio, ordenadas por fecha (las más recientes primero). De forma predeterminada, la salida es una tabla con el ID corto, la marca de tiempo, el nombre de host, las rutas, la cantidad de archivos y el tamaño total de cada instantánea.

Use `--json` para obtener una salida JSON legible por máquinas.

Alias: `hitstor ls`

## Flags { #flags }

| Flag | Tipo | Predeterminado | Descripción |
|------|------|----------------|-------------|
| `--host` | string | | Muestra solo las instantáneas de este nombre de host |
| `--path` | string | | Muestra solo las instantáneas que incluyen esta ruta respaldada |

Además de todos los [flags globales](index.md#global-flags), incluido `--repo-name` para apuntar a un repositorio específico en configuraciones con varios repositorios.

## Ejemplos { #examples }

Listar todas las instantáneas:

```bash
hitstor snapshots --repo /backups/hitstor --password "secret"
```

Filtrar por nombre de host:

```bash
hitstor snapshots --repo /backups/hitstor --password "secret" --host webserver-01
```

Salida JSON:

```bash
hitstor snapshots --repo /backups/hitstor --password "secret" --json
```

## Formato de salida { #output-format }

### Tabla (predeterminado) { #table-default }

```
ID         Time                 Host         Paths                   Files       Size
a1b2c3d4   2025-01-15 10:30:00  myhost       /home/user/documents      150     45 MiB
b5c6d7e8   2025-01-14 10:30:00  myhost       /home/user/documents      148     44 MiB
```

### JSON (`--json`) { #json-json }

```json
[
  {
    "id": "a1b2c3d4",
    "time": "2025-01-15T10:30:00Z",
    "hostname": "myhost",
    "paths": ["/home/user/documents"],
    "total_files": 150,
    "total_size": 47185920
  }
]
```
