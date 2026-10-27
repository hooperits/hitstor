# Primeros pasos

Este tutorial lo guía por el flujo de trabajo principal de HitStor de principio a fin. Al terminar, habrá inicializado un repositorio, creado respaldos, listado instantáneas (snapshots), restaurado archivos, verificado la integridad y aplicado una política de retención.

## Inicializar un repositorio { #initialize-a-repository }

Un **repositorio** es el directorio donde HitStor almacena los datos de respaldo cifrados y deduplicados. Antes de crear cualquier respaldo, debe inicializar uno.

```console
hitstor init --repo /path/to/repo --password "secret"
created hitstor repository at /path/to/repo
```

El flag `--repo` indica el directorio de destino (se crea si no existe). El flag `--password` proporciona la frase de contraseña de cifrado que protege todos los datos del repositorio.

!!! warning "Advertencia"
    No olvide su contraseña. Sin ella, los datos del repositorio no pueden recuperarse. Considere guardar la contraseña en un gestor de contraseñas o usar `HITSTOR_PASSWORD_FILE` para leerla desde un archivo.

## Crear su primer respaldo { #create-your-first-backup }

Respalde un directorio pasando su ruta como último argumento:

```console
hitstor backup --repo /path/to/repo --password "secret" /data
snapshot abc12345 saved
  Files:        1,247 new,     0 changed,     0 unmodified
  Dirs:            38 new,     0 changed,     0 unmodified
  Data:       512.00 MiB added
  Processed:  1,247 files, 512.00 MiB in 0:08
```

HitStor divide cada archivo en fragmentos (chunks) de longitud variable, los deduplica, los comprime y cifra el resultado antes de escribirlo en el repositorio.

## Listar instantáneas { #list-snapshots }

Cada respaldo genera una **instantánea**: un registro de los archivos respaldados en un momento determinado. Liste todas las instantáneas con:

```console
hitstor snapshots --repo /path/to/repo --password "secret"
ID        Time                 Host        Tags    Paths
------------------------------------------------------------------------
abc12345  2026-02-22 10:00:05  myhost              /data
```

## Crear un segundo respaldo (comportamiento incremental) { #create-a-second-backup-incremental-behavior }

Ejecute de nuevo el mismo comando de respaldo después de hacer algunos cambios en `/data`:

```console
hitstor backup --repo /path/to/repo --password "secret" /data
snapshot def67890 saved
  Files:           3 new,     5 changed, 1,239 unmodified
  Dirs:            0 new,     2 changed,    36 unmodified
  Data:        4.20 MiB added
  Processed:  1,247 files, 512.00 MiB in 0:01
```

Observe cuánto más rápido es el segundo respaldo. HitStor solo almacena los fragmentos nuevos o modificados; los datos sin cambios se deduplican automáticamente.

```console
hitstor snapshots --repo /path/to/repo --password "secret"
ID        Time                 Host        Tags    Paths
------------------------------------------------------------------------
abc12345  2026-02-22 10:00:05  myhost              /data
def67890  2026-02-22 10:15:32  myhost              /data
```

## Restaurar desde una instantánea { #restore-from-a-snapshot }

Para restaurar el contenido de una instantánea en un directorio de destino, pase el ID de la instantánea y la ruta de destino:

```console
hitstor restore --repo /path/to/repo --password "secret" abc12345 /restore/path
restoring snapshot abc12345 to /restore/path
  Files:  1,247 restored
  Dirs:      38 restored
```

Puede usar el ID completo de la instantánea o un prefijo corto único (los primeros 8 caracteres o más). También puede usar la palabra clave `latest` para restaurar la instantánea más reciente:

```console
hitstor restore --repo /path/to/repo --password "secret" latest /restore/path
```

## Verificar el repositorio { #verify-the-repository }

El comando `check` verifica la integridad estructural del repositorio. Lee cada archivo de paquete, valida las sumas de verificación y confirma que todos los datos referenciados estén presentes.

```console
hitstor check --repo /path/to/repo --password "secret"
using temporary cache in /tmp/hitstor-check-123456
repository structure is ok
all pack files verified
no errors found
```

Ejecute `check` periódicamente, en especial antes y después de migrar los datos del repositorio, para detectar daños a tiempo.

## Aplicar una política de retención { #apply-a-retention-policy }

Con el tiempo, la cantidad de instantáneas crece. El comando `forget` olvida (forget), es decir, elimina, las instantáneas que quedan fuera de una política de retención, y el comando `prune` depura (prune) el repositorio para recuperar el espacio en disco ocupado por datos que ya no están referenciados.

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --keep-last 7 \
    --keep-daily 30
Applying retention policy:
  keep the last 7 snapshots
  keep daily snapshots for 30 days

remove 0 snapshots
```

En este ejemplo, ambas instantáneas son lo bastante recientes como para conservarse. A medida que crezca su historial de instantáneas, se eliminarán las instantáneas más antiguas que no cumplan ninguna regla de la política.

## Recuperar espacio en disco { #reclaim-disk-space }

Después de que `forget` marca instantáneas para eliminarlas, los fragmentos de datos subyacentes pueden seguir presentes. Ejecute `prune` para borrar los datos no referenciados y reempaquetar los archivos de paquete parcialmente usados:

```console
hitstor prune --repo /path/to/repo --password "secret"
counting files in repo
finding data still in use for 2 snapshots
[0:00] 100.00%  2 / 2 snapshots
searching used packs...
collecting packs for deletion and repacking
  no packs to delete or repack
done
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
