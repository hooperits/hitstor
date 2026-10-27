# Referencia de la API gRPC

!!! note "Nota"
    Para la API JSON del dashboard integrado, consulte [API HTTP del dashboard](dashboard-api.md).

HitStor usa gRPC sobre TLS para la comunicación entre componentes. Hay dos servicios gRPC:

- **BackupService**: del lado del servidor; atiende las operaciones de respaldo del agente hacia el servidor (`internal/proto/hitstor/v1/backup.proto`)
- **AgentService**: del lado del agente; atiende las operaciones de administración del servidor hacia el agente (`internal/proto/hitstor/v1/agent.proto`)

## Servicio: `BackupService` { #service-backupservice }

| RPC | Tipo | Descripción |
|-----|------|-------------|
| `BeginSession` | Unaria | Inicia una sesión de respaldo y recibe los parámetros del fragmentador (chunker) y la IDKey |
| `CheckBlobs` | Unaria | Comprueba qué ID de blob ya tiene el servidor (por lotes) |
| `UploadChunks` | Streaming del cliente | Envía en flujo los fragmentos comprimidos al servidor |
| `FinalizeBackup` | Unaria | Envía el árbol de archivos, crea la instantánea y cierra la sesión |
| `Ping` | Unaria | Verificación de estado; devuelve la versión del servidor |
| `GetSchedule` | Unaria | Consulta la programación de respaldos de este agente |

## Autenticación { #authentication }

Todas las RPC de BackupService (excepto `Ping`) requieren una clave de API válida. La clave es una cadena hexadecimal de 64 caracteres, generada con `hitstor serve agent add <name>` (CLI) o enviada al agente durante el registro desde el dashboard.

- `BeginSession` y `GetSchedule`: la clave de API se envía en el mensaje de solicitud (campo `api_key`)
- `CheckBlobs`, `UploadChunks`, `FinalizeBackup`: se autentican mediante el `session_id` obtenido de `BeginSession`
- `Ping`: no requiere autenticación

## Flujo de respaldo { #backup-flow }

```
Agent                                          Server
  |                                              |
  |-- BeginSession(api_key, hostname, paths) --> |
  |<-- session_id, id_key, chunker params -------|
  |                                              |
  |  [scan files, chunk with FastCDC,            |
  |   compute blob IDs with id_key]              |
  |                                              |
  |-- CheckBlobs(session_id, blob_ids[]) ------> |
  |<-- needed_ids[] -----------------------------|
  |  (repeat in batches of ~1000)                |
  |                                              |
  |-- UploadChunks(stream) --------------------> |
  |   [blob_id + compressed_data per chunk]      |
  |<-- chunks_received, bytes_received ----------|
  |                                              |
  |-- FinalizeBackup(session_id, files[]) ------> |
  |<-- snapshot_id, stats -----------------------|
```

## RPC { #rpcs }

### BeginSession

Inicia una sesión de respaldo para un agente autenticado.

**Solicitud: `BeginSessionRequest`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `api_key` | string | Clave de API hexadecimal de 64 caracteres |
| `hostname` | string | Nombre de host de la máquina que se respalda |
| `paths` | repeated string | Rutas absolutas que se respaldan |
| `excludes` | repeated string | Patrones glob que se excluyen |
| `tags` | repeated string | Etiquetas opcionales de la instantánea |
| `job_id` | string | ID de la tarea de respaldo (vincula la sesión con una tarea del dashboard) |

**Respuesta: `BeginSessionResponse`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `session_id` | string | Identificador único de la sesión |
| `id_key` | bytes | Clave HMAC-SHA256 de 32 bytes para calcular los ID de blob |
| `chunker_min` | uint32 | Tamaño mínimo de fragmento en bytes (predeterminado: 262144) |
| `chunker_avg` | uint32 | Tamaño promedio de fragmento en bytes (predeterminado: 1048576) |
| `chunker_max` | uint32 | Tamaño máximo de fragmento en bytes (predeterminado: 4194304) |
| `previous_files` | repeated `PreviousFileEntry` | Metadatos de la instantánea padre para la detección incremental |
| `parent_snapshot_id` | string | ID hexadecimal de la instantánea padre (vacío en un respaldo completo) |

**Campos de `PreviousFileEntry`:** `path`, `mtime_ns`, `size`, `content_ids` (ID de blob de los fragmentos, en orden).

### CheckBlobs

Comprueba qué ID de blob ya tiene el servidor. Se llama por lotes (recomendado: 1000 ID por llamada).

**Solicitud: `CheckBlobsRequest`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `session_id` | string | Sesión obtenida de `BeginSession` |
| `blob_ids` | repeated bytes | Lote de ID de blob de 32 bytes que se comprueban |

**Respuesta: `CheckBlobsResponse`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `needed_ids` | repeated bytes | ID de blob que el servidor NO tiene (súbalos) |

### UploadChunks

Envía en flujo los fragmentos comprimidos al servidor. Es una RPC con streaming del lado del cliente: el agente envía varios mensajes `UploadChunkMessage` en un único flujo.

