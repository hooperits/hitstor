# Guía de desarrollo de plugins

HitStor admite cuatro tipos de plugins mediante el framework [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin). Los plugins se ejecutan como procesos separados y se comunican con HitStor por gRPC.

## Tipos de plugins { #plugin-types }

| Tipo | Patrón del binario | Clave de configuración | Archivo proto |
|------|---------------|------------|------------|
| Backend de almacenamiento | `hitstor-backend-<name>` | `repo: plugin:<name>/args` | `api/proto/hitstor/plugin/v1/storage.proto` |
| Notificador | `hitstor-notifier-<name>` | `alert_notifier_plugins` | `api/proto/hitstor/plugin/v1/notifier.proto` |
| Hook | `hitstor-hook-<name>` | `hook_plugins` | `api/proto/hitstor/plugin/v1/hook.proto` |
| Escáner de sistemas de archivos | `hitstor-scanner-<name>` | `fs_scanner_plugin` | `api/proto/hitstor/plugin/v1/scanner.proto` |

## Conceptos comunes { #common-concepts }

### Descubrimiento { #discovery }

Los plugins se descubren buscando binarios que coincidan con el patrón de nombres en:

1. La ruta de configuración `plugin_dir` (si está definida)
2. El `$PATH` del sistema

### Handshake { #handshake }

Todos los plugins usan el mismo handshake con magic cookie (`HITSTOR_PLUGIN`). Si el handshake falla, el proceso del plugin se termina de inmediato.

### Interfaz configurable { #configurable-interface }

Todos los tipos de plugins admiten, de forma opcional, una RPC `Configure(map[string]string)`, que se llama una vez después del lanzamiento. Implemente `plugin.Configurable` para recibir la configuración.

### Versión del protocolo { #protocol-version }

El handshake usa `ProtocolVersion: 1`. Los cambios incompatibles en el protocolo incrementarán esta versión, lo que hará que los plugins anteriores fallen de forma controlada.

---

## Plugins de backend de almacenamiento { #storage-backend-plugins }

Los plugins de backend de almacenamiento proporcionan almacenamiento alternativo para el repositorio (proveedores de nube, bases de datos, etc.).

### Arquitectura { #architecture }

```
hitstor (host)                    hitstor-backend-<name> (plugin)
     |                                    |
     |--- launch subprocess ------------->|
     |--- gRPC handshake (magic cookie) ->|
     |--- Configure(settings) ----------->|
     |--- Save/Load/Stat/Remove/List ---->|
     |--- Close() kills subprocess ------>|
```

### RPC del servicio { #service-rpcs }

| RPC | Descripción |
|-----|-------------|
| `Configure` | Recibe la configuración clave-valor específica del plugin. Se llama una vez después del lanzamiento. |
| `Save` | Almacena datos bajo un tipo de archivo y un ID. |
| `Load` | Recupera los datos de un tipo de archivo y un ID. |
| `Stat` | Devuelve los metadatos (tamaño) de un archivo almacenado. |
| `Remove` | Elimina un archivo almacenado. |
| `List` | Devuelve todos los ID de un tipo de archivo dado. |

### Tipos de archivo { #file-types }

| Valor | Constante | Descripción |
|-------|----------|-------------|
| 0 | `DataFile` | Archivos de paquete que contienen fragmentos (chunks) cifrados |
| 1 | `IndexFile` | Archivos de índice que asocian los ID de fragmentos con su ubicación en los paquetes |
| 2 | `SnapshotFile` | Metadatos de instantáneas (snapshots) |
| 3 | `KeyFile` | Archivos de claves de cifrado |
| 4 | `LockFile` | Bloqueos del repositorio |
| 5 | `ConfigFile` | Configuración del repositorio |

### Esquema de URI { #uri-scheme }

```
--repo plugin:azure/myaccount/container  ->  hitstor-backend-azure
--repo plugin:gcs/my-bucket              ->  hitstor-backend-gcs
```

La parte que sigue al nombre del plugin se pasa como la clave `uri` en `Configure()`.

### Escribir un plugin de backend de almacenamiento { #writing-a-storage-backend-plugin }

```go
package main

import (
    "github.com/hooperits/hitstor/internal/backend"
    hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
)

type myBackend struct{}

var _ backend.Backend = (*myBackend)(nil)

// Implement Save, Load, Stat, Remove, List, Close...

func (b *myBackend) Configure(settings map[string]string) error {
    uri := settings["uri"]
    // parse uri, set up connection
    return nil
}

func main() {
    hitstorplugin.ServeStorageBackend(&myBackend{})
}
```

**Ejemplo:** consulte `cmd/hitstor-backend-memory/main.go` para ver un backend en memoria completo (~90 líneas).

---

## Plugins de notificador { #notifier-plugins }

