<div align="center">

<div style="display: inline-block; padding: 2.5rem 3rem; border-radius: 12px; background: linear-gradient(145deg, #071018 0%, #0b1f3a 42%, #132d4f 100%); text-align: center; max-width: 480px;">

<img src="assets/icon.png" alt="" width="72" height="72" style="border-radius: 18px; box-shadow: 0 12px 32px rgba(0,0,0,0.35); margin-bottom: 1.5rem;">

<h1 style="margin: 0 0 0.75rem; font-family: system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif; font-size: 2.75rem; font-weight: 800; letter-spacing: -0.04em; line-height: 1;">
<span style="color: #f4f7fb">hit</span><span style="color: #ff7a45">stor</span>
</h1>

<p style="margin: 0; font-family: system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif; font-size: 1.05rem; line-height: 1.5; color: rgba(244,247,251,0.88);">
Respaldos cifrados y deduplicados para su infraestructura.
</p>

</div>

<br><br>

**Un único binario estático, sin dependencias en tiempo de ejecución.**

<br>

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white)](go.mod)
[![License](https://img.shields.io/badge/License-AGPL--3.0-0B1F3A?style=flat-square)](LICENSE)
[![Build](https://img.shields.io/badge/Build-CGO__disabled-0B1F3A?style=flat-square)](#compilación)
[![Dashboard](https://img.shields.io/badge/Dashboard-embedded-E8602C?style=flat-square)](#dashboard-web)
[![Agents](https://img.shields.io/badge/Agents-gRPC_TLS-FF7A45?style=flat-square)](#agente-de-respaldo-remoto)

<br>

[Inicio rápido](#inicio-rápido) · [Arquitectura](#arquitectura) · [Dashboard](#dashboard-web) · [Agentes](#agente-de-respaldo-remoto) · [Plugins](#backends-de-plugins) · [Desarrollo](#desarrollo)

</div>

[English](README.md) | **Español**

---

## Contenido

| | |
|---|---|
| [Características](#características) | [Inicio rápido](#inicio-rápido) · [Arquitectura](#arquitectura) |
| [Almacenamiento del repositorio](#almacenamiento-del-repositorio) | [Configuración](#configuración) |
| [Servidor de respaldo remoto](#servidor-de-respaldo-remoto) | [Agente de respaldo remoto](#agente-de-respaldo-remoto) |
| [Dashboard web](#dashboard-web) | [Backends de plugins](#backends-de-plugins) |
| [Desarrollo](#desarrollo) | [Licencia](#licencia) |

---

## Características

### Seguridad y motor de almacenamiento

| | |
|---|---|
| **Cifrado del lado del cliente** | AES-256-GCM con derivación de clave scrypt: los datos se cifran antes de salir de la máquina |
| **Deduplicación definida por contenido** | La fragmentación (chunking) FastCDC (256 KiB–4 MiB) identifica bloques idénticos entre respaldos |
| **Compresión** | Zstd aplicado a cada fragmento antes del cifrado |
| **Múltiples backends** | Sistema de archivos local, almacenamiento compatible con S3 (AWS S3, MinIO, etc.) y backends de plugins |
| **Un único binario estático** | `CGO_ENABLED=0`: sin dependencias en tiempo de ejecución |

### Respaldo y restauración

| | |
|---|---|
| **Respaldos incrementales** | El servidor envía a los agentes los metadatos de las instantáneas (snapshots); la comparación de mtime/tamaño omite los archivos sin cambios, sin requerir estado local |
| **Tipos de instantánea** | Etiquetas de completa e incremental en el dashboard, con trazabilidad de la instantánea padre |
| **Reescaneo completo periódico** | Un intervalo configurable protege contra la corrupción silenciosa; se puede lanzar a demanda desde el dashboard |
| **E/S en paralelo** | Grupos de workers para respaldos y restauraciones 2× o más rápidos en sistemas multinúcleo |
| **Restauración desde el dashboard** | Explore árboles de archivos, elija rutas, seleccione un agente de destino y supervise el progreso mediante SSE |

### Operaciones remotas

| | |
|---|---|
| **Servidor de respaldo remoto** | `hitstor serve`: gRPC sobre TLS; los agentes suben datos ya fragmentados; una clave de API por agente |
| **Agente de respaldo remoto** | `hitstor-agent` para Linux amd64/arm64: temporizadores de systemd o programación del daemon gestionada por el servidor |
| **Progreso del respaldo en vivo** | El dashboard muestra las tareas en ejecución, la fase y el rendimiento; avisos de sesiones inactivas en Resumen |
| **Barra de progreso en vivo** | Interfaz de terminal: `[####----] 42% \| 3,200/7,600 files \| 45.2 MiB/s \| ETA 1m23s` |

### Dashboard y experiencia de uso

| | |
|---|---|
| **Dashboard web** | Interfaz HTML5 integrada: agentes, instantáneas, almacenamiento, progreso en vivo; autenticación opcional con contraseña; htmx + Chart.js dentro del binario |
| **Salida en colores** | Colores ANSI en la terminal, con soporte para [NO_COLOR](https://no-color.org) |
| **Errores comprensibles** | Mensajes detectados por patrones, con sugerencias según el contexto |

### Plugins e integraciones

HitStor es un producto de respaldo y restauración de sistemas de archivos para Windows y Linux.

HitStor utiliza una arquitectura modular de plugins. Los binarios principales `hitstor` y `hitstor-agent` se centran en el motor y en el respaldo de sistemas de archivos (mediante el plugin escáner integrado). Las fuentes de respaldo avanzadas se entregan como plugins opcionales independientes.

| | |
|---|---|
| **Backends de plugins** | Backends de almacenamiento con HashiCorp go-plugin (proto `StorageBackend`) |
| **Plugins notificadores** | Slack, PagerDuty, correo electrónico, etc. mediante el proto `Notifier` |
| **Plugins de hooks** | Ciclo de vida previo/posterior al respaldo mediante el proto `Hook` |

---

## Inicio rápido

> **Ruta más rápida (servidor + agentes):** [docs/installation/quickstart.md](docs/installation/quickstart.md)

```bash
cp .env.example .env   # defina HITSTOR_PASSWORD
docker compose -f docker-compose.quickstart.yml up -d
# Dashboard → http://localhost:8421 — luego instale hitstor-agent en cada host
```

### Compilación

```bash
# Compilación estándar
go build -o hitstor ./cmd/hitstor

# Compilación con inyección de versión
make build

# Instalar en su PATH
sudo cp bin/hitstor /usr/local/bin/
```

### Inicializar un repositorio

```bash
# Local
hitstor init --repo /path/to/repo --password "secret"

# S3
hitstor init --repo s3:bucket-name/prefix \
  --password "secret" \
  --s3-access-key KEY --s3-secret-key SECRET
```

### Respaldo

```bash
hitstor backup --repo /path/to/repo --password "secret" /data/to/backup

# Controlar el paralelismo (predeterminado: detección automática, máximo 8)
hitstor backup --repo /path/to/repo --password "secret" --parallel 4 /data/to/backup

# Previsualizar lo que se respaldaría (no se escriben datos)
hitstor backup --repo /path/to/repo --password "secret" --dry-run /data/to/backup
```

### Listar instantáneas

```bash
hitstor snapshots --repo /path/to/repo --password "secret"

# Alias
hitstor ls --repo /path/to/repo --password "secret"
```

### Restauración

```bash
hitstor restore --repo /path/to/repo --password "secret" SNAPSHOT_ID /target/path
```

### Comparar instantáneas

```bash
hitstor diff --repo /path/to/repo --password "secret" SNAPSHOT_ID_1 SNAPSHOT_ID_2
```

### Uso de disco del repositorio

```bash
hitstor du --repo /path/to/repo --password "secret"
```

### Versión

```bash
hitstor version
```

### Verificar la integridad del repositorio

```bash
hitstor check --repo /path/to/repo --password "secret"
hitstor check --repo /path/to/repo --password "secret" --read-data  # verificación completa de blobs
```

### Olvidar (forget) instantáneas

```bash
# Olvidar por ID explícito (alias: hitstor rm)
hitstor forget --repo /path/to/repo --password "secret" SNAPSHOT_ID

# Olvidar según una política de retención
hitstor forget --repo /path/to/repo --password "secret" --keep-last 5 --keep-daily 7 --keep-weekly 4

# Olvidar + depurar (prune) en un solo comando
hitstor forget --repo /path/to/repo --password "secret" --keep-last 5 --prune

# Previsualizar lo que se olvidaría
hitstor forget --repo /path/to/repo --password "secret" --keep-last 3 --dry-run
```

Flags de retención: `--keep-last`, `--keep-hourly`, `--keep-daily`,
`--keep-weekly`, `--keep-monthly`, `--keep-yearly`, `--keep-tag`.
Flags de filtro: `--host`, `--path`.

### Depurar (recuperar almacenamiento)

```bash
hitstor prune --repo /path/to/repo --password "secret"
hitstor prune --repo /path/to/repo --password "secret" --dry-run
hitstor prune --repo /path/to/repo --password "secret" --max-repack-size 500MiB
```

---

## Arquitectura

```
cmd/hitstor/                    CLI + hitstor serve
cmd/hitstor-agent/              Remote backup agent
cmd/hitstor-scanner-builtin/    Built-in filesystem scanner plugin (ships with core)
api/proto/                      Plugin gRPC definitions (storage, notifier, hook, scanner)
internal/
  archiver/                     Local backup pipeline
  server/                       gRPC BackupService, sessions, multi-repo
  agent/                        Remote backup client + AgentService
  dashboard/                    Embedded web UI + HTTP API
  discovery/                    LAN agent auto-discovery
  sentinel/                     Ransomware telemetry (Gemini)
  repository/                   Content-addressable store
  plugin/                       HashiCorp go-plugin manager
  config/                       YAML config + repositories[]
  ...                           chunker, crypto, retention, metrics, etc.
```

Mapa completo: [docs/dev-guide/codebase-overview.md](docs/dev-guide/codebase-overview.md)

---

## Almacenamiento del repositorio

Al ejecutar `hitstor init`, se crea un repositorio con esta estructura de directorios:

```
<repo-root>/
├── config                     # Metadatos del repositorio (JSON sin cifrar)
├── keys/
│   └── <key_id>               # Clave maestra cifrada con la contraseña
├── data/
│   ├── ab/                    # Distribución por los 2 primeros caracteres hex
│   │   └── ab3f...pack_id     # Archivos de paquete cifrados
│   └── ...
├── index/
│   └── <index_id>             # Tablas de búsqueda blob→paquete cifradas
├── snapshots/
│   └── <snapshot_id>          # Metadatos de instantáneas cifrados
└── locks/
    └── <lock_id>              # Archivos de bloqueo exclusivo durante las operaciones
```

Todos los ID de objetos son hashes SHA-256 de 32 bytes que se muestran como 64 caracteres hexadecimales. Los ID de instantáneas que se muestran a los usuarios se truncan a 8 caracteres para facilitar la lectura.

### Archivo de configuración

Es el único archivo sin cifrar. Contiene el ID del repositorio y los parámetros de fragmentación:

```json
{
  "version": 1,
  "id": "64-hex-char-repository-id",
  "chunker_params": {
    "min_size": 262144,
    "max_size": 4194304,
    "avg_size": 1048576
  }
}
```

### Cifrado

Todos los datos (paquetes, índices, instantáneas) se cifran con **AES-256-GCM** antes de escribirse en disco:

1. **Derivación de clave**: su contraseña se procesa con **scrypt** (N=32768, r=8, p=1) y una sal aleatoria de 64 bytes para derivar una clave de 64 bytes.
2. **Clave maestra**: los primeros 32 bytes son la **clave de cifrado** (AES-256-GCM). Los últimos 32 bytes son la **clave de ID** (HMAC-SHA256 para el direccionamiento por contenido).
3. **Cifrado por blob**: cada blob de datos recibe un nonce aleatorio nuevo de 12 bytes. Formato de salida: `nonce || ciphertext || GCM-tag`.
4. **Detección de manipulación**: las etiquetas de autenticación de GCM detectan automáticamente cualquier corrupción o modificación.

La clave maestra se almacena en `keys/`, cifrada con su contraseña. La contraseña en sí nunca se escribe en disco.

### Cómo funcionan los respaldos

El pipeline de respaldo procesa sus archivos a través de estas etapas:

```
Files → Scan → Chunk (FastCDC) → Compress (Zstd) → Encrypt (AES-256-GCM) → Pack → Upload
```

1. **Escaneo**: recorre el sistema de archivos y compara las fechas de modificación con la caché SQLite para omitir los archivos sin cambios.
2. **Fragmentación**: divide el contenido de los archivos en fragmentos (chunks) de tamaño variable (256 KiB–4 MiB) mediante la fragmentación definida por contenido de FastCDC. El mismo contenido produce los mismos fragmentos, sin importar el nombre ni la ubicación del archivo.
3. **Deduplicación**: cada fragmento se procesa con HMAC-SHA256. Si el hash ya existe en el índice, el fragmento se omite.
4. **Compresión**: los fragmentos nuevos se comprimen con Zstd.
5. **Cifrado**: los fragmentos comprimidos se cifran con AES-256-GCM usando nonces aleatorios nuevos.
6. **Empaquetado**: los blobs cifrados se agrupan en archivos de paquete (~128 MiB cada uno) para un almacenamiento eficiente. Cada paquete contiene un encabezado que lista todos los blobs que incluye.
7. **Instantánea**: un archivo de instantánea registra la fecha y hora, el nombre del host, las rutas y un puntero al árbol del directorio raíz.

### Deduplicación definida por contenido

HitStor usa fragmentación definida por contenido para identificar y eliminar datos duplicados entre archivos y entre ejecuciones de respaldo. Esta sección explica en detalle cómo funciona.

#### Algoritmo de fragmentación: FastCDC con Gear hash

En lugar de dividir los archivos en desplazamientos fijos (cada 1 MiB), HitStor usa **FastCDC** (Fast Content-Defined Chunking) para encontrar los puntos de corte según el contenido real del archivo. Esto es fundamental, porque los fragmentos de tamaño fijo funcionan muy mal cuando se insertan o eliminan datos: todos los fragmentos posteriores al cambio se desplazan y ninguno coincide con el respaldo anterior.

FastCDC funciona deslizando un **hash rodante Gear** sobre los bytes del archivo. En cada posición, el hash se actualiza con una sola operación:

```
fingerprint = (fingerprint << 1) + gear_table[byte]
```

La `gear_table` es una tabla de búsqueda fija de 256 valores aleatorios de 64 bits, generada a partir de una semilla determinista (`0xFA57CDC`). Esto garantiza que el mismo contenido de archivo produzca siempre los mismos límites de fragmento, sin importar cuándo ni dónde se ejecute el respaldo.

**La detección de límites de fragmento** usa dos máscaras de bits con distintos niveles de exigencia:

| Región del archivo | Máscara usada | Efecto |
|-------------|-----------|--------|
| Entre el mínimo (256 KiB) y el promedio (1 MiB) | maskS (más bits activos) | Coincide con más facilidad: produce fragmentos más cortos |
| Entre el promedio (1 MiB) y el máximo (4 MiB) | maskL (menos bits activos) | Coincide con más dificultad: permite que los fragmentos crezcan |

Cuando el resultado del AND entre el hash rodante y la máscara activa es cero, se declara un límite de fragmento. Si no se encuentra ningún límite al llegar al tamaño máximo (4 MiB), el fragmento se corta ahí. Este enfoque de dos máscaras mantiene el tamaño promedio de fragmento cerca del objetivo configurado de 1 MiB.

**Por qué esto importa en la práctica:**

```
Original file:    [AAAA][BBBB][CCCC][DDDD]     ← 4 chunks

Insert "XY" near the start:
  Fixed-size:     [AAXY][AABB][BBCC][CCDD][DD]  ← ALL 5 chunks are different
  FastCDC:        [AAXY][AA][BBBB][CCCC][DDDD]  ← only 2 chunks changed, 2 reused
```

Con la fragmentación definida por contenido, las ediciones pequeñas solo afectan a los fragmentos cercanos al punto de edición. Los fragmentos más alejados conservan los mismos límites y el mismo contenido, por lo que se deduplican perfectamente.

#### Identificación de blobs

Cada fragmento recibe un ID único calculado así:

```
blob_id = HMAC-SHA256(IDKey, chunk_data)
```

La `IDKey` corresponde a los segundos 32 bytes de la clave maestra de 64 bytes (los primeros 32 bytes son la clave de cifrado). Usar HMAC en lugar de SHA-256 simple significa que los ID de blob son específicos de cada repositorio: el mismo contenido de archivo produce ID de blob distintos en repositorios distintos, lo que evita la filtración de información entre repositorios.

**Propiedad determinista:** el mismo contenido de fragmento dentro del mismo repositorio produce siempre el mismo ID de blob. Esto es lo que hace posible la deduplicación.

#### Verificación de deduplicación en tres niveles

Durante un respaldo, cada fragmento pasa por tres niveles de deduplicación antes de almacenarse:

```
Chunk computed
    │
    ▼
┌─────────────────────────────┐
│ Level 1: Session map        │  In-memory map of blob IDs added
│ (blobs added this backup)   │  during the current backup run.
└──────────┬──────────────────┘
           │ not found
           ▼
┌─────────────────────────────┐
│ Level 2: Files cache        │  SQLite database tracking file
│ (unchanged files)           │  metadata (inode, size, mtime).
└──────────┬──────────────────┘  If file is unchanged, reuse
           │ not found           cached chunk IDs without
           ▼                     re-reading the file at all.
┌─────────────────────────────┐
│ Level 3: Repository index   │  All blob IDs from all previous
│ (all previous backups)      │  backups, loaded into memory
└──────────┬──────────────────┘  when the repo is opened.
           │ not found
           ▼
      NEW BLOB → compress → encrypt → add to pack
```

Si el ID de blob se encuentra en **cualquiera** de los niveles, el fragmento se omite por completo: no se comprime, no se cifra y no se almacena. Solo se registra su ID como referencia en el árbol de la instantánea.

#### Procesamiento de blobs nuevos

Cuando un fragmento es realmente nuevo (no se encontró en ningún nivel de deduplicación):

1. **Compresión**: la compresión Zstd reduce el tamaño del fragmento (proporción típica: 40-60 % para texto; mínima para datos ya comprimidos).
2. **Cifrado**: AES-256-GCM con un nonce aleatorio nuevo de 12 bytes por blob.
3. **Empaquetado**: el blob cifrado se agrega al búfer del paquete actual. Cuando el búfer alcanza 128 MiB, el paquete se finaliza (el encabezado se cifra y se agrega), se guarda en el directorio `data/` y sus correspondencias de blobs se registran en el índice.

#### Deduplicación entre archivos

Los archivos no necesitan ser idénticos para que la deduplicación funcione. Cualquier contenido compartido entre archivos produce fragmentos coincidentes:

```
                    ┌──────────────────────────────────┐
                    │         Repository index          │
                    ├──────────┬──────────┬─────────────┤
                    │ Chunk A  │ Chunk B  │  Chunk C    │
                    │ (stored) │ (stored) │  (stored)   │
                    └────┬─────┴────┬─────┴──────┬──────┘
                         │          │            │
  Backup 1:              │          │            │
    server.log    →  [Chunk A] [Chunk B]         │
    server.log.1  →  [Chunk A] [Chunk C]         │
                     ▲ same content = same ID,   │
                       stored only once           │
                                                  │
  Backup 2 (next day):                           │
    server.log    →  [Chunk A] [Chunk B] [Chunk D]
                     ▲ found    ▲ found   ▲ NEW
                     in index   in index   → stored
```

En este ejemplo:
- **Respaldo 1** almacena 3 fragmentos únicos (A, B, C). El fragmento A aparece en ambos archivos, pero se almacena una sola vez.
- **Respaldo 2** solo almacena 1 fragmento nuevo (D). Los fragmentos A y B se encuentran en el índice del repositorio desde el respaldo 1 y se omiten.

#### Optimización de respaldos incrementales

Además de la deduplicación a nivel de fragmento, HitStor realiza la detección incremental del lado del servidor. Cuando comienza una sesión de respaldo, el servidor envía al agente los metadatos de los archivos de la instantánea anterior (ruta, mtime, tamaño, ID de contenido). El agente compara el mtime y el tamaño actuales de cada archivo con los valores anteriores; si ambos coinciden, reutiliza los ID de contenido en caché del archivo sin leerlo del disco.

Este enfoque no requiere estado local en el agente. Funciona después de reinstalaciones, en máquinas nuevas y cuando varios agentes respaldan en el mismo repositorio. El primer respaldo siempre es un escaneo completo; los respaldos siguientes son incrementales de forma predeterminada.

Esto significa que un respaldo incremental típico de un conjunto de datos de 1 TB en el que solo cambiaron 10 GB:
- **Omitirá** por completo ~990 GB de archivos sin cambios (los metadatos coinciden con la instantánea anterior)
- **Volverá a fragmentar** ~10 GB de archivos modificados
- **Almacenará** solo los fragmentos nuevos que no coincidan con ningún ID de blob existente

Para protegerse contra la corrupción silenciosa de archivos, puede configurar un intervalo de reescaneo completo periódico (por ejemplo, cada 7 días) que obliga al agente a leer todos los archivos sin importar los metadatos. También puede lanzar un reescaneo completo inmediato desde el dashboard.

### Límites de almacenamiento

HitStor no impone ningún límite artificial de tamaño. El repositorio crece hasta que se llena el disco subyacente.

La principal restricción práctica es el **uso de memoria del índice**. Al abrir un repositorio, todo el índice de blobs se carga en la RAM. Cada entrada del índice (`PackedBlob`) contiene un ID de blob de 32 bytes, el tipo de blob, el desplazamiento, la longitud y la longitud sin comprimir: aproximadamente **120 bytes por entrada** en memoria.

Como los fragmentos promedian ~1 MiB (`avg_size` de FastCDC), puede estimar la memoria del índice a partir de la cantidad de datos únicos almacenados:

| Datos únicos almacenados | Fragmentos (aprox.) | Uso de RAM del índice | Notas |
|--------------------|-------------------|-----------------|-------|
| 100 GB | ~100,000 | ~12 MB | Laptop, servidor pequeño |
| 1 TB | ~1,000,000 | ~120 MB | Servidor típico |
| 10 TB | ~10,000,000 | ~1.2 GB | Servidor grande; necesita RAM suficiente |
| 100 TB | ~100,000,000 | ~12 GB | Muy grande; requiere bastante RAM |

**"Datos únicos almacenados"** se refiere a los datos después de la deduplicación. Si respalda 1 TB a diario y solo cambia el 1 % entre ejecuciones, después de 30 instantáneas el repositorio contiene ~1.3 TB de datos únicos, no 30 TB. La deduplicación mantiene el índice mucho más pequeño que la suma de todas las instantáneas.

Los archivos de paquete tienen un tope de ~128 MiB cada uno. Un repositorio de 1 TB contiene aproximadamente 8,000 archivos de paquete, distribuidos en 256 subdirectorios (de `data/00/` a `data/ff/`). Esta estructura funciona bien en todos los sistemas de archivos Linux comunes:

| Sistema de archivos | Máx. de archivos por directorio | Tamaño máx. de archivo | Uso típico |
|------------|------------------------|---------------|-------------|
| ext4 | ~10 millones | 16 TB | La mayoría de los sistemas Linux |
| XFS | Miles de millones | 8 EB | Servidores de almacenamiento grandes |
| Btrfs | ~2^64 | 16 EB | Sistemas copy-on-write |

Para los casos de uso típicos (laptops, estaciones de trabajo, servidores con hasta unos pocos TB de datos), HitStor funciona con holgura. El diseño sigue el de herramientas como restic, que manejan repositorios de varios TB en producción.

### Variables de entorno

| Variable | Descripción |
|----------|-------------|
| `HITSTOR_REPO` | Ubicación del repositorio (igual que `--repo`) |
| `HITSTOR_PASSWORD` | Contraseña del repositorio (igual que `--password`) |
| `NO_COLOR` | Desactiva la salida en colores cuando tiene cualquier valor |

Las variables de entorno tienen prioridad sobre los valores del archivo de configuración, pero los flags de la CLI tienen prioridad sobre ellas.

---

## Salida en colores

HitStor usa colores ANSI en las terminales interactivas:
- **Verde** para mensajes de éxito
- **Amarillo** para advertencias
- **Rojo** para errores
- **Cian** para ID y metadatos

Los colores se desactivan automáticamente cuando la salida se canaliza (pipe) o se redirige. Para desactivarlos explícitamente, defina la variable de entorno `NO_COLOR`:

```bash
NO_COLOR=1 hitstor snapshots --repo /path/to/repo --password "secret"
```

---

## Autocompletado del shell

```bash
# Bash
hitstor completion bash > /etc/bash_completion.d/hitstor

# Zsh
hitstor completion zsh > "${fpath[1]}/_hitstor"

# Fish
hitstor completion fish > ~/.config/fish/completions/hitstor.fish
```

Los ID de instantáneas se completan dinámicamente al presionar Tab en los comandos `restore`, `forget` y `diff`.

---

## Configuración

HitStor lee la configuración desde un archivo YAML, variables de entorno y flags de la CLI.
Precedencia (de mayor a menor):

1. **Flags de la CLI** (`--repo`, `--password`, etc.)
2. **Variables de entorno** (`HITSTOR_REPO`, `HITSTOR_PASSWORD`)
3. **Archivo de configuración** (`~/.config/hitstor/config.yaml`)
4. **Valores predeterminados**

Use `--config` para indicar la ruta de un archivo de configuración personalizado.

### Gestión de la configuración

```bash
# Ver todos los valores de configuración y su origen
hitstor config show
hitstor config show --json

# Gestión moderna de múltiples repositorios
hitstor repo list
hitstor repo create myrepo /path/to/repo --password "secret"
hitstor --repo-name myrepo snapshots

# Tareas programadas (equivalentes al dashboard)
hitstor jobs list
hitstor jobs create --name daily-backup --repo-name myrepo --agent-name linux-agent --interval-hours 24

# Establecer un valor
hitstor config set repo /backups/hitstor
hitstor config set parallel 4

# Quitar un valor (volver al predeterminado)
hitstor config unset parallel

# Gestionar valores de lista (p. ej., patrones de exclusión)
hitstor config add exclude "*.tmp"
hitstor config add exclude "*.log"
hitstor config remove exclude "*.tmp"
hitstor config set exclude "*.tmp,*.log,*.cache"  # reemplaza la lista completa

# Asistente de configuración interactivo
hitstor config init

# Abrir el archivo de configuración en su editor
hitstor config edit

# Mostrar la ruta del archivo de configuración
hitstor config path
```

Todos los subcomandos de configuración admiten `--json` para obtener una salida legible por máquinas.
Por comodidad, el comando `config` también se puede invocar como `cfg`.

### Asistente de configuración interactivo

Ejecute `hitstor config init` para un recorrido guiado. Presione Enter en cualquier pregunta para aceptar el valor predeterminado que se muestra entre `[corchetes]`. Ctrl+C cancela sin escribir nada.

#### Ejemplo: backend local

```
$ hitstor config init
 _   _ _ _   ____  _
| | | (_) |_/ ___|| |_ ___  _ __
| |_| | | __\___ \| __/ _ \| '__|
|  _  | | |_ ___) | || (_) | |
|_| |_|_|\__|____/ \__\___/|_|   v0.1.0

Welcome to HitStor configuration setup!

Backend type (local/s3) [local]: local

Repository path: /backups/myserver
Password file (optional): /etc/hitstor/password
Parallel workers (0 = auto) [0]: 4
Exclude patterns (comma-separated, optional): *.tmp,*.cache,*.log

Configuration saved to /home/user/.config/hitstor/config.yaml
```

#### Ejemplo: backend S3

```
$ hitstor config init
 _   _ _ _   ____  _
| | | (_) |_/ ___|| |_ ___  _ __
| |_| | | __\___ \| __/ _ \| '__|
|  _  | | |_ ___) | || (_) | |
|_| |_|_|\__|____/ \__\___/|_|   v0.1.0

Welcome to HitStor configuration setup!

Backend type (local/s3) [local]: s3

Repository (s3:endpoint/bucket): s3:s3.amazonaws.com/my-backup-bucket
S3 access key: AKIAIOSFODNN7EXAMPLE
S3 secret key: wJalrXUtnFEMI/K7MDENG
S3 region [us-east-1]: eu-west-1
Password file (optional): /etc/hitstor/password
Parallel workers (0 = auto) [0]:
Exclude patterns (comma-separated, optional): *.tmp,*.cache

Configuration saved to /home/user/.config/hitstor/config.yaml
```

#### Volver a ejecutarlo con una configuración existente

Si ya existe un archivo de configuración, el asistente muestra los valores actuales como predeterminados y pregunta antes de sobrescribirlo:

```
$ hitstor config init
...
Config file already exists at /home/user/.config/hitstor/config.yaml
Overwrite? (y/N): y

Backend type (local/s3) [local]: local

Repository path [/backups/myserver]: /backups/newserver
Password file (optional) [/etc/hitstor/password]:
Parallel workers (0 = auto) [4]: 8
Exclude patterns (comma-separated, optional) [*.tmp,*.cache,*.log]:

Configuration saved to /home/user/.config/hitstor/config.yaml
```

#### Verificar su configuración

Después de ejecutar el asistente, verifique el resultado con `config show`:

```
$ hitstor config show
KEY               VALUE                     SOURCE
────────────────  ────────────────────────  ───────
repo              /backups/myserver         config
password_file     /etc/hitstor/password     config
s3_access_key     (not set)                 default
s3_secret_key     (not set)                 default
s3_region         us-east-1                 default
exclude           *.tmp, *.cache, *.log     config
parallel          4                         config
```

### Claves configurables

| Clave | Tipo | Predeterminado | Descripción |
|-----|------|---------|-------------|
| `repo` | string | | Ubicación del repositorio (ruta o s3:endpoint/bucket) |
| `password_file` | string | | Ruta al archivo de contraseña |
| `s3_access_key` | string | | Clave de acceso de S3 (oculta en `show`) |
| `s3_secret_key` | string | | Clave secreta de S3 (oculta en `show`) |
| `s3_region` | string | us-east-1 | Región de S3 |
| `exclude` | list | | Patrones de exclusión |
| `parallel` | int | 0 (auto) | Número de workers en paralelo |
| `serve_port` | int | 8420 | Puerto de escucha del servidor gRPC |
| `serve_tls_cert` | string | | Ruta al archivo de certificado TLS del servidor |
| `serve_tls_key` | string | | Ruta al archivo de clave privada TLS del servidor |
| `agent_server` | string | | Dirección del servidor remoto (host:port) |
| `agent_api_key` | string | | Clave de API del agente (oculta en `show`) |
| `agent_ca_cert` | string | | Ruta al certificado de la CA para TLS |
| `agent_hostname` | string | | Reemplaza el nombre de host que se envía al servidor |
| `agent_paths` | list | | Rutas de respaldo predeterminadas |
| `agent_excludes` | list | | Patrones de exclusión predeterminados del agente |
| `plugin_dir` | string | | Directorio donde buscar los binarios de plugins |
| `hook_plugins` | list | | Nombres de plugins de hooks que se cargan para los eventos del ciclo de vida previo/posterior al respaldo |
| `fs_scanner_plugin` | string | | Nombre del plugin escáner del sistema de archivos (`builtin`, `db`, etc.) |
| `serve_advertise_addr` | string | | Dirección que usan los agentes para llegar al servidor (NAT/IP pública) |
| `update_check` | bool | true | Buscar nuevas versiones en GitHub |
| `update_url` | string | | Reemplaza la URL de búsqueda de versiones |

### Ejemplo de archivo de configuración

```yaml
repo: /backups/hitstor
password_file: /etc/hitstor/password
parallel: 4
exclude:
  - "*.tmp"
  - "*.cache"

# Servidor de respaldo remoto (opcional)
serve_port: 8420
serve_tls_cert: /etc/ssl/hitstor/server.crt
serve_tls_key: /etc/ssl/hitstor/server.key

# Agente de respaldo remoto (opcional)
agent_server: backup-server.internal:8420
agent_api_key: abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789
agent_ca_cert: /etc/ssl/hitstor/ca.pem
agent_hostname: my-server
agent_excludes:
  - "*.log"
  - "*.cache"
```

---

## Servidor de respaldo remoto

HitStor incluye un servidor gRPC que acepta respaldos de agentes remotos a través de TLS. Esto permite respaldar de forma centralizada varias máquinas Linux y Windows en un único repositorio.

### Arquitectura

El sistema usa un diseño de **agente grueso** (thick agent):

```
Agent (remote machine)                    Server (hitstor serve)
┌─────────────────────┐                  ┌──────────────────────────┐
│ Scan filesystem     │                  │ Verify API key           │
│ Chunk (FastCDC)     │    gRPC/TLS      │ Check dedup (repo index) │
│ Compute blob IDs    │ ──────────────►  │ Encrypt (AES-256-GCM)    │
│ Compress (Zstd)     │                  │ Pack into 128 MiB packs  │
│ Send new chunks     │                  │ Build trees & snapshot   │
└─────────────────────┘                  │ Save index               │
                                         └──────────────────────────┘
```

- El **agente** se encarga localmente del escaneo del sistema de archivos, la fragmentación, el cálculo de los ID de blob y la compresión.
- Solo se envían por la red los **fragmentos comprimidos nuevos**; los fragmentos que ya están en el repositorio se omiten.
- El **servidor** se encarga de la verificación de deduplicación, el cifrado, el armado de paquetes, la gestión del índice y la creación de instantáneas.
- La **clave de cifrado nunca sale del servidor**: los agentes reciben solo la IDKey para identificar los blobs.

### Iniciar el servidor

```bash
# Iniciar con un certificado TLS autofirmado generado automáticamente
hitstor serve --repo /path/to/repo --password "secret"

# Iniciar con un certificado TLS propio
hitstor serve --repo /path/to/repo --password "secret" \
  --tls-cert /etc/ssl/server.crt \
  --tls-key /etc/ssl/server.key

# Puerto personalizado (predeterminado: 8420)
hitstor serve --repo /path/to/repo --password "secret" --port 9000
```

El servidor obtiene un bloqueo exclusivo sobre el repositorio y espera conexiones de los agentes.

### Gestión de agentes

Cada máquina remota necesita una clave de API registrada para autenticarse con el servidor.

```bash
# Registrar un agente nuevo (muestra la clave de API una sola vez)
hitstor serve agent add myserver --repo /path/to/repo --password "secret"
# Output:
#   Agent "myserver" registered.
#   API Key: a1b2c3d4...64-hex-chars
#   Save this key — it cannot be displayed again.

# Listar todos los agentes registrados
hitstor serve agent list --repo /path/to/repo --password "secret"
# Output:
#   NAME                  CREATED               LAST BACKUP
#   myserver              2026-02-20 10:30:00   never

# Eliminar un agente (revoca su clave de API)
hitstor serve agent remove myserver --repo /path/to/repo --password "secret"
```

Los registros de agentes se almacenan cifrados en el directorio del repositorio (`agents.json`), protegidos por la clave de cifrado del repositorio.

### Protocolo gRPC

El servidor expone 5 RPC sobre TLS:

| RPC | Tipo | Descripción |
|-----|------|-------------|
| `Ping` | Unario | Verificación de estado: devuelve la versión del servidor y el ID del repositorio |
| `BeginSession` | Unario | Autentica con la clave de API y recibe la IDKey, los parámetros del fragmentador y los metadatos de la instantánea anterior para la detección incremental |
| `CheckBlobs` | Unario | Comprueba por lotes qué ID de blob ya tiene el servidor (deduplicación) |
| `UploadChunks` | Streaming del cliente | Envía al servidor un flujo de fragmentos comprimidos |
| `FinalizeBackup` | Unario | Envía el árbol de archivos; el servidor crea la instantánea y devuelve un resumen |
| `GetSchedule` | Unario | Obtiene la programación de respaldos configurada para el agente y la hora de la próxima ejecución |

### Flujo del respaldo

1. **BeginSession**: el agente se autentica con su clave de API. El servidor devuelve la IDKey (para calcular los ID de blob), los parámetros del fragmentador y los metadatos de los archivos de la instantánea anterior (rutas, mtimes, tamaños, ID de contenido) para la detección incremental.
2. **Escaneo y fragmentación**: el agente escanea su sistema de archivos y omite los archivos cuyo mtime y tamaño coinciden con los metadatos de la instantánea anterior. Los archivos modificados y nuevos se fragmentan con FastCDC, calculando `blob_id = HMAC-SHA256(IDKey, chunk_data)`.
3. **CheckBlobs**: el agente envía un lote de ID de blob para comprobar. El servidor devuelve solo los ID que no están en el repositorio (los que hay que subir).
4. **UploadChunks**: el agente comprime los fragmentos nuevos con Zstd y los envía en flujo al servidor. El servidor cifra cada fragmento y los empaqueta.
5. **FinalizeBackup**: el agente envía el árbol completo de archivos y directorios. El servidor crea los blobs de árbol y la instantánea, guarda el índice y devuelve un resumen.

---

## Agente de respaldo remoto

El binario `hitstor-agent` se ejecuta en las máquinas remotas y se conecta a un servidor HitStor para realizar los respaldos.

### Instalación

**Con el instalador interactivo** (recomendado para Linux):

```bash
sudo ./hitstor-agent-X.Y.Z-linux-amd64.run
```

El asistente configura la dirección del servidor, la clave de API y las rutas de respaldo, e inicia el servicio. Consulte [Instalación rápida en Linux](docs/installation/linux-installer.md) para más detalles.

**Compilar desde el código fuente:**

```bash
make agent                    # Linux amd64
make agent-arm64              # Linux arm64
make agent-windows            # Windows amd64
make agent-windows-arm64      # Windows arm64
make agent-all                # Todas las plataformas

# Instalar (Linux)
sudo cp bin/hitstor-agent /usr/local/bin/
```

Para la configuración en Windows (instalación, servicio de Windows, limitaciones conocidas), consulte [docs/installation/windows-agent.md](docs/installation/windows-agent.md).

### Inicio rápido

```bash
# Probar la conectividad
hitstor-agent ping --server backup-server:8420 --ca-cert /path/to/server.crt

# Ejecutar un respaldo
hitstor-agent backup /etc /home /var/lib \
  --server backup-server:8420 \
  --api-key <64-char-hex-key> \
  --ca-cert /path/to/server.crt

# Con exclusiones y etiquetas
hitstor-agent backup /home \
  --server backup-server:8420 \
  --api-key <key> \
  --exclude "*.log" --exclude "*.cache" \
  --tag production --tag daily
```

### Configuración

En lugar de pasar flags cada vez, configure el agente mediante el archivo de configuración o variables de entorno:

```yaml
# ~/.config/hitstor/config.yaml
agent_server: backup-server.internal:8420
agent_api_key: abcdef0123456789...
agent_ca_cert: /etc/ssl/hitstor/ca.pem
agent_hostname: my-server
agent_excludes:
  - "*.log"
  - "*.cache"
```

Variables de entorno: `HITSTOR_AGENT_SERVER`, `HITSTOR_AGENT_API_KEY`.

### Respaldos programados con systemd

Se incluyen archivos de unidad de plantilla en `docs/`:

```bash
# Instalar el servicio y el temporizador
sudo cp packaging/systemd/hitstor-agent.service /etc/systemd/system/
sudo cp packaging/systemd/hitstor-agent.timer /etc/systemd/system/

# Editar el servicio para definir la dirección del servidor, la clave de API y las rutas
sudo systemctl edit hitstor-agent.service

# Habilitar el temporizador diario
sudo systemctl enable --now hitstor-agent.timer

# Ver el estado del respaldo
sudo systemctl status hitstor-agent.service
sudo journalctl -u hitstor-agent.service
```

### Claves de configuración del agente

| Clave | Tipo | Descripción |
|-----|------|-------------|
| `agent_server` | String | Dirección del servidor (host:port) |
| `agent_api_key` | String | Clave de API del agente (64 caracteres hex) |
| `agent_ca_cert` | String | Ruta al certificado de la CA para TLS |
| `agent_hostname` | String | Reemplaza el nombre de host en las instantáneas |
| `agent_paths` | List | Rutas de respaldo predeterminadas |
| `agent_excludes` | List | Patrones de exclusión predeterminados |

---

## Dashboard web

El comando `hitstor serve` inicia un dashboard HTTP junto al servidor gRPC. Ofrece una interfaz web para supervisar los agentes, explorar las instantáneas y ver el uso del almacenamiento.

> La interfaz usa el sistema de marca de HitStor: superficies azul marino, acentos naranjas e isotipo de candado. Consulte [docs/design/branding.md](docs/design/branding.md).

```
hitstor serve
  ├─ gRPC :8420 ── agents (backup traffic)
  └─ HTTP :8421 ── browser (dashboard)
```

### Acceso

Abra `http://server-address:8421` en un navegador. El dashboard se actualiza automáticamente mediante sondeo con htmx (cada 30 segundos).

### Páginas

| Página | URL | Descripción |
|------|-----|-------------|
| Resumen | `/` | Cantidad de agentes, cantidad de instantáneas, hora del último respaldo, cantidad de respaldos activos |
| Respaldos | `/backups` | Tareas de respaldo de sistemas de archivos |
| Agentes | `/agents` | Lista de agentes con descubrimiento e insignias de estado (OK / stale / never) |
| Detalle del agente | `/agents/{name}` | Programación, rutas, historial de respaldos |
| Detalle de la instantánea | `/snapshots/{id}` | Metadatos de la instantánea, explorador del árbol de archivos, restauración con progreso |
| Almacenamiento | `/storage` | Gestión de repositorios, instantáneas, verificación, depuración, gráficos de crecimiento |
| Restauraciones | `/restores` | Sesiones de restauración activas y completadas |
| Configuración | `/settings` | TLS, red, integraciones, notificaciones |

### API JSON

| Endpoint | Descripción |
|----------|-------------|
| `GET /api/agents` | Lista de agentes con estado, cantidad de instantáneas e información de programación |
| `POST /api/agents/{name}/schedule` | Define la programación de respaldos del agente (JSON: enabled, interval_hours) |
| `POST /api/agents/{name}/repo` | Define el repositorio de destino del agente |
| `GET /api/snapshots?host=&limit=` | Instantáneas (las más recientes primero; límite predeterminado 50) |
| `GET /api/stats` | Estadísticas del repositorio (instantáneas, paquetes, índices, tamaño total) |
| `GET /api/repos` | Lista los repositorios configurados con sus estadísticas |
| `POST /api/repos` | Crea un repositorio nuevo (local o S3) |
| `DELETE /api/repos/{name}` | Desconecta un repositorio |
| `POST /api/repos/{name}/check` | Inicia una verificación de integridad (devuelve el ID de la operación) |
| `POST /api/repos/{name}/prune` | Inicia una operación de depuración (devuelve el ID de la operación) |
| `GET /api/repos/{name}/growth` | Datos de crecimiento del almacenamiento para gráficos |
| `POST /api/snapshots/{id}/restore` | Inicia una restauración (JSON: agent_name, target_path, includes) |
| `GET /api/snapshots/{id}/tree?path=` | Explora el árbol de archivos de la instantánea (carga diferida de directorios) |
| `GET /api/restore/{id}/events` | Flujo SSE del progreso de la restauración |
| `GET /api/restores` | Lista todas las sesiones de restauración (activas y completadas) |
| `GET /api/operations/{id}/events` | Flujo SSE del progreso de la operación |
| `GET /api/active-backups` | Cantidad y detalles de todas las sesiones de respaldo activas |
| `GET /api/backup-progress/events` | Flujo SSE del progreso de los respaldos (todas las sesiones, cada 2 s) |
| `GET /metrics` | Métricas compatibles con Prometheus (sin autenticación) |

### Métricas de Prometheus

El endpoint `/metrics` expone las métricas del servidor en el formato de texto de exposición de Prometheus. Nunca requiere autenticación, para que Prometheus pueda recopilarlas sin credenciales.

Las métricas expuestas incluyen:
- **Medidores (gauges)**: `hitstor_snapshots_total`, `hitstor_packs_total`, `hitstor_indexes_total`, `hitstor_repository_size_bytes`, `hitstor_active_sessions`, `hitstor_registered_agents`, `hitstor_agents_ok`, `hitstor_agents_stale`, `hitstor_agents_never`
- **Contadores**: `hitstor_backups_completed_total`, `hitstor_backups_failed_total`, `hitstor_bytes_received_total`, `hitstor_chunks_received_total`, `hitstor_checks_total`, `hitstor_check_errors_total`, `hitstor_prunes_total`, `hitstor_prune_freed_bytes_total`
- **Por agente**: `hitstor_agent_last_backup_timestamp_seconds{agent="name"}`, `hitstor_agent_schedule_enabled{agent="name"}`, `hitstor_agent_schedule_interval_hours{agent="name"}`

### Alertas

HitStor puede enviar notificaciones cuando fallan los respaldos o cuando los agentes quedan inactivos (stale). Las notificaciones se entregan mediante el notificador de webhook integrado y/o plugins notificadores.

```bash
# Notificador de webhook integrado
hitstor config set alert_webhook_url "https://hooks.slack.com/services/..."
hitstor config set alert_stale_threshold 25  # horas (predeterminado)

# Plugins notificadores (se cargan junto con el webhook integrado)
hitstor config add alert_notifier_plugins slack
hitstor config set plugin_dir /usr/local/lib/hitstor/plugins
```

Los eventos se envían como solicitudes POST con JSON:

| Evento | Disparador |
|-------|---------|
| `backup_failed` | El agente se desconecta durante la subida o falla la finalización |
| `agent_stale` | No hubo respaldos dentro del umbral configurado (se comprueba cada 5 min) |

Los agentes que nunca han realizado un respaldo no disparan alertas de inactividad. Las alertas de inactividad incluyen un período de espera para evitar notificaciones repetidas: un agente solo vuelve a generar una alerta después de que transcurre la duración del umbral.

| Clave | Tipo | Predeterminado | Descripción |
|-----|------|---------|-------------|
| `alert_webhook_url` | String | *(vacío)* | URL del webhook para las notificaciones de alerta (vacío = desactivado) |
| `alert_stale_threshold` | Int | `25` | Horas antes de que un agente se considere inactivo |
| `alert_notifier_plugins` | List | *(vacío)* | Nombres de los plugins notificadores que se cargan (separados por comas) |

### Programación

HitStor admite la programación de respaldos gestionada por el servidor. Configure una programación por agente desde el dashboard y luego ejecute el agente en modo daemon para que consulte al servidor y ejecute los respaldos automáticamente.

#### Habilitar una programación

1. Abra el dashboard y vaya a la página de detalle de una tarea de respaldo
2. Marque "Habilitar respaldos programados"
3. Elija un modo de programación:
   - **Intervalo**: repetición simple "cada X horas"
   - **Programación**: generador visual con opciones predefinidas Diaria, Semanal o Mensual; seleccione la hora, los días de la semana o el día del mes mediante listas desplegables y botones de alternancia (no se necesita sintaxis cron)
   - **Avanzado**: entrada directa de una expresión cron con validación en vivo y vista previa de la próxima ejecución
4. Haga clic en "Guardar programación"

El generador visual de programación detecta automáticamente las expresiones cron existentes y las muestra en el modo correspondiente. Las expresiones complejas que no corresponden a un patrón visual se muestran en el modo Avanzado.

#### Modo daemon del agente

En lugar de depender de los temporizadores de systemd, ejecute el agente como un daemon de larga duración que consulta al servidor para obtener su programación:

```bash
# Ejecutar en modo daemon (consulta al servidor cada 5 minutos de forma predeterminada)
hitstor-agent daemon --server backup-server:8420 --api-key <key>

# Intervalo de consulta personalizado
hitstor-agent daemon --server backup-server:8420 --api-key <key> --poll-interval 2m
```

El daemon:
- Consulta al servidor la programación configurada cada 5 minutos (configurable con `--poll-interval`)
- Ejecuta un respaldo cuando llega la hora programada (`next_run = last_backup + interval_hours`)
- Si el agente nunca ha realizado un respaldo y hay una programación habilitada, el primer respaldo se ejecuta de inmediato
- Usa `agent_paths` y `agent_excludes` del archivo de configuración como rutas de respaldo
- En Windows, el servicio de Windows se ejecuta automáticamente en modo daemon

Los agentes existentes que usan temporizadores de systemd o el Programador de tareas de Windows siguen funcionando sin cambios. El modo daemon es opcional.

### Autenticación

De forma predeterminada, el dashboard está abierto (no requiere inicio de sesión). Para habilitar la protección con contraseña:

```bash
hitstor config set serve_dashboard_password "your-secret"
```

Cuando está definida, todas las páginas y los endpoints de la API requieren autenticación mediante una cookie de sesión. La página de inicio de sesión está en `/login`.

### Claves de configuración del dashboard

| Clave | Tipo | Predeterminado | Descripción |
|-----|------|---------|-------------|
| `serve_dashboard_port` | Int | `8421` | Puerto de escucha del dashboard HTTP |
| `serve_dashboard_password` | String | *(vacío)* | Contraseña de inicio de sesión del dashboard (vacío = acceso abierto) |

### Flags

```bash
hitstor serve --dashboard-port 9090   # reemplaza el puerto
```

---

## Backends de plugins

HitStor admite backends de almacenamiento como plugins externos mediante el framework [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin). Los plugins son binarios independientes que se comunican con hitstor a través de gRPC.

### Uso

```bash
# Inicializar un repositorio con un backend de plugin
hitstor init --repo plugin:azure/myaccount/mycontainer --password "secret"

# Respaldar con un backend de plugin
hitstor backup --repo plugin:azure/myaccount/mycontainer --password "secret" /data
```

El esquema de URI `plugin:<name>[/<uri>]` corresponde a un binario llamado `hitstor-backend-<name>`.

### Descubrimiento de plugins

1. Buscar en la ruta de configuración `plugin_dir` (si está configurada)
2. Buscar en `$PATH`

### Ejemplo integrado: backend en memoria

Se incluye un plugin en memoria para pruebas:

```bash
make plugin-memory   # compila bin/hitstor-backend-memory
```

### Escribir plugins

Un binario de plugin implementa el servicio gRPC `StorageBackend` definido en [`api/proto/hitstor/plugin/v1/storage.proto`](api/proto/hitstor/plugin/v1/storage.proto). La función `main()` del plugin llama a `plugin.ServeStorageBackend(impl)` con una implementación de `backend.Backend`. Consulte [`cmd/hitstor-backend-memory/`](cmd/hitstor-backend-memory/) para ver un ejemplo completo.

### Plugins notificadores

Los plugins notificadores entregan notificaciones de alerta por canales personalizados (Slack, PagerDuty, correo electrónico, etc.). Son binarios independientes llamados `hitstor-notifier-<name>` que implementan el servicio gRPC `Notifier` definido en [`api/proto/hitstor/plugin/v1/notifier.proto`](api/proto/hitstor/plugin/v1/notifier.proto).

```bash
# Configurar plugins notificadores
hitstor config add alert_notifier_plugins slack
hitstor config set plugin_dir /usr/local/lib/hitstor/plugins
```

La función `main()` de un plugin notificador llama a `plugin.ServeNotifier(impl)` con una implementación de `notifier.Notifier`. Consulte [`cmd/hitstor-notifier-log/`](cmd/hitstor-notifier-log/) para ver un ejemplo completo.

### Plugins de hooks

Los plugins de hooks se ejecutan en los eventos del ciclo de vida previo/posterior al respaldo. Son binarios independientes llamados `hitstor-hook-<name>` que implementan el servicio gRPC `Hook` definido en [`api/proto/hitstor/plugin/v1/hook.proto`](api/proto/hitstor/plugin/v1/hook.proto).

```bash
# Configurar plugins de hooks
hitstor config add hook_plugins log
hitstor config set plugin_dir /usr/local/lib/hitstor/plugins
```

Los hooks se disparan en dos niveles:
- **Del lado del servidor** (`hitstor serve`): `PreBackup` se dispara después de la autenticación del agente en `BeginSession`; `PostBackup` se dispara después de guardar la instantánea en `FinalizeBackup`.
- **Respaldo local** (`hitstor backup`): `PreBackup` se dispara antes de que comience el respaldo; `PostBackup` se dispara después de un respaldo exitoso.

Los errores de `PreBackup` cancelan el respaldo. Los errores de `PostBackup` se registran, pero no hacen fallar el respaldo.

La función `main()` de un plugin de hook llama a `plugin.ServeHook(impl)` con una implementación de `hook.Hook`. Consulte [`cmd/hitstor-hook-log/`](cmd/hitstor-hook-log/) para ver un ejemplo completo.

### Plugins FilesystemScanner

Los plugins FilesystemScanner reemplazan el recorrido de directorios integrado por una lógica de escaneo personalizada. Son binarios independientes llamados `hitstor-scanner-<name>` que implementan el servicio gRPC `FilesystemScanner` definido en [`api/proto/hitstor/plugin/v1/scanner.proto`](api/proto/hitstor/plugin/v1/scanner.proto).

```bash
# Configurar un plugin escáner (solo un escáner activo a la vez)
hitstor config set fs_scanner_plugin builtin
hitstor config set plugin_dir /usr/local/lib/hitstor/plugins
```

A diferencia de los plugins notificadores y de hooks (que admiten varios a la vez), solo puede haber **un** plugin escáner activo a la vez. Cuando está definido, reemplaza el recorrido predeterminado del sistema de archivos tanto en los respaldos locales (`hitstor backup`) como en los respaldos del agente (`hitstor-agent backup`).

El RPC `Scan` usa streaming del lado del servidor para devolver las entradas de archivo una a una, lo que lo hace eficiente con árboles de directorios grandes.

La función `main()` de un plugin escáner llama a `plugin.ServeFilesystemScanner(impl)` con una implementación de `scanner.Scanner`. Consulte [`cmd/hitstor-scanner-builtin/`](cmd/hitstor-scanner-builtin/) para ver un ejemplo completo.

Para consultar la documentación detallada sobre el desarrollo de plugins, vea [`docs/reference/plugin-development.md`](docs/reference/plugin-development.md).

### Eliminado: cargas de trabajo sin agente

Los respaldos de Microsoft 365, Google Workspace, bases de datos y máquinas virtuales en la nube (incluido VMware/vSphere)
se eliminaron de forma permanente en la v2.0.0. HitStor es un producto de respaldo y
restauración de sistemas de archivos para Windows y Linux. Consulte
[docs/admin-guide/roadmap.md](docs/admin-guide/roadmap.md).

---

## Pruebas de rendimiento

Tiempos reproducibles de respaldo y restauración (con comparación opcional con restic):

```bash
./benchmarks/run.sh --profile small
make bench PROFILE=medium
```

Consulte [benchmarks/README.es.md](benchmarks/README.es.md) y [docs/dev-guide/benchmarks.md](docs/dev-guide/benchmarks.md).

## Desarrollo

### Requisitos previos

- Go 1.26+ (consulte `go.mod`)
- Docker y Docker Compose (para las pruebas de integración con S3/MinIO)

### Compilar y probar

```bash
# Compilar
go build ./cmd/...

# Ejecutar todas las pruebas
go test ./...

# Análisis estático (vet)
go vet ./...

# Iniciar MinIO para las pruebas de integración con S3
docker compose up -d
```

### Convenciones del proyecto

- **Pruebas primero**: TDD con pruebas basadas en tablas.
- **Encapsulado de errores**: `fmt.Errorf("package: action: %w", err)`.
- **Registro estructurado**: `log/slog` con salida JSON en producción.
- **Sin `utils/` ni `helpers/`**: las funciones viven en el paquete de su dominio.

---

## Licencia

Copyright (C) 2026 HooperITS

HitStor es software libre, distribuido bajo la [GNU Affero General Public License v3.0](LICENSE) (AGPL-3.0-only).
Si modifica HitStor y lo pone a disposición de usuarios a través de una red (por ejemplo, el dashboard web),
debe ofrecerles el código fuente correspondiente de su versión modificada.

---

<div align="center">

<sub>Recursos de marca: <code>assets/icon.png</code> · <a href="docs/design/branding.md">sistema de diseño</a></sub>

</div>
