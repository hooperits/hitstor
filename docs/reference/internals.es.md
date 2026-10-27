# Funcionamiento interno

Este documento describe el diseño interno de HitStor: cifrado, fragmentación, deduplicación, formato de los paquetes y estructura del repositorio.

## Cifrado { #encryption }

Todos los datos almacenados en el repositorio se cifran con **AES-256-GCM** (Galois/Counter Mode), que proporciona tanto confidencialidad como autenticación. El formato del texto cifrado es:

```
nonce (12 bytes) || ciphertext || authentication tag (16 bytes)
```

Se genera un nonce aleatorio de 12 bytes para cada operación de cifrado.

### Derivación de claves { #key-derivation }

HitStor usa **scrypt** para derivar una clave de 64 bytes a partir de la contraseña del usuario:

| Parámetro | Valor |
|---|---|
| N (costo de CPU/memoria) | 32768 |
| r (tamaño de bloque) | 8 |
| p (paralelismo) | 1 |
| Sal (salt) | 64 bytes aleatorios |
| Salida | 64 bytes |

Los primeros 32 bytes de la clave derivada se usan como clave de cifrado AES-256-GCM del archivo de clave. Los 32 bytes restantes no se usan directamente (la clave maestra proporciona las claves reales de cifrado y de identificación).

### Clave maestra { #master-key }

Cada repositorio tiene una **clave maestra** compuesta por dos claves de 32 bytes:

- **EncryptKey**: se usa para el cifrado AES-256-GCM de todos los datos del repositorio (paquetes, índices, instantáneas)
- **IDKey**: se usa para la identificación de blobs con HMAC-SHA256

La clave maestra se genera a partir de 64 bytes criptográficamente aleatorios y se almacena en el directorio `keys/`, cifrada con la clave derivada de la contraseña.

### ID de blob { #blob-ids }

Los ID de blob se calculan como **HMAC-SHA256(IDKey, data)**, lo que produce un identificador de 32 bytes. Este esquema es direccionable por contenido (el mismo contenido siempre produce el mismo ID), pero no se puede adivinar sin la IDKey, lo que brinda privacidad incluso si un atacante puede observar los ID de blob.

## Fragmentación { #chunking }

HitStor usa **FastCDC** (Fast Content-Defined Chunking) con un hash rodante Gear para dividir los datos de los archivos en fragmentos (chunks) de tamaño variable cuyos límites se determinan según el contenido.

| Parámetro | Valor |
|---|---|
| Tamaño mínimo de fragmento | 256 KiB |
| Tamaño promedio de fragmento | 1 MiB |
| Tamaño máximo objetivo de fragmento | 4 MiB |

El algoritmo usa una detección de límites en dos fases:

1. **Fase 1** (posiciones del mínimo al promedio): usa una máscara más permisiva (más bits activos), lo que facilita encontrar un límite. Esto inclina los fragmentos hacia el tamaño promedio.
2. **Fase 2** (posiciones del promedio al máximo): usa una máscara más estricta (menos bits activos), lo que permite que los fragmentos crezcan más allá del promedio cuando no se encontró un límite antes.

Si no se encuentra ningún límite antes del tamaño máximo, el fragmento se corta en el máximo.

La fragmentación definida por contenido implica que las inserciones o eliminaciones en un archivo solo afectan a los límites de fragmento cercanos, lo que maximiza la deduplicación entre respaldos.

## Deduplicación { #deduplication }

La deduplicación ocurre a nivel de fragmento. Antes de escribir un nuevo blob en el repositorio, HitStor verifica:

1. **Conjunto de la sesión**: los blobs ya agregados durante el respaldo en curso
2. **Índice del repositorio**: el índice completo de todos los blobs almacenados en el repositorio

Para los **respaldos locales con la CLI** (`hitstor backup`), se aplica una capa adicional:

3. **Caché de archivos**: base de datos SQLite que registra el inode, el tamaño, el mtime y los ID de fragmento de cada archivo

Para los **respaldos de agentes remotos**, los archivos sin cambios se detectan mediante los **metadatos de la instantánea anterior** que se devuelven en `BeginSession` (`previous_files` con mtime, tamaño e ID de contenido). No se requiere una caché SQLite del lado del agente.

## Compresión { #compression }

Cada fragmento se comprime con **Zstd** (Zstandard) antes del cifrado. La compresión se aplica por fragmento, no por archivo, lo que permite descomprimir cada fragmento de forma independiente durante la restauración.

## Archivos de paquete { #pack-files }

Los blobs no se almacenan de forma individual. En su lugar, varios blobs se agrupan en **archivos de paquete** para reducir la cantidad de objetos en el backend y mejorar la eficiencia de E/S.

| Parámetro | Valor |
|---|---|
| Umbral de tamaño del paquete | 128 MiB |

Cuando los datos de blob acumulados en el paquete actual superan 128 MiB, el paquete se finaliza y se escribe en el backend. Cada paquete contiene:

- Los datos de blob cifrados, concatenados
- Un encabezado cifrado al final del archivo con los desplazamientos, tamaños, tipos e ID de los blobs

Hay dos tipos de blob almacenados en los paquetes:

- **Blobs de datos**: fragmentos del contenido de los archivos
- **Blobs de árbol**: estructuras del árbol de directorios

## Archivos de índice { #index-files }

