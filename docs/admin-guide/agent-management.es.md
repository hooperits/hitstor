# Administración de agentes

HitStor admite dos métodos para registrar agentes de respaldo: **administrado desde el dashboard** (recomendado) y **basado en la CLI** (heredado). Los agentes administrados desde el dashboard usan un flujo basado en tokens en el que el servidor se conecta al agente, lo autentica y lo configura automáticamente. Los agentes basados en la CLI usan una clave de API que se distribuye manualmente.

## Agentes administrados desde el dashboard (recomendado) { #dashboard-managed-agents-recommended }

Los agentes administrados desde el dashboard se agregan directamente desde el dashboard web. El servidor se conecta al agente por gRPC, lo autentica con el token del agente y le envía automáticamente la clave de API y la dirección del servidor.

### Requisitos previos { #prerequisites }

1. Instale el agente en el equipo remoto (consulte [Despliegue de agentes](agent-deployment.md))
2. El agente genera un token la primera vez que se inicia; anote el token (`hitstor-agent show-id`) y la dirección IP del equipo
3. Asegúrese de que el puerto **8422** (puerto de escucha gRPC del agente) sea accesible desde el servidor

### Agregar un agente desde el dashboard { #add-an-agent-from-the-dashboard }

1. Abra el dashboard de HitStor en `https://SERVER_IP:8421` (administradores)
2. Vaya a **Agentes** y haga clic en **+ Agregar agente**
3. Elija **Escaneo de red** para escanear una subred local, o **Entrada manual** para escribir la dirección IP del agente y hacer clic en **Conectar**
4. Haga clic en **Registrar** en el agente encontrado y complete **Registrar agente**:
    - **Nombre del agente**: identificador descriptivo (por ejemplo, `web-server-01`); letras, números, guiones y guiones bajos
    - **Dirección del agente**: `host:port`, que se completa desde el escaneo (por ejemplo, `192.168.1.50:8422`)
    - **Token del agente**: token hexadecimal de 64 caracteres del agente (`hitstor-agent show-id` en el equipo del agente)
5. Haga clic en **Registrar agente**

Consulte [Descubrimiento de agentes](agent-discovery.md) para ver el flujo de escaneo de red.

El servidor se conecta al agente, verifica el token y le envía una clave de API y la dirección del servidor, que el agente escribe en su `agent.env`. No es necesario copiar ninguna clave manualmente. Registrar de nuevo un nombre existente vuelve a registrar ese agente (nueva clave de API) y conserva sus tareas.

### Elegir qué respaldar { #choose-what-to-back-up }

Lo que respalda un agente se define en cada tarea de respaldo, no en el agente. En **Respaldos → + Nueva tarea de respaldo**, el árbol de selección explora en vivo, por gRPC, el sistema de archivos del agente (y sus plugins instalados); marque las carpetas que desea respaldar. Consulte [Tareas de respaldo](backup-jobs.md).

### Eliminar un agente { #remove-an-agent }

Desde la lista de Agentes o la página del agente (**Eliminar**, administradores), o mediante la CLI:

```bash
hitstor serve agent remove AGENT_NAME
```

Esto revoca la clave de API del agente. El agente ya no podrá conectarse al servidor. Las instantáneas (snapshots) existentes creadas por el agente no se eliminan. El dashboard pregunta si se conservan las tareas de respaldo del agente; la CLI las elimina salvo que agregue `--keep-jobs`.

## Agentes basados en la CLI (heredado) { #cli-based-agents-legacy }

Los agentes basados en la CLI se registran desde la línea de comandos del servidor. La clave de API debe copiarse manualmente al equipo del agente.

### Registrar un agente { #register-an-agent }

```bash
hitstor serve agent add AGENT_NAME
```

Si tiene éxito, el comando imprime una clave de API hexadecimal de 64 caracteres:

```
Agent "web-server-01" registered.
API Key: a3f8c1d9e4b7...6f2a0e5d

Save this key — it cannot be displayed again.
```

