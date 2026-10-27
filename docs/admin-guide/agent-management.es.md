# Administración de agentes

HitStor admite dos métodos para registrar agentes de respaldo: **administrado desde el dashboard** (recomendado) y **basado en la CLI** (heredado). Los agentes administrados desde el dashboard usan un flujo basado en tokens en el que el servidor se conecta al agente, lo autentica y lo configura automáticamente. Los agentes basados en la CLI usan una clave de API que se distribuye manualmente.

## Agentes administrados desde el dashboard (recomendado) { #dashboard-managed-agents-recommended }

Los agentes administrados desde el dashboard se agregan directamente desde el dashboard web. El servidor se conecta al agente por gRPC, lo autentica con el token del agente y le envía automáticamente la clave de API y la dirección del servidor.

### Requisitos previos { #prerequisites }

1. Instale el agente en el equipo remoto (consulte [Despliegue de agentes](agent-deployment.md))
2. El agente genera un token durante la instalación; anote el token y la dirección IP del equipo
3. Asegúrese de que el puerto **8422** (puerto de escucha gRPC del agente) sea accesible desde el servidor

### Agregar un agente desde el dashboard { #add-an-agent-from-the-dashboard }

1. Abra el dashboard de HitStor en `http://SERVER_IP:8421`
2. Vaya a **Agentes**
3. Elija **Descubrimiento de red** para escanear las subredes locales, o **Entrada manual** para agregarlo por dirección
4. Para la entrada manual, complete:
    - **Nombre**: identificador descriptivo (por ejemplo, `web-server-01`)
    - **Dirección del agente**: `host:port` (por ejemplo, `192.168.1.50:8422`)
    - **Token**: token hexadecimal de 64 caracteres del agente (`hitstor-agent show-id` en el equipo del agente)
5. Haga clic en **Agregar agente**

Consulte [Descubrimiento de agentes](agent-discovery.md) para ver el flujo de escaneo de red.

El servidor se conecta al agente, verifica el token, envía una clave de API y la dirección del servidor, y el agente comienza a respaldar automáticamente. No es necesario copiar ninguna clave manualmente.

### Explorar el sistema de archivos del agente { #browse-agent-filesystem }

En los agentes administrados desde el dashboard, la página de detalle del agente incluye una sección **Rutas de respaldo** con un explorador en árbol del sistema de archivos:

1. Haga clic en el nombre del agente en la lista de Agentes
2. Expanda los directorios del árbol haciendo clic en los íconos de flecha
3. Marque los directorios que desea respaldar
4. Haga clic en **Guardar rutas de respaldo**

El servidor se conecta al agente por gRPC para explorar su sistema de archivos en tiempo real. Solo los directorios pueden seleccionarse como rutas de respaldo.

### Eliminar un agente { #remove-an-agent }

Desde la lista de Agentes, o mediante la CLI:

```bash
hitstor serve agent remove AGENT_NAME
```

Esto revoca la clave de API del agente. El agente ya no podrá conectarse al servidor. Las instantáneas (snapshots) existentes creadas por el agente no se eliminan.

## Agentes basados en la CLI (heredado) { #cli-based-agents-legacy }

Los agentes basados en la CLI se registran desde la línea de comandos del servidor. La clave de API debe copiarse manualmente al equipo del agente.

### Registrar un agente { #register-an-agent }

```bash
hitstor serve agent add AGENT_NAME
```

Si tiene éxito, el comando imprime una clave de API hexadecimal de 64 caracteres:

```
Agent "web-server-01" registered.
API Key: a3f8c1d9e4b7...6f2a0e5d (64 characters)

Store this key securely. It cannot be displayed again.
```

Copie esta clave en el archivo `/etc/hitstor/agent.env` del agente.

### Listar agentes { #list-agents }

Vea todos los agentes registrados (tanto administrados como basados en la CLI):

```bash
hitstor serve agent list
```

Salida:

```
NAME              STATUS     LAST BACKUP           SNAPSHOTS
web-server-01     active     2026-02-21 03:00:12   47
db-primary        active     2026-02-21 03:15:08   47
staging-app       stale      2026-02-19 12:30:00   23
```

### Eliminar un agente { #remove-an-agent_1 }

```bash
hitstor serve agent remove AGENT_NAME
```

Para eliminar un agente y todas sus instantáneas:

```bash
hitstor serve agent remove AGENT_NAME --purge-snapshots
```

## Estado del agente { #agent-status }

La columna de estado muestra:

| Estado | Significado |
|--------|-------------|
| `active` | El agente realizó un respaldo dentro del umbral de inactividad |
| `stale` | El agente no ha realizado un respaldo dentro del umbral de inactividad (predeterminado: 25 horas) |
| `new` | El agente está registrado, pero nunca ha completado un respaldo |

## Token del agente (administrado desde el dashboard) { #agent-token-dashboard-managed }

Los agentes administrados desde el dashboard se autentican con un **token**: una cadena hexadecimal de 64 caracteres (256 bits de entropía) que se genera durante la instalación del agente.

**Propiedades del token:**

- Se genera automáticamente durante la instalación y se guarda en `/etc/hitstor/agent-token` (modo 0600)
- Puede obtenerse en cualquier momento con `sudo hitstor-agent show-id`
- Se almacena cifrado (AES-256-GCM) en el servidor, en el registro de agentes
- El servidor lo usa para autenticar las llamadas gRPC al agente (explorar el sistema de archivos, configurar rutas)

## Seguridad de la clave de API (basado en la CLI) { #api-key-security-cli-based }

Los agentes basados en la CLI se autentican ante el servidor con una **clave de API hexadecimal de 64 caracteres** (256 bits de entropía).

**Propiedades de la clave:**

- Una clave por agente
- Se muestra una sola vez, al momento del registro
- No puede obtenerse ni mostrarse de nuevo después del registro
- Se almacena cifrada en el registro de agentes, junto al repositorio

**Buenas prácticas:**

- Copie la clave directamente en el archivo de entorno del agente al momento del registro
- No guarde claves en el control de versiones, wikis ni registros de chat
- Si una clave se ve comprometida, elimine el agente y vuelva a registrarlo

### Rotar una clave de API { #rotating-an-api-key }

1. Elimine el agente: `hitstor serve agent remove AGENT_NAME`
2. Vuelva a registrar el agente: `hitstor serve agent add AGENT_NAME`
3. Actualice la configuración del agente con la nueva clave
4. Reinicie el servicio del agente

Las instantáneas existentes se conservan siempre que no se use `--purge-snapshots`.

## Registro de agentes { #agent-registry }

El registro de agentes es una estructura de datos cifrada que se almacena junto al repositorio. Contiene:

- Nombres de los agentes y hashes de las claves de API
- Metadatos de los agentes (fecha de registro, fecha del último respaldo, cantidad de instantáneas)
- Configuración de programación de cada agente
- Para agentes administrados: token cifrado y dirección del agente

El registro está cifrado con la clave del repositorio y no puede leerse sin la contraseña del repositorio.

## Próximos pasos { #next-steps }

- [Despliegue de agentes](agent-deployment.md): instale y configure agentes en equipos remotos
- [Dashboard](dashboard.md): vea y administre agentes desde la interfaz web
- [Monitoreo](monitoring.md): configure alertas para agentes inactivos