Los plugins de notificador entregan notificaciones de eventos de respaldo a sistemas externos (Slack, PagerDuty, correo electrónico, etc.).

### Arquitectura { #architecture_1 }

```
hitstor serve (host)                  hitstor-notifier-<name> (plugin)
     |                                       |
     |--- launch subprocess --------------->|
     |--- gRPC handshake (magic cookie) --->|
     |--- Configure(settings) ------------->|
     |--- Notify(event) ---- (per event) ->|
     |--- Close() kills subprocess -------->|
```

### RPC del servicio { #service-rpcs_1 }

| RPC | Descripción |
|-----|-------------|
| `Configure` | Recibe la configuración clave-valor específica del plugin. Se llama una vez después del lanzamiento. |
| `Notify` | Entrega una notificación de evento de respaldo. Se llama para cada evento. |

### Campos del evento { #event-fields }

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `event_type` | string | `"backup_failed"` o `"agent_stale"` |
| `server` | string | Siempre `"hitstor"` |
| `timestamp` | string | Marca de tiempo RFC3339 |
| `agent` | string | Nombre del agente que generó el evento |
| `hostname` | string | Nombre de host del agente (puede estar vacío) |
| `message` | string | Descripción del evento legible por humanos |
| `details` | map | Metadatos clave-valor arbitrarios |

### Configuración { #configuration }

```bash
hitstor config add alert_notifier_plugins slack
```

El notificador webhook integrado (`alert_webhook_url`) y los notificadores de plugins pueden estar activos al mismo tiempo.

### Escribir un plugin de notificador { #writing-a-notifier-plugin }

```go
package main

import (
    "context"
    "github.com/hooperits/hitstor/internal/notifier"
    hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
)

type myNotifier struct{}

var _ notifier.Notifier = (*myNotifier)(nil)

func (n *myNotifier) Notify(ctx context.Context, event notifier.Event) error {
    // Deliver the event to your notification channel
    return nil
}

func (n *myNotifier) Close() error { return nil }

func main() {
    hitstorplugin.ServeNotifier(&myNotifier{})
}
```

**Ejemplo:** consulte `cmd/hitstor-notifier-log/main.go` para ver un plugin de notificador basado en registros.

---

## Plugins de hook { #hook-plugins }

Los plugins de hook se ejecutan antes y después de los respaldos, lo que permite lógica personalizada como poner en reposo (quiesce) una base de datos, precalentar cachés o registrar auditorías.

### Arquitectura { #architecture_2 }

```
hitstor (host)                        hitstor-hook-<name> (plugin)
     |                                       |
     |--- launch subprocess --------------->|
     |--- gRPC handshake (magic cookie) --->|
     |--- Configure(settings) ------------->|
     |--- PreBackup(event) --- (before) --->|
     |--- PostBackup(event) -- (after) ---->|
     |--- Close() kills subprocess -------->|
```

### RPC del servicio { #service-rpcs_2 }

| RPC | Descripción |
|-----|-------------|
| `Configure` | Recibe la configuración clave-valor específica del plugin. Se llama una vez después del lanzamiento. |
| `PreBackup` | Se llama antes de que comience el respaldo. Devuelva un error para cancelar el respaldo. |
| `PostBackup` | Se llama después de un respaldo exitoso. Los errores se registran, pero no hacen fallar el respaldo. |

### Campos del evento { #event-fields_1 }

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `agent` | string | Nombre del agente que realiza el respaldo |
| `hostname` | string | Nombre de host del agente |
| `paths` | repeated string | Rutas de origen del respaldo |
| `excludes` | repeated string | Patrones de exclusión |
| `tags` | repeated string | Etiquetas del respaldo |
| `snapshot_id` | string | ID de la instantánea (solo PostBackup) |
| `details` | map | Metadatos clave-valor arbitrarios |

### Puntos de integración { #integration-points }

- **Lado del servidor (`hitstor serve`):** PreBackup se ejecuta en `BeginSession()` después de la autenticación. PostBackup se ejecuta en `FinalizeBackup()` después de crear la instantánea.
- **Respaldo local (`hitstor backup`):** PreBackup se ejecuta antes de que inicie el archivador. PostBackup se ejecuta después de un respaldo exitoso.

### Configuración { #configuration_1 }

```bash
hitstor config add hook_plugins myhook
```

### Escribir un plugin de hook { #writing-a-hook-plugin }

```go
package main

import (
    "context"
    "github.com/hooperits/hitstor/internal/hook"
    hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
)

type myHook struct{}

var _ hook.Hook = (*myHook)(nil)

func (h *myHook) PreBackup(ctx context.Context, event hook.HookEvent) error {
    // Return an error to abort the backup
    return nil
}

func (h *myHook) PostBackup(ctx context.Context, event hook.HookEvent) error {
    // event.SnapshotID contains the new snapshot ID
    return nil
}

func (h *myHook) Close() error { return nil }

func main() {
    hitstorplugin.ServeHook(&myHook{})
}
```