**Solicitud (flujo): `UploadChunkMessage`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `session_id` | string | Sesión obtenida de `BeginSession` |
| `blob_id` | bytes | ID de blob de 32 bytes (HMAC-SHA256 de los datos sin comprimir) |
| `compressed_data` | bytes | Datos del fragmento comprimidos con Zstd |
| `uncompressed_length` | uint32 | Tamaño original sin comprimir, en bytes |

**Respuesta: `UploadChunksResponse`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `chunks_received` | uint32 | Total de fragmentos recibidos y almacenados |
| `bytes_received` | uint64 | Total de bytes comprimidos recibidos |

### FinalizeBackup

Envía el árbol completo de archivos y directorios y cierra la sesión. El servidor construye los blobs de árbol, crea una instantánea y devuelve estadísticas de resumen.

**Solicitud: `FinalizeBackupRequest`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `session_id` | string | Sesión obtenida de `BeginSession` |
| `files` | repeated `FileNode` | Árbol de directorios completo |
| `backup_type` | string | `"full"` o `"incremental"` |
| `parent_snapshot_id` | string | ID hexadecimal de la instantánea padre (solo incremental) |

**Campos de `FileNode`:**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `path` | string | Ruta relativa (barras normales, sin barra inicial) |
| `is_dir` | bool | `true` para directorios |
| `mode` | uint32 | Modo de archivo Unix (permisos) |
| `mtime_ns` | int64 | Fecha de modificación (nanosegundos desde la época Unix) |
| `size` | int64 | Tamaño del archivo en bytes (0 para directorios) |
| `uid` | uint32 | ID del usuario propietario |
| `gid` | uint32 | ID del grupo propietario |
| `content_ids` | repeated bytes | ID de blob de los fragmentos, en orden (solo archivos) |
| `link_target` | string | Ruta de destino del enlace simbólico (solo enlaces simbólicos) |

**Respuesta: `FinalizeBackupResponse`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `snapshot_id` | string | Prefijo hexadecimal de 8 caracteres del ID de la instantánea |
| `files_new` | uint64 | Número de archivos nuevos |
| `files_unchanged` | uint64 | Número de archivos sin cambios |
| `bytes_added` | uint64 | Datos nuevos agregados al repositorio |
| `bytes_processed` | uint64 | Total de datos procesados |
| `duration_seconds` | double | Duración del respaldo |

### Ping

RPC de verificación de estado. No requiere autenticación.

**Solicitud:** `PingRequest` (vacía)

**Respuesta: `PingResponse`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `version` | string | Versión del servidor HitStor |
| `repo_id` | string | ID hexadecimal del repositorio, de 64 caracteres |

### GetSchedule

Devuelve la programación de respaldos configurada para este agente.

**Solicitud: `GetScheduleRequest`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `api_key` | string | Clave de API hexadecimal de 64 caracteres |

**Respuesta: `GetScheduleResponse`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `enabled` | bool | Heredado: primera tarea habilitada (compatibilidad con versiones anteriores) |
| `interval_hours` | int32 | Heredado: intervalo de la primera tarea |
| `next_run_unix` | int64 | Heredado: próxima ejecución de la primera tarea |
| `jobs` | repeated `ScheduledJob` | Todas las tareas de respaldo de este agente |

**Campos de `ScheduledJob`:** `job_id`, `name`, `paths`, `excludes`, `repo_name`, `enabled`, `interval_hours`, `next_run_unix`, `run_now`, `full_rescan_days`, `last_full_backup_unix`, `cron_expression`, `timezone`, `use_vss`.

## Transporte { #transport }

- **Protocolo:** gRPC sobre TLS (obligatorio)
- **Puerto:** 8420 (predeterminado, configurable mediante `serve_port`)
- **TLS:** el servidor genera un certificado autofirmado si no se proporciona ninguno; los agentes pueden usar `--ca-cert` para confiar en CA personalizadas
- **Tamaño máximo de mensaje:** ~2 GiB (definido por el framework go-plugin; los archivos de paquete tienen un límite de 128 MiB)

## Códigos de error { #error-codes }

| Código gRPC | Significado |
|-------------|-------------|
| `Unauthenticated` | Clave de API no válida o ausente |
| `NotFound` | ID de sesión desconocido |
| `FailedPrecondition` | Un hook previo al respaldo rechazó la operación |
| `Internal` | Error del lado del servidor (revise los registros) |

## Servicio: `AgentService` { #service-agentservice }

AgentService se ejecuta en cada agente (puerto predeterminado 8422) y atiende las operaciones de administración del servidor hacia el agente. El servidor se conecta a este servicio al agregar agentes desde el dashboard, explorar sistemas de archivos o configurar rutas de respaldo.

