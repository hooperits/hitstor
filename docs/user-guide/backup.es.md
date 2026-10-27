# Respaldo

El comando `hitstor backup` crea una nueva instantánea (snapshot) de uno o más directorios. Cada instantánea captura el estado completo de las rutas especificadas en el momento en que se ejecuta el respaldo, pero gracias a la fragmentación definida por contenido y a la deduplicación, solo los datos nuevos o modificados se escriben realmente en el repositorio.

## Uso básico { #basic-usage }

```console
hitstor backup --repo /path/to/repo --password "secret" /path/to/backup
```

Esto crea una instantánea que contiene todos los archivos que hay en `/path/to/backup`.

## Respaldar varias rutas { #backing-up-multiple-paths }

Puede especificar más de una ruta de origen en un mismo respaldo:

```console
hitstor backup --repo /path/to/repo --password "secret" \
    /home/user/documents \
    /home/user/photos \
    /etc
```

Todas las rutas indicadas se incluyen en la misma instantánea.

## Patrones de exclusión { #exclude-patterns }

### Exclusiones en la línea de comandos { #command-line-excludes }

Use `--exclude` para omitir los archivos o directorios que coincidan con un patrón:

```console
hitstor backup --repo /path/to/repo --password "secret" \
    --exclude "*.tmp" \
    --exclude "node_modules" \
    /home/user/projects
```

Los patrones siguen la sintaxis glob estándar:

| Patrón | Coincide con |
|---------|---------|
| `*.log` | Todos los archivos que terminan en `.log` |
| `cache/` | Cualquier directorio llamado `cache` |
| `/data/temp` | La ruta exacta `/data/temp` relativa a la raíz del respaldo |
| `**/build` | Un directorio llamado `build` a cualquier profundidad |

### Exclusiones en el archivo de configuración { #config-file-excludes }

También puede definir exclusiones en el archivo de configuración (`~/.config/hitstor/config.yaml`):

```yaml
backup:
  exclude:
    - "*.tmp"
    - ".cache"
    - "node_modules"
```

### Comportamiento de combinación { #merging-behavior }

Las exclusiones de ambos orígenes se combinan en tiempo de ejecución. Un archivo se excluye si coincide con **cualquier** patrón, ya sea de la línea de comandos o del archivo de configuración. Los patrones de la línea de comandos no reemplazan los del archivo de configuración, sino que se suman a ellos.

## Etiquetas { #tags }

Las etiquetas le permiten identificar las instantáneas para filtrarlas más fácilmente después. Use `--tag` (se puede repetir) para asignar una o más etiquetas:

```console
hitstor backup --repo /path/to/repo --password "secret" \
    --tag project-alpha \
    --tag weekly \
    /data/project-alpha
```

Las etiquetas aparecen en la salida de `hitstor snapshots` y se pueden usar con `--keep-tag` en las políticas de retención. Consulte [Retención](retention.md) para obtener más detalles.

## Sobrescribir el host { #host-override }

De forma predeterminada, HitStor registra el nombre de host del sistema en cada instantánea. Si respalda los mismos datos desde varios equipos, o si su nombre de host cambia, puede sobrescribirlo:

```console
hitstor backup --repo /path/to/repo --password "secret" \
    --host webserver-01 \
    /var/www
```

`forget` usa el valor del host al aplicar políticas de retención por host.

## Modo de simulación { #dry-run-mode }

Para ver de antemano lo que haría un respaldo sin escribir ningún dato, pase `--dry-run`:

```console
hitstor backup --repo /path/to/repo --password "secret" \
    --dry-run \
    /data
```

El comando analiza las rutas de origen e informa la cantidad de archivos y el tamaño estimado de los datos, pero no crea ninguna instantánea ni escribe ningún paquete.

## Workers en paralelo { #parallel-workers }

HitStor procesa los archivos de forma concurrente. El flag `--parallel` controla el número de goroutines de trabajo (workers):

```console
hitstor backup --repo /path/to/repo --password "secret" \
    --parallel 4 \
    /data
```