**Ejemplo:** consulte `cmd/hitstor-hook-log/main.go` para ver un plugin de hook basado en registros.

---

## Plugins de escáner de sistemas de archivos { #filesystem-scanner-plugins }

Los plugins de escáner reemplazan el recorrido de directorios integrado por una lógica de escaneo personalizada para almacenamiento en la nube, volcados de bases de datos, recursos compartidos de red o filtrado personalizado.

### Arquitectura { #architecture_3 }

```
hitstor (host)                        hitstor-scanner-<name> (plugin)
     |                                       |
     |--- launch subprocess --------------->|
     |--- gRPC handshake (magic cookie) --->|
     |--- Configure(settings) ------------->|
     |--- Scan(paths, excludes) ----------->|
     |<-- stream ScanEntry (per file) ------|
     |--- Close() kills subprocess -------->|
```

La RPC `Scan` usa **streaming del lado del servidor**: el plugin envía un `ScanEntry` por cada archivo o directorio.

### RPC del servicio { #service-rpcs_3 }

| RPC | Descripción |
|-----|-------------|
| `Configure` | Recibe la configuración clave-valor específica del plugin. Se llama una vez después del lanzamiento. |
| `Scan` | Recorre las rutas y transmite al host las entradas de archivos y directorios. |

### Campos de ScanEntry { #scanentry-fields }

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `path` | string | Ruta absoluta del archivo o directorio |
| `node` | `NodeInfo` | Metadatos del archivo |
| `error_message` | string | Error no fatal de esta entrada |

### Campos de NodeInfo { #nodeinfo-fields }

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `name` | string | Nombre base |
| `type` | string | `"file"`, `"dir"` o `"symlink"` |
| `mode` | uint32 | Bits de modo de archivo Unix |
| `uid` | uint32 | ID del usuario propietario |
| `gid` | uint32 | ID del grupo propietario |
| `mtime_ns` | int64 | Hora de modificación (nanosegundos desde epoch) |
| `atime_ns` | int64 | Hora de acceso (nanosegundos desde epoch) |
| `ctime_ns` | int64 | Hora de cambio (nanosegundos desde epoch) |
| `size` | uint64 | Tamaño del archivo en bytes |
| `inode` | uint64 | Número de inodo |
| `links` | uint64 | Cantidad de enlaces duros |
| `link_target` | string | Ruta de destino del enlace simbólico |

### Configuración { #configuration_2 }

```bash
# Solo puede haber un plugin de escáner activo a la vez
hitstor config set fs_scanner_plugin myscanner
```

### Escribir un plugin de escáner { #writing-a-scanner-plugin }

```go
package main

import (
    "github.com/hooperits/hitstor/internal/scanner"
    hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
)

type myScanner struct{}

var _ scanner.Scanner = (*myScanner)(nil)

func (s *myScanner) Scan(paths []string, excludes []string) (<-chan scanner.ScanResult, error) {
    ch := make(chan scanner.ScanResult)
    go func() {
        defer close(ch)
        // Walk paths and send results on the channel
    }()
    return ch, nil
}

func (s *myScanner) Close() error { return nil }

func main() {
    hitstorplugin.ServeFilesystemScanner(&myScanner{})
}
```

**Ejemplo:** consulte `cmd/hitstor-scanner-builtin/main.go` para ver un plugin de escáner que envuelve el recorrido de directorios integrado.

---

## Pruebas de plugins { #testing-plugins }

### Pruebas unitarias { #unit-tests }

Pruebe directamente la implementación de su interfaz sin lanzar un subproceso:

```go
be := &myBackend{}
be.Configure(map[string]string{"uri": "test"})
be.Save(ctx, backend.DataFile, id, data)
// ...
```

### Pruebas de integración { #integration-tests }

Compile el binario del plugin y luego láncelo mediante el administrador de plugins:

```go
be, err := plugin.OpenStorageBackend("mycloud", "/path/to/bin", map[string]string{
    "uri": "test-bucket",
})
defer be.Close()
```

Consulte `internal/plugin/integration_test.go` para ver ejemplos funcionales de los cuatro tipos de plugins.

## Límites de tamaño de mensaje { #message-size-limits }

go-plugin establece el tamaño máximo de mensaje gRPC en ~2 GiB. Los archivos de paquete de HitStor tienen un máximo de 128 MiB, por lo que los backends de almacenamiento no necesitan streaming. El plugin de escáner de sistemas de archivos usa streaming del lado del servidor para árboles de directorios grandes.

## Consulte también { #see-also }

- [Guía de uso de plugins](../user-guide/plugins.md) -- instalar y configurar plugins
- [Referencia de la API](api-reference.md) -- protocolo gRPC principal de respaldo
