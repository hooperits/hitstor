# hitstor prune

Elimina del repositorio los datos que ya no están referenciados.

## Sinopsis { #synopsis }

```
hitstor prune [flags]
```

## Descripción { #description }

Elimina los archivos de paquete, las entradas de índice y los blobs que ya no están referenciados por ninguna instantánea (snapshot). Ejecute este comando después de `forget` para recuperar espacio de almacenamiento.

La depuración (prune) realiza una recolección de basura de tipo marcado y barrido (mark-and-sweep):

1. Explora todas las instantáneas para determinar qué blobs siguen referenciados.
2. Identifica los archivos de paquete que contienen solo blobs no referenciados (se eliminan por completo) o una mezcla de blobs referenciados y no referenciados (se reempaquetan).
3. Escribe nuevos archivos de paquete que contienen solo los blobs referenciados de los paquetes mixtos.
4. Elimina los archivos de paquete obsoletos y actualiza los índices.

Se adquiere un bloqueo exclusivo sobre el repositorio mientras dura la operación.

## Flags { #flags }

| Flag | Tipo | Predeterminado | Descripción |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Solo muestra lo que se eliminaría |
| `--max-repack-size` | string | | Limita el total de bytes reempaquetados (p. ej., `500MiB`, `2GiB`) |

Además de todos los [flags globales](index.md#global-flags), incluido `--repo-name` para configuraciones con varios repositorios.

### Sufijos de tamaño { #size-suffixes }

| Sufijo | Significado |
|--------|---------|
| `B` | Bytes |
| `KiB`, `MiB`, `GiB`, `TiB` | Unidades binarias (base 1024) |
| `KB`, `MB`, `GB`, `TB` | Unidades decimales (base 1000) |

Si no se indica ningún sufijo, el valor se interpreta en bytes.

## Ejemplos { #examples }

Depurar los datos no referenciados:

```bash
hitstor prune --repo /backups/hitstor --password "secret"
```

Ver una vista previa de lo que se eliminaría:

```bash
hitstor prune --repo /backups/hitstor --password "secret" --dry-run
```

Limitar el reempaquetado a 500 MiB:

```bash
hitstor prune --repo /backups/hitstor --password "secret" --max-repack-size 500MiB
```

## Notas { #notes }

- La depuración también está disponible como parte de `forget --prune`, que combina ambas operaciones.
- La operación informa la cantidad de paquetes eliminados, paquetes reempaquetados, bytes liberados e índices actualizados.