Los archivos de índice asocian los ID de blob con su ubicación dentro de los archivos de paquete. Cada entrada de índice registra:

- El ID del paquete que contiene el blob
- El ID del blob, su tipo, su desplazamiento y sus longitudes (comprimida y sin comprimir)

Los índices se escriben **antes** que las instantáneas para garantizar la consistencia ante fallos: si ocurre un fallo después de escribir el índice pero antes de escribir la instantánea, la siguiente depuración (prune) limpia el índice y los datos de paquete huérfanos.

## Instantáneas { #snapshots }

Una instantánea (snapshot) registra los metadatos de una sola operación de respaldo:

- Marca de tiempo
- Nombre de host y nombre de usuario
- Rutas respaldadas y patrones de exclusión
- Etiquetas
- ID del blob de árbol raíz (el punto de entrada al árbol de directorios)
- Estadísticas de resumen (archivos nuevos, archivos sin cambios, bytes agregados, bytes procesados, duración)

Las instantáneas se almacenan como archivos JSON cifrados en el directorio `snapshots/`.

## Caché { #cache }

HitStor mantiene una base de datos **SQLite** local para mejorar el rendimiento de los respaldos incrementales:

```
~/.cache/hitstor/<repo-id>/cache.db
```

La base de datos usa el modo **WAL** (Write-Ahead Logging) y registra:

- **files**: asocia las rutas de los archivos (por su hash SHA-256) con su inode, tamaño, mtime e ID de fragmento. Un acierto de caché (los tres campos de metadatos coinciden) significa que el archivo no cambió y que sus fragmentos se pueden reutilizar sin volver a leer el archivo.
- **known_chunks**: registra los ID de fragmento que existen en el repositorio, lo que evita búsquedas redundantes en el índice.
- **meta**: contador de generación para la limpieza de la caché.

Cada apertura de la caché incrementa un contador de generación. Las entradas antiguas que no se han visto durante varias generaciones se pueden limpiar.

## Estructura del repositorio { #repository-layout }

```
<repo>/
├── config                          # Unencrypted JSON: repo ID, chunker params
├── data/
│   └── <first-2-hex>/
│       └── <pack-id>              # Encrypted pack files (blobs + header)
├── index/
│   ├── latest                     # Plain-text pointer to consolidated index ID (optional)
│   └── <index-id>                 # Encrypted JSON: blob → pack mapping
├── keys/
│   └── <key-id>                   # Encrypted JSON: master key + KDF params
├── locks/
│   └── <lock-id>                  # Lock files for concurrency control
└── snapshots/
    └── <snapshot-id>              # Encrypted JSON: snapshot metadata + tree ref
```

Todos los ID son valores de 32 bytes representados como cadenas hexadecimales de 64 caracteres. El directorio `data/` usa un prefijo de distribución de dos caracteres (los dos primeros caracteres hexadecimales del ID del paquete) para evitar una cantidad excesiva de entradas por directorio.

## Bloqueo del repositorio { #repository-locking }

HitStor usa archivos de bloqueo en `locks/` para coordinar a los procesos que escriben. Un bloqueo exclusivo registra el nombre de host, el PID, el UID y la marca de tiempo. Un segundo proceso que intente escribir recibe un error, a menos que el bloqueo existente esté huérfano (tenga más de 30 minutos y el proceso propietario no esté activo en el mismo host). Use `--break-lock` en los comandos de la CLI para eliminar los bloqueos huérfanos antes de ejecutar backup, prune, forget o check.

El servidor adquiere un bloqueo exclusivo por cada sesión de respaldo activa (no durante toda la vida de `serve`), por lo que los respaldos locales con la CLI se pueden ejecutar cuando no hay ninguna sesión remota en curso.

Durabilidad opcional: configure `sync_writes: true` en la configuración o `HITSTOR_SYNC_WRITES=1` para ejecutar `fsync` sobre los archivos del repositorio local después del renombrado atómico.

En Unix, un `flock` adicional sobre `locks/.exclusive` protege contra escrituras concurrentes en sistemas de archivos locales. Los archivos de bloqueo JSON en `locks/` siguen siendo el mecanismo de coordinación entre hosts.

`min_free_reserve_bytes` en la configuración (predeterminado: 1 GiB) controla el espacio libre mínimo en disco requerido antes de escribir nuevos archivos de paquete en backends locales.

Cuando existen más de 10 archivos de índice, HitStor los consolida en uno solo y registra su ID en `index/latest`, de modo que las aperturas posteriores carguen primero el índice consolidado.

## Protocolo de respaldo remoto { #remote-backup-protocol }

Cuando se usa el agente remoto (`hitstor-agent`), el proceso de respaldo se divide entre el agente y el servidor:

**Responsabilidades del agente:**

- Exploración del sistema de archivos
- Fragmentación definida por contenido (FastCDC)
- Cálculo de los ID de blob (HMAC-SHA256 con la IDKey proporcionada por el servidor)
- Compresión Zstd

**Responsabilidades del servidor:**

- Verificación de la deduplicación
- Cifrado AES-256-GCM
- Ensamblado de los archivos de paquete
- Gestión de los índices
- Creación de las instantáneas

La IDKey se transmite al agente al inicio de la sesión, pero la EncryptKey nunca sale del servidor. Consulte la [Referencia de la API gRPC](grpc-api.md) para ver los detalles del protocolo.