| RPC | Tipo | Descripción |
|-----|------|-------------|
| `Register` | Unaria | Registra el agente en un servidor (envía la clave de API + la dirección del servidor) |
| `ListDirectory` | Unaria | Lista los archivos y directorios de una ruta en el agente |
| `Configure` | Unaria | Define las rutas de respaldo y los patrones de exclusión en el agente |
| `AgentInfo` | Unaria | Consulta el nombre de host, la versión, el sistema operativo, las rutas y el estado de registro del agente |
| `RestoreData` | Streaming del cliente | Envía en flujo los datos de restauración al agente |
| `WakeUp` | Unaria | Indica al agente que revise su programación de inmediato ("Respaldar ahora" en el dashboard) |

### Autenticación de AgentService { #agentservice-authentication }

Todas las RPC de AgentService requieren un **token de agente** válido: una cadena hexadecimal de 64 caracteres generada durante la instalación del agente. El token se envía en el campo `token` de cada mensaje de solicitud.

### Register

La llama el servidor cuando un administrador agrega el agente desde el dashboard. El servidor envía una clave de API y su dirección; el agente las almacena e inicia el ciclo de respaldo.

**Solicitud: `RegisterRequest`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `token` | string | Token de agente hexadecimal de 64 caracteres |
| `api_key` | string | Clave de API para la autenticación de respaldos del agente hacia el servidor |
| `server_addr` | string | Dirección del servidor (`host:port`) a la que se envían los respaldos |
| `server_ca_cert` | bytes | Certificado TLS codificado en PEM (opcional) |

**Respuesta: `RegisterResponse`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `hostname` | string | Nombre de host de la máquina del agente |
| `version` | string | Versión del binario del agente |
| `os` | string | Sistema operativo (por ejemplo, `linux`) |
| `arch` | string | Arquitectura (por ejemplo, `amd64`) |

### ListDirectory

Explora el sistema de archivos del agente. Se llama cuando el administrador expande un directorio en el explorador de árbol del sistema de archivos del dashboard.

**Solicitud: `ListDirectoryRequest`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `token` | string | Token de agente hexadecimal de 64 caracteres |
| `path` | string | Ruta absoluta que se lista (una cadena vacía devuelve los directorios del nivel raíz) |

**Respuesta: `ListDirectoryResponse`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `entries` | repeated `DirEntry` | Entradas del directorio, ordenadas con los directorios primero |

**Campos de `DirEntry`:**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `name` | string | Nombre de la entrada |
| `path` | string | Ruta absoluta |
| `is_dir` | bool | `true` para directorios |
| `size` | int64 | Tamaño del archivo en bytes (0 para directorios) |
| `mode` | uint32 | Modo de archivo Unix |
| `readable` | bool | Indica si el proceso del agente puede leer esta ruta |
| `has_children` | bool | Indica si este directorio contiene subdirectorios (sugerencia para el icono de expandir en la interfaz) |

### Configure

Define las rutas de respaldo y los patrones de exclusión en el agente. Se llama cuando el administrador hace clic en "Guardar rutas de respaldo" en el dashboard.

**Solicitud: `ConfigureRequest`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `token` | string | Token de agente hexadecimal de 64 caracteres |
| `backup_paths` | repeated string | Rutas absolutas que se respaldan |
| `excludes` | repeated string | Patrones glob que se excluyen |

**Respuesta: `ConfigureResponse`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `success` | bool | Indica si la configuración se guardó |
| `message` | string | Mensaje de estado |

### AgentInfo

Consulta el estado actual del agente.

**Solicitud: `AgentInfoRequest`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `token` | string | Token de agente hexadecimal de 64 caracteres |

**Respuesta: `AgentInfoResponse`**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `hostname` | string | Nombre de host de la máquina del agente |
| `version` | string | Versión del binario del agente |
| `os` | string | Sistema operativo |
| `arch` | string | Arquitectura |
| `backup_paths` | repeated string | Rutas de respaldo configuradas actualmente |
| `excludes` | repeated string | Patrones de exclusión configurados actualmente |
| `registered` | bool | Indica si el agente está registrado en un servidor |
| `server_addr` | string | Dirección del servidor registrado (vacía si no está registrado) |

### WakeUp

Se llama cuando un administrador activa "Respaldar ahora" desde el dashboard, para que el agente no espere al siguiente intervalo de sondeo.

**Solicitud:** `WakeUpRequest`: `token` (token de agente hexadecimal de 64 caracteres)

**Respuesta:** `WakeUpResponse`: `acknowledged` (bool)

## Transporte de AgentService { #agentservice-transport }

- **Protocolo:** gRPC sobre TLS (autofirmado, generado automáticamente por el agente)
- **Puerto:** 8422 (predeterminado, configurable mediante `--listen-port` o `HITSTOR_AGENT_LISTEN_PORT`)
- **TLS:** el agente genera un certificado autofirmado; el servidor usa `InsecureSkipVerify`, ya que el token proporciona la autenticación

## Vea también { #see-also }

- [Funcionamiento interno](internals.md): detalles de cifrado, fragmentación y deduplicación
- [Desarrollo de plugins](plugin-development.md): protocolos gRPC de los plugins
