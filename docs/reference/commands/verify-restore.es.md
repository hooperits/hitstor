# hitstor verify-restore

Realiza una restauración de prueba de una instantánea (snapshot) y vuelve a calcular el hash de los archivos (verificación de restauración).

## Sinopsis { #synopsis }

```
hitstor verify-restore <snapshot-id> [flags]
```

## Descripción { #description }

Restaura una instantánea (o una muestra de sus archivos) en un directorio temporal, vuelve a fragmentar cada archivo restaurado con el chunker del repositorio y compara los IDs de blob del contenido con el árbol de la instantánea.

Esta es la principal señal de confianza de que un respaldo de sistema de archivos realmente puede restaurarse: hace pasar datos reales por la restauración y la refragmentación, en lugar de solo revisar los metadatos.

Use `latest` como ID de instantánea para verificar la instantánea más reciente. Está disponible el autocompletado con Tab para los IDs de instantánea.

## Flags { #flags }

| Flag | Tipo | Predeterminado | Descripción |
|------|------|----------------|-------------|
| `--max-files` | int | 0 (todos) | Limita la cantidad de archivos verificados |
| `--max-bytes` | uint64 | 0 (sin límite) | Limita el total de bytes verificados |
| `--keep-temp` | bool | false | Conserva el directorio temporal de restauración en lugar de eliminarlo |

Además de todos los [flags globales](index.md#global-flags), incluido `--repo-name` para configuraciones con varios repositorios.

## Ejemplos { #examples }

Verificar por completo la instantánea más reciente:

```bash
hitstor verify-restore latest --repo /backups/hitstor --password "secret"
```

Verificar una muestra de una instantánea grande, con un límite de 500 archivos:

```bash
hitstor verify-restore a1b2c3d4 --max-files 500
```

Verificar y conservar la restauración temporal para inspeccionarla manualmente:

```bash
hitstor verify-restore latest --keep-temp
```

## Notas { #notes }

- La entrada de cada tarea de respaldo en el dashboard también puede ejecutar esto automáticamente después de cada respaldo; consulte [Recuperación ante desastres guiada](../../user-guide/disaster-recovery.md).
- Un código de salida distinto de cero significa que se encontró una discrepancia o que la propia verificación falló; las discrepancias se imprimen en stderr con la ruta afectada y el motivo.
- Con `--json`, la salida es un único objeto JSON (`ok`, `snapshot_id`, `files_checked`, `files_sampled`, `bytes_checked`, `symlinks_ok`, `sampled`, `duration`, `mismatches`, `error`, `restore_target`) en lugar del resumen legible.