De forma predeterminada, se detecta automáticamente el número de CPU, con un máximo de **8** workers. En cargas de trabajo limitadas por E/S (almacenamiento en red, discos mecánicos) puede obtener mejor rendimiento con menos workers; en SSD rápidos puede probar a aumentar la cantidad.

| Situación | `--parallel` recomendado |
|-----------|--------------------------|
| SSD NVMe, muchos archivos pequeños | 6--8 |
| Disco mecánico | 1--2 |
| Montaje de red (NFS, SMB) | 2--4 |
| Backend S3 | 4--8 |

## Indicador de progreso { #progress-display }

Mientras se ejecuta el respaldo, HitStor muestra una línea de progreso en vivo con:

- Número de archivos analizados
- Cantidad de datos procesados
- Cantidad de datos nuevos agregados al repositorio
- Tiempo transcurrido y tiempo restante estimado

El indicador de progreso se actualiza en el mismo lugar en las terminales que admiten retornos de carro. Cuando la salida se redirige a un archivo o a una tubería (pipe), HitStor desactiva las actualizaciones en vivo y, en su lugar, muestra un resumen final.

## Cómo funciona la deduplicación { #how-deduplication-works }

HitStor usa una estrategia de deduplicación de tres capas para minimizar el consumo de almacenamiento:

### 1. Fragmentación definida por contenido (FastCDC) { #1-content-defined-chunking-fastcdc }

Los archivos se dividen en fragmentos (chunks) de longitud variable mediante el algoritmo FastCDC. A diferencia de la división en bloques de tamaño fijo, FastCDC tolera las inserciones y eliminaciones: si agrega un párrafo al principio de un archivo grande, solo cambian los primeros fragmentos y el resto permanece idéntico.

### 2. Deduplicación a nivel de fragmento { #2-chunk-level-deduplication }

Cada fragmento se identifica por su hash criptográfico. Antes de escribir un fragmento en el repositorio, HitStor comprueba si ya existe un fragmento con el mismo hash. Si existe, simplemente se hace referencia al fragmento en lugar de volver a almacenarlo.

### 3. Agrupación a nivel de paquete { #3-pack-level-aggregation }

Los fragmentos individuales se agrupan en **archivos de paquete** para reducir el número de objetos del repositorio y mejorar la eficiencia de E/S. Cada archivo de paquete contiene muchos fragmentos y va acompañado de un índice que asocia los hashes de los fragmentos con sus posiciones dentro del paquete.

En conjunto, estas capas hacen que:

- Los archivos idénticos en distintas instantáneas se almacenen una sola vez.
- Los archivos que difieren poco compartan la mayoría de sus fragmentos.
- Los respaldos de equipos similares en el mismo repositorio se beneficien de la deduplicación entre equipos.

## Uso del backend S3 { #s3-backend-usage }

HitStor admite Amazon S3 (y servicios compatibles con S3) como backend. Para respaldar en un bucket de S3:

```console
hitstor init --repo s3:s3.amazonaws.com/my-bucket/hitstor-repo --password "secret"
```

Luego ejecute los respaldos con el mismo URI de repositorio:

```console
hitstor backup --repo s3:s3.amazonaws.com/my-bucket/hitstor-repo \
    --password "secret" \
    /data
```

Configure las credenciales mediante las variables de entorno estándar de AWS o el archivo de credenciales compartido:

| Variable | Propósito |
|----------|---------|
| `AWS_ACCESS_KEY_ID` | Clave de acceso |
| `AWS_SECRET_ACCESS_KEY` | Clave secreta |
| `AWS_DEFAULT_REGION` | Región del bucket |

Para servicios compatibles con S3 (MinIO, Backblaze B2, Wasabi), apunte el URI al endpoint correspondiente:

```console
hitstor init --repo s3:https://s3.us-west-001.backblazeb2.com/my-bucket/hitstor-repo \
    --password "secret"
```

!!! tip "Consejo"
    Guarde la ruta del repositorio y las credenciales en el archivo de configuración o en variables de entorno para no tener que escribirlas en cada comando. Consulte [Configuración](configuration.md).
