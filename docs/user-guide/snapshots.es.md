# Instantáneas

Cada vez que ejecuta `hitstor backup`, se crea una nueva **instantánea (snapshot)**. Una instantánea es un registro de metadatos que apunta a los fragmentos deduplicados que componen sus archivos en un momento determinado. En esta página se explica cómo listar, filtrar, comparar e inspeccionar instantáneas.

## Listar todas las instantáneas { #list-all-snapshots }

```console
hitstor snapshots --repo /path/to/repo --password "secret"
ID        Time                 Host        Tags        Paths
----------------------------------------------------------------------------
abc12345  2026-02-20 08:00:12  webserver   daily       /var/www
bcd23456  2026-02-21 08:00:09  webserver   daily       /var/www
cde34567  2026-02-22 08:00:11  webserver   daily       /var/www
def45678  2026-02-22 10:30:00  laptop      manual      /home/user
```

El alias `hitstor ls` funciona exactamente igual:

```console
hitstor ls --repo /path/to/repo --password "secret"
```

## Filtrar instantáneas { #filtering-snapshots }

### Por host { #by-host }

Muestre solo las instantáneas registradas desde un host específico:

```console
hitstor snapshots --repo /path/to/repo --password "secret" --host webserver
ID        Time                 Host        Tags        Paths
----------------------------------------------------------------------------
abc12345  2026-02-20 08:00:12  webserver   daily       /var/www
bcd23456  2026-02-21 08:00:09  webserver   daily       /var/www
cde34567  2026-02-22 08:00:11  webserver   daily       /var/www
```

### Por ruta { #by-path }

Muestre solo las instantáneas que incluyen una ruta de respaldo específica:

```console
hitstor snapshots --repo /path/to/repo --password "secret" --path /home/user
ID        Time                 Host        Tags        Paths
----------------------------------------------------------------------------
def45678  2026-02-22 10:30:00  laptop      manual      /home/user
```

### Combinar filtros { #combining-filters }

`--host` y `--path` se pueden usar juntos. Para aparecer, una instantánea debe coincidir con **ambos** filtros.

## Salida JSON { #json-output }

Para scripts y para la integración con otras herramientas, use `--json` para obtener una salida legible por máquinas:

```console
hitstor snapshots --repo /path/to/repo --password "secret" --json
```

La salida es un arreglo JSON de objetos de instantánea:

```json
[
  {
    "id": "abc12345...",
    "time": "2026-02-20T08:00:12Z",
    "host": "webserver",
    "tags": ["daily"],
    "paths": ["/var/www"]
  },
  ...
]
```

Esto resulta útil para canalizar la salida hacia `jq` u otros procesadores de JSON:

```console
hitstor snapshots --repo /path/to/repo --password "secret" --json \
    | jq '.[].id'
```

## Entender los ID de instantánea { #understanding-snapshot-ids }

Cada instantánea se identifica mediante una cadena hexadecimal derivada de un hash criptográfico de su contenido. En la salida de tabla predeterminada, los ID se muestran como un prefijo corto (normalmente de 8 caracteres).

### Coincidencia por prefijo corto { #short-prefix-matching }

Siempre que un comando acepta un ID de instantánea, puede indicar un prefijo corto en lugar de la cadena hexadecimal completa. El prefijo debe ser lo bastante largo para identificar de forma única una instantánea del repositorio. Si el prefijo es ambiguo, HitStor muestra un error con la lista de instantáneas coincidentes para que usted indique un prefijo más largo.

```console
hitstor snapshots --repo /path/to/repo --password "secret"
ID        ...
--------  ...
abc12345  ...
abc12399  ...

hitstor restore --repo /path/to/repo --password "secret" abc123 /target
Fatal: prefix "abc123" matches multiple snapshots: abc12345, abc12399

hitstor restore --repo /path/to/repo --password "secret" abc12345 /target
restoring snapshot abc12345 ...
```

### La palabra clave `latest` { #the-latest-keyword }

La palabra clave `latest` se puede usar en lugar de un ID de instantánea. Corresponde a la instantánea más reciente del repositorio (según su marca de tiempo).

## Comparar instantáneas { #comparing-snapshots }

El comando `hitstor diff` muestra qué cambió entre dos instantáneas:

```console
hitstor diff --repo /path/to/repo --password "secret" abc12345 bcd23456
```

La salida lista los archivos agregados, eliminados o modificados:

```
+    /var/www/blog/new-post.html
-    /var/www/old-page.html
M    /var/www/index.html
M    /var/www/css/style.css
```

| Prefijo | Significado |
|--------|---------|
| `+` | Archivo agregado en la segunda instantánea |
| `-` | Archivo eliminado en la segunda instantánea |
| `M` | Archivo modificado entre las dos instantáneas |

Esto es útil para auditar qué cambió entre respaldos o para verificar que se haya capturado un conjunto de cambios esperado.

## Uso de disco { #disk-usage }

El comando `hitstor du` informa cuánto espacio del repositorio ocupan una o más instantáneas:

```console
hitstor du --repo /path/to/repo --password "secret"
Snapshot    Size
---------------------
abc12345    512.00 MiB
bcd23456      4.20 MiB
cde34567      3.80 MiB
def45678    128.00 MiB
---------------------
Total:      648.00 MiB
```

Debido a la deduplicación, los tamaños por instantánea reflejan solo los datos **únicos** que aporta cada instantánea. El total puede ser mucho menor que la suma de los tamaños de los archivos originales de todas las instantáneas.

!!! note "Nota"
    `hitstor du` lee el índice del repositorio y no descarga ni descifra los datos reales, por lo que es rápido incluso con repositorios grandes.
