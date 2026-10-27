# Dashboard web

HitStor incluye un dashboard web integrado para monitorear la infraestructura de respaldo, administrar agentes, explorar sistemas de archivos remotos y configurar programaciones. El dashboard lo sirve el mismo proceso `hitstor serve` que atiende las conexiones gRPC.

## Acceso al dashboard { #accessing-the-dashboard }

El dashboard está disponible en:

```
http://SERVER_IP:8421
```

Reemplace `SERVER_IP` por el nombre de host o la dirección IP del servidor HitStor. El puerto predeterminado del dashboard es `8421` y se puede cambiar con `--dashboard-port`.

## Protección con contraseña { #password-protection }

El dashboard siempre requiere contraseña. Establézcala con la clave de configuración `serve_dashboard_password`:

```yaml
# /etc/hitstor/config.yaml
serve_dashboard_password: "DASHBOARD_PASSWORD"
```

Si no hay ninguna contraseña configurada, `hitstor serve` genera una aleatoria al iniciar, guarda su hash en el archivo de configuración e imprime el texto plano una sola vez en el registro del servidor (busque `no dashboard password configured`). Inicie sesión con ella y cámbiela en **Configuración**. Si el servidor no tiene archivo de configuración, se genera una contraseña nueva en cada reinicio.

El dashboard presenta una página de inicio de sesión. La autenticación usa cookies de sesión con las siguientes propiedades de seguridad:

- `HttpOnly`: las cookies no son accesibles desde JavaScript
- `SameSite=Strict`: las cookies no se envían en solicitudes entre sitios

Las sesiones expiran al cerrar el navegador.

## Notificaciones de actualización { #update-notifications }

Cuando `update_check: true` está definido en la configuración (valor predeterminado), el servidor busca nuevas versiones mediante la API de GitHub Releases cada 24 horas. Si hay una versión más reciente, aparece un banner de notificación en la parte superior de cada página del dashboard:

> **HitStor v1.1.0 está disponible (actual: v1.0.0).** [Ver versión](#)

El banner enlaza a la página de la versión en GitHub. Para deshabilitar las comprobaciones automáticas, defina `update_check: false` en `/etc/hitstor/config.yaml`. También puede comprobarlo manualmente desde la CLI con `hitstor version --check`.

## Diseño { #layout }

El dashboard usa una barra de encabezado fija con el logotipo y la versión de HitStor, una barra lateral con navegación por íconos y un área de contenido principal. La barra lateral agrupa las páginas en dos secciones:

- **Monitoreo**: Resumen, Respaldos, Agentes
- **Administración**: Almacenamiento, Configuración

## Páginas del dashboard { #dashboard-pages }

### Resumen { #overview }

La página principal muestra un resumen de la infraestructura de respaldo:

- Número total de agentes, instantáneas (snapshots) y hora del último respaldo
- Desglose del estado de los agentes (OK, inactivo, nunca respaldado)
- Accesos rápidos a las páginas Agentes, Instantáneas y Almacenamiento

### Agentes { #agents }

Lista todos los agentes registrados con su estado actual, tipo (administrado o CLI), hora del último respaldo y fecha de creación. Haga clic en el nombre de un agente para ver su página de detalle.

**Agregar un agente desde el dashboard:**

1. Use **Descubrimiento de red** para escanear subredes, o **Entrada manual** para un `host:port` específico
2. Ingrese el nombre del agente, `agent_addr` y el token (ingreso manual)
3. Confirme el registro

El servidor se conecta al agente mediante AgentService (puerto 8422), verifica el token y envía la clave de API y la dirección del servidor. Consulte [Administración de agentes](agent-management.md) y [Descubrimiento de agentes](agent-discovery.md).

### Detalle del agente { #agent-detail }

Muestra información detallada de un agente:

- **Repositorio de destino**: seleccione en qué repositorio respalda este agente
- **Programación de respaldo**: habilite o deshabilite los respaldos programados y defina el intervalo en horas
- **Rutas de respaldo** (solo agentes administrados): explorador en árbol del sistema de archivos para seleccionar los directorios que se respaldarán
- **Historial de instantáneas**: tabla con los ID de instantánea, marcas de tiempo, cantidad de archivos, tamaños y duraciones

#### Explorador en árbol del sistema de archivos { #filesystem-tree-browser }

Para los agentes administrados desde el dashboard, la sección Rutas de respaldo muestra en vivo el árbol del sistema de archivos de la máquina del agente. El servidor se conecta al agente por gRPC para listar los directorios en tiempo real.

- Haga clic en el ícono de flecha para expandir un directorio y cargar su contenido
- Marque los directorios que desea incluir en los respaldos
- Haga clic en **Guardar rutas de respaldo** para enviar la configuración al agente

Los directorios que el proceso del agente no puede leer aparecen atenuados.

### Instantáneas { #snapshots }

Lista todas las instantáneas de todos los agentes. Permite filtrar por nombre de agente. Cada entrada muestra el ID de la instantánea, el agente, la marca de tiempo, las rutas, la cantidad de archivos y el tamaño total.

### Almacenamiento { #storage }

La página Almacenamiento ofrece administración completa de repositorios:

- **Lista de repositorios**: muestra todos los repositorios configurados (locales y S3) con insignias de estado, barras de uso de disco, cantidad de instantáneas y estadísticas de paquetes e índices
- **Crear repositorio**: inicializa un nuevo repositorio local o compatible con S3 directamente desde el dashboard. Los repositorios locales muestran el espacio en disco disponible y detectan repositorios existentes en la ruta indicada
- **Conectar repositorio existente**: si ya existe un repositorio en una ruta (por ejemplo, después de reinstalar), el dashboard lo detecta y se conecta a él en lugar de reinicializarlo
- **Desconectar repositorio**: quita un repositorio de la configuración (no borra los datos en disco)
- **Verificación de integridad**: ejecuta `hitstor check` sobre un repositorio con progreso en tiempo real mediante Server-Sent Events. Los resultados muestran si pasó o falló y los errores encontrados
- **Depurar (prune)**: libera espacio eliminando datos no referenciados, con un diálogo de confirmación y progreso en vivo. Muestra los paquetes eliminados, los bytes liberados y los índices reescritos
- **Gráfico de crecimiento**: visualiza el crecimiento del almacenamiento a lo largo del tiempo con un rango de fechas configurable (30 días, 90 días, 6 meses, 1 año, todo) y granularidad (diaria, semanal)
- **Detalle de instantánea**: explore el contenido de una instantánea y restaure archivos individuales o rutas completas. Consulte [Restauración](../user-guide/restore.md).

### Configuración { #settings }

Punto central de la configuración del servidor: rendimiento general, TLS, puertos de red, **Integraciones** (Gemini) y notificaciones. Consulte [Configuración del servidor](settings.md).

## Administración de programaciones { #schedule-management }

El dashboard permite a los administradores configurar la programación de respaldos de cada agente sin iniciar sesión en la línea de comandos del servidor.

Desde la página Detalle del agente:

1. Active o desactive la programación del agente
2. Defina el intervalo de respaldo en horas (por ejemplo, cada 24 horas)
3. Haga clic en **Guardar programación**

Los agentes que se ejecutan en modo daemon (`hitstor-agent daemon`) consultan al servidor si hay cambios en la programación y ejecutan los respaldos en consecuencia.

## API JSON { #json-api }

El dashboard expone una API JSON para acceso programático. Todos los endpoints devuelven respuestas JSON.

### Endpoints { #endpoints }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/api/agents` | Lista todos los agentes registrados con su estado |
| `POST` | `/api/agents` | Agrega un agente administrado desde el dashboard (nombre, dirección, token) |
| `DELETE` | `/api/agents/{name}` | Elimina un agente |
| `POST` | `/api/agents/{name}/schedule` | Actualiza la programación de respaldo del agente |
| `POST` | `/api/agents/{name}/repo` | Define el repositorio de destino del agente |
| `POST` | `/api/agents/{name}/browse` | Explora el sistema de archivos del agente (agentes administrados) |
| `POST` | `/api/agents/{name}/configure` | Configura las rutas de respaldo del agente (agentes administrados) |
| `GET` | `/api/agents/{name}/info` | Consulta la información del agente (agentes administrados) |
| `GET` | `/api/snapshots` | Lista todas las instantáneas (admite el filtro `?agent=NAME`) |
| `GET` | `/api/stats` | Estadísticas del repositorio (almacenamiento, deduplicación, conteos) |
| `GET` | `/api/repos` | Lista todos los repositorios configurados |
| `POST` | `/api/repos` | Crea un nuevo repositorio (local o S3) |
| `DELETE` | `/api/repos/{name}` | Desconecta un repositorio |
| `POST` | `/api/repos/test-s3` | Prueba la conectividad con un endpoint S3 |
| `POST` | `/api/repos/detect` | Detecta un repositorio existente en una ruta |
| `GET` | `/api/repos/{name}/growth` | Datos de crecimiento del almacenamiento para gráficos |
| `GET` | `/api/repos/{name}/sessions` | Sesiones de respaldo activas de un repositorio |
| `POST` | `/api/repos/{name}/check` | Inicia una verificación de integridad (devuelve el ID de la operación) |
| `POST` | `/api/repos/{name}/prune` | Inicia una operación de depuración (devuelve el ID de la operación) |
| `GET` | `/api/operations/{id}/events` | Flujo SSE con el progreso de la operación |
| `GET` | `/api/disk-space` | Espacio en disco disponible en una ruta dada |

### Ejemplo: agregar un agente { #example-add-an-agent }

```bash
curl -X POST http://localhost:8421/api/agents \
  -H 'Content-Type: application/json' \
  -d '{"name":"web-01","address":"192.168.1.50:8422","token":"a3f8c1d9e4b7..."}'
```

### Ejemplo: explorar el sistema de archivos del agente { #example-browse-agent-filesystem }

```bash
curl -X POST http://localhost:8421/api/agents/web-01/browse \
  -H 'Content-Type: application/json' \
  -d '{"path":"/home"}'
```

### Ejemplo: listar agentes { #example-list-agents }

```bash
curl -s http://localhost:8421/api/agents | jq .
```

Si la protección con contraseña del dashboard está habilitada, incluya la cookie de sesión o autentíquese primero:

```bash
# Autenticarse y guardar la cookie
curl -c cookies.txt -d "password=DASHBOARD_PASSWORD" http://localhost:8421/login

# Usar la cookie en las solicitudes a la API
curl -b cookies.txt -s http://localhost:8421/api/stats | jq .
```

## Métricas de Prometheus { #prometheus-metrics }

El dashboard expone un endpoint de métricas compatible con Prometheus:

```
http://SERVER_IP:8421/metrics
```

Este endpoint **no requiere autenticación** (es accesible sin la contraseña del dashboard) para que Prometheus pueda recolectar métricas sin gestionar credenciales. Si esto le preocupa, restrinja el acceso a nivel de red.

Consulte [Monitoreo](monitoring.md) para ver la lista completa de métricas y la configuración de Prometheus.

## Próximos pasos { #next-steps }

- [Administración de agentes](agent-management.md): registrar y administrar agentes
- [Monitoreo](monitoring.md): configurar la recolección de Prometheus y las alertas
- [Programación](scheduling.md): opciones detalladas de configuración de la programación
- [Seguridad](security.md): autenticación y endurecimiento del dashboard
