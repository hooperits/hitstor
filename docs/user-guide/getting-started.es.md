# Primeros pasos

Este tutorial lo guía por el flujo de trabajo principal de HitStor de principio a fin. Al terminar, habrá inicializado un repositorio, creado respaldos, listado instantáneas (snapshots), restaurado archivos, verificado la integridad y aplicado una política de retención.

## Inicializar un repositorio { #initialize-a-repository }

Un **repositorio** es el directorio donde HitStor almacena los datos de respaldo cifrados y deduplicados. Antes de crear cualquier respaldo, debe inicializar uno.

```console
hitstor init --repo /path/to/repo --password "secret"
created new repository 3d3f89b7 at /path/to/repo
```

El flag `--repo` indica el directorio de destino (se crea si no existe). El flag `--password` proporciona la frase de contraseña de cifrado que protege todos los datos del repositorio.

!!! warning "Advertencia"
    No olvide su contraseña. Sin ella, los datos del repositorio no pueden recuperarse. Considere guardar la contraseña en un gestor de contraseñas o usar `--password-file` para leerla desde un archivo.

## Crear su primer respaldo { #create-your-first-backup }

Respalde un directorio pasando su ruta como último argumento:

```console
hitstor backup --repo /path/to/repo --password "secret" /data
scanning files...
scanned 1,247 files in 38 directories
✓ snapshot abc12345 saved
  Files:       1,247 new,        0 unchanged
  Dirs:           38 new
  Added:    498.2 MiB (512.0 MiB processed)
  Duration:      8.2s
```

HitStor divide cada archivo en fragmentos (chunks) de longitud variable, los deduplica, los comprime y cifra el resultado antes de escribirlo en el repositorio.

## Listar instantáneas { #list-snapshots }

Cada respaldo genera una **instantánea**: un registro de los archivos respaldados en un momento determinado. Liste todas las instantáneas con:

```console
hitstor snapshots --repo /path/to/repo --password "secret"
ID        Time                 Host    Tags  Files  Size
────────  ───────────────────  ──────  ────  ─────  ─────────
abc12345  2026-02-22 10:00:05  myhost        1,247  512.0 MiB
```

## Crear un segundo respaldo (comportamiento incremental) { #create-a-second-backup-incremental-behavior }

Ejecute de nuevo el mismo comando de respaldo después de hacer algunos cambios en `/data`:

```console
hitstor backup --repo /path/to/repo --password "secret" /data
scanning files...
scanned 1,247 files in 38 directories
✓ snapshot def67890 saved
  Files:           8 new,    1,239 unchanged
  Dirs:            2 new
  Added:     4.2 MiB (6.1 MiB processed)
  Duration:      1.1s
```

Observe cuánto más rápido es el segundo respaldo. HitStor solo almacena los fragmentos nuevos o modificados; los datos sin cambios se deduplican automáticamente.

```console
hitstor snapshots --repo /path/to/repo --password "secret"
ID        Time                 Host    Tags  Files  Size
────────  ───────────────────  ──────  ────  ─────  ─────────
def67890  2026-02-22 10:15:32  myhost        1,247  512.3 MiB
abc12345  2026-02-22 10:00:05  myhost        1,247  512.0 MiB
```

## Restaurar desde una instantánea { #restore-from-a-snapshot }

Para restaurar el contenido de una instantánea en un directorio de destino, pase el ID de la instantánea y la ruta de destino:

```console
hitstor restore --repo /path/to/repo --password "secret" abc12345 /restore/path
restoring snapshot abc12345 to /restore/path
✓ restored 1,247 files (512.0 MiB) in 6.3s
```

Puede usar el ID completo de la instantánea o cualquier prefijo único de él, como el ID de 8 caracteres que muestra `hitstor snapshots`. También puede usar la palabra clave `latest` para restaurar la instantánea más reciente:

```console
hitstor restore --repo /path/to/repo --password "secret" latest /restore/path
```

## Verificar el repositorio { #verify-the-repository }

El comando `check` verifica la integridad estructural del repositorio: que sus índices, archivos de paquete e instantáneas sean coherentes y que todos los datos referenciados estén presentes. Agregue `--read-data` para leer y descifrar además cada blob.

```console
hitstor check --repo /path/to/repo --password "secret"
checking indexes...
checking pack files...
checking snapshots...
no errors found
```

Ejecute `check` periódicamente, en especial antes y después de migrar los datos del repositorio, para detectar daños a tiempo.

## Aplicar una política de retención { #apply-a-retention-policy }

Con el tiempo, la cantidad de instantáneas crece. El comando `forget` olvida (forget), es decir, elimina, las instantáneas que quedan fuera de una política de retención, y el comando `prune` depura (prune) el repositorio para recuperar el espacio en disco ocupado por datos que ya no están referenciados.

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --keep-last 7 \
    --keep-daily 30
snapshots: 2 kept, 0 forgotten
```

En este ejemplo, ambas instantáneas son lo bastante recientes como para conservarse. A medida que crezca su historial de instantáneas, se eliminarán las instantáneas más antiguas que no cumplan ninguna regla de la política.

## Recuperar espacio en disco { #reclaim-disk-space }

Después de que `forget` marca instantáneas para eliminarlas, los fragmentos de datos subyacentes pueden seguir presentes. Ejecute `prune` para borrar los datos no referenciados y reempaquetar los archivos de paquete parcialmente usados:

```console
hitstor prune --repo /path/to/repo --password "secret"
collecting referenced blobs...
classifying packs...
nothing to prune
prune complete
Packs:      0 deleted,    0 repacked
Freed:   0 B (repacked 0 B)
Indexes:    0 deleted,    0 written
```

Cuando hay datos no referenciados que limpiar, `prune` informa exactamente cuánto espacio se recuperó.

## Próximos pasos { #next-steps }

Ahora que conoce el flujo de trabajo básico, puede explorar el resto de la Guía del usuario:

- [Respaldo](backup.md): opciones avanzadas de respaldo, patrones de exclusión y workers en paralelo.
- [Restauración](restore.md): restauraciones selectivas, permisos y verificación.
- [Instantáneas](snapshots.md): filtrar, comparar e inspeccionar instantáneas.
- [Retención](retention.md): estrategias de retención detalladas y depuración automatizada.
- [Configuración](configuration.md): archivo de configuración, variables de entorno y el asistente de configuración.
- [Plugins](plugins.md): amplíe HitStor con backends de almacenamiento, notificadores, hooks y escáneres.