Copie esta clave en el archivo `/etc/hitstor/agent.env` del agente como `HITSTOR_AGENT_API_KEY`, junto a `HITSTOR_AGENT_SERVER` (consulte [Despliegue de agentes](agent-deployment.md#manual-cli-registration)).

### Listar agentes { #list-agents }

Vea todos los agentes registrados (tanto administrados como basados en la CLI):

```bash
hitstor serve agent list
```

Salida:

```
NAME                  CREATED               LAST BACKUP           MANAGED   SUSPENDED   ADDR
web-server-01         2026-02-01 10:12:40   2026-02-21 03:00:12   yes       no          192.168.1.50:8422
db-primary            2026-02-01 10:20:05   2026-02-21 03:15:08   no        no          -
staging-app           2026-02-03 09:41:17   never                 yes       yes         192.168.1.52:8422
```

`--all-repos` lista los agentes de todos los repositorios configurados; `--json` imprime los registros en JSON.

### Eliminar un agente { #remove-an-agent_1 }

```bash
hitstor serve agent remove AGENT_NAME
```

Para conservar las tareas de respaldo del agente (por ejemplo, para registrarlo de nuevo con el mismo nombre):

```bash
hitstor serve agent remove AGENT_NAME --keep-jobs
```

Eliminar un agente nunca elimina sus instantáneas; para eso use [`hitstor forget`](../user-guide/retention.md).

## Estado del agente { #agent-status }

La página **Agentes** del dashboard muestra el estado de cada agente:

| Estado | Significado |
|--------|-------------|
| `ok` | El agente realizó un respaldo en las últimas 25 horas |
| `stale` (inactivo) | El agente no ha realizado un respaldo en las últimas 25 horas |
| `never` (nunca) | El agente está registrado, pero nunca ha completado un respaldo |
| `suspended` (suspendido) | El agente está suspendido (por un operador o por Sentinel); no puede respaldar |

Las alertas por webhook usan en cambio el umbral configurable `alert_stale_threshold` (predeterminado: 25 horas); consulte [Monitoreo](monitoring.md).

## Token del agente (administrado desde el dashboard) { #agent-token-dashboard-managed }

Los agentes administrados desde el dashboard se autentican con un **token**: una cadena hexadecimal de 64 caracteres (256 bits de entropía) que se genera la primera vez que se inicia el agente (los instaladores inician el agente, así que existe justo después de la instalación).

**Propiedades del token:**

- Se genera automáticamente la primera vez que se inicia el agente y se guarda en `/etc/hitstor/agent-token` (modo 0600; `C:\ProgramData\HitStor\agent-token` en Windows)
- Puede obtenerse en cualquier momento con `sudo hitstor-agent show-id`
- Se almacena cifrado (AES-256-GCM) en el servidor, en el registro de agentes
- El servidor lo usa para autenticar las llamadas gRPC al agente (registro, exploración, instalación de plugins, activación inmediata)

## Seguridad de la clave de API (basado en la CLI) { #api-key-security-cli-based }

Los agentes basados en la CLI se autentican ante el servidor con una **clave de API hexadecimal de 64 caracteres** (256 bits de entropía).

**Propiedades de la clave:**

- Una clave por agente
- Se muestra una sola vez, al momento del registro
- No puede obtenerse ni mostrarse de nuevo después del registro
- Se almacena como hash en el registro de agentes cifrado, junto al repositorio

**Buenas prácticas:**

- Copie la clave directamente en el archivo de entorno del agente al momento del registro
- No guarde claves en el control de versiones, wikis ni registros de chat
- Si una clave se ve comprometida, elimine el agente y vuelva a registrarlo

### Rotar una clave de API { #rotating-an-api-key }

En un agente administrado desde el dashboard, regístrelo de nuevo desde **Agentes → + Agregar agente** con el mismo nombre y su token: el servidor emite una clave nueva, se la envía al agente y conserva las tareas. Desde la CLI, `hitstor serve agent re-register AGENT_NAME --token TOKEN` emite la clave nueva (colóquela en el `agent.env` del agente).

En un agente basado en la CLI:

1. Elimine el agente conservando sus tareas: `hitstor serve agent remove AGENT_NAME --keep-jobs`
2. Vuelva a registrar el agente: `hitstor serve agent add AGENT_NAME`
3. Actualice la configuración del agente con la nueva clave
4. Reinicie el servicio del agente

Eliminar un agente nunca elimina sus instantáneas.

## Registro de agentes { #agent-registry }

El registro de agentes es una estructura de datos cifrada que se almacena junto al repositorio. Contiene:

- Nombres de los agentes y hashes de las claves de API
- Metadatos de los agentes (fecha de registro, fecha del último respaldo, suspensión, plugins instalados)
- Tareas de respaldo, con su selección, programación o plan de respaldo y sus últimos resultados
- Para agentes administrados: token cifrado y dirección del agente

El registro está cifrado con la clave del repositorio y no puede leerse sin la contraseña del repositorio.

## Próximos pasos { #next-steps }

- [Despliegue de agentes](agent-deployment.md): instale y configure agentes en equipos remotos
- [Dashboard](dashboard.md): vea y administre agentes desde la interfaz web
- [Monitoreo](monitoring.md): configure alertas para agentes inactivos
