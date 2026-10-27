# API HTTP del dashboard

El dashboard integrado que sirve `hitstor serve` expone una API JSON en el puerto HTTP (predeterminado **8421**). Las rutas se definen en `internal/dashboard/dashboard.go`.

Salvo que se indique lo contrario, los endpoints requieren autenticación en el dashboard cuando `serve_dashboard_password` está definido. Las cookies de sesión se emiten mediante `POST /login`.

## Páginas { #pages }

| Ruta | Descripción |
|------|-------------|
| `GET /` | Resumen |
| `GET /backups` | Lista de tareas de respaldo |
| `GET /backups/{id}` | Detalle de una tarea de respaldo |
| `GET /agents` | Agentes |
| `GET /agents/{name}` | Detalle de un agente |
| `GET /snapshots/{id}` | Detalle de una instantánea (snapshot) (exploración + restauración) |
| `GET /storage` | Administración de repositorios |
| `GET /storage/{name}` | Detalle de un repositorio |
| `GET /restores` | Sesiones de restauración |
| `GET /settings` | Configuración del servidor (General, Seguridad/TLS, Red, Integraciones, Notificaciones) |

`GET /snapshots` redirige a `/storage`.

## Agentes { #agents }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/api/agents` | Lista los agentes con su estado e información de programación |
| `POST` | `/api/agents` | Registra un agente (entrada manual: `name`, `agent_addr`, `api_key` opcional) |
| `DELETE` | `/api/agents/{name}` | Elimina un agente |
| `POST` | `/api/agents/{name}/schedule` | Define la programación (intervalo, cron, zona horaria, tareas) |
| `POST` | `/api/agents/{name}/repo` | Define el repositorio de destino |
| `POST` | `/api/agents/{name}/browse` | Explora el sistema de archivos del agente |
| `POST` | `/api/agents/{name}/configure` | Define las rutas de respaldo y las exclusiones |
| `GET` | `/api/agents/{name}/info` | Información del agente |
| `POST` | `/api/agents/{name}/suspend` | Suspende los respaldos del agente |
| `POST` | `/api/agents/{name}/unsuspend` | Reanuda el agente suspendido |

## Descubrimiento de agentes { #agent-discovery }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/api/discover/subnets` | Lista las subredes locales que se pueden escanear |
| `POST` | `/api/discover/scan` | Escanea una subred en busca de agentes |
| `POST` | `/api/discover/probe` | Sondea un host:puerto específico |

## Tareas de respaldo { #backup-jobs }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/api/backups` | Lista las tareas de respaldo |
| `POST` | `/api/backups` | Crea una tarea (`job_type` siempre es `files`) |
| `GET` | `/api/backups/{id}` | Obtiene una tarea |
| `PUT` | `/api/backups/{id}` | Actualiza una tarea |
| `DELETE` | `/api/backups/{id}` | Elimina una tarea |
| `POST` | `/api/backups/{id}/run` | Ejecuta la tarea ahora |
| `POST` | `/api/backups/{id}/cancel` | Cancela un respaldo en curso |
| `GET` | `/api/backups/validate-cron` | Valida una expresión cron |
| `GET` | `/api/timezones` | Lista las zonas horarias admitidas |

## Repositorios y almacenamiento { #repositories-storage }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/api/repos` | Lista los repositorios |
| `POST` | `/api/repos` | Crea un repositorio (local o S3) |
| `DELETE` | `/api/repos/{name}` | Desconecta un repositorio |
| `GET` | `/api/disk-space?path=` | Espacio en disco de una ruta |
| `POST` | `/api/repos/test-s3` | Prueba las credenciales de S3 |
| `POST` | `/api/repos/detect` | Detecta un repositorio existente en una ruta |
| `GET` | `/api/repos/{name}/growth` | Datos del gráfico de crecimiento (`?range=`, `?granularity=`) |
| `POST` | `/api/repos/{name}/check` | Inicia una verificación de integridad |
| `POST` | `/api/repos/{name}/prune` | Inicia una depuración (prune) |
| `GET` | `/api/repos/{name}/sessions` | Sesiones activas del repositorio |

## Instantáneas y restauración { #snapshots-restore }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/api/snapshots?host=&limit=` | Lista las instantáneas (las más recientes primero) |
| `GET` | `/api/snapshots/{id}/tree?path=` | Explora el árbol de archivos de una instantánea |
| `POST` | `/api/snapshots/{id}/restore` | Inicia una restauración en un agente (`agent_name`, `target_path`, `includes` opcional) |
| `GET` | `/api/restores` | Lista las sesiones de restauración |
| `GET` | `/api/restore/{id}/events` | Progreso de la restauración vía SSE |

## Progreso y operaciones { #progress-operations }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/api/stats` | Estadísticas del repositorio |
| `GET` | `/api/active-backups` | Sesiones de respaldo activas |
| `GET` | `/api/backup-progress/events` | Progreso de los respaldos vía SSE (todas las sesiones) |
| `GET` | `/api/operations/{id}/events` | SSE para las operaciones de verificación/depuración |

## Configuración { #settings }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/api/settings` | Configuración efectiva del servidor (secretos enmascarados) |
| `PUT` | `/api/settings/general` | `update_check`, `update_url`, `parallel`, `sync_writes`, `min_free_reserve_bytes` |
| `PUT` | `/api/settings/security` | `dashboard_password` (vacío = conservar la actual); puede establecer `restart_required` |
| `PUT` | `/api/settings/network` | `serve_port`, `serve_dashboard_port`, `serve_advertise_addr`; puede establecer `restart_required` |
| `PUT` | `/api/settings/notifications` | `alert_webhook_url`, `alert_stale_threshold`, `alert_notifier_plugins` |
| `PUT` | `/api/settings/integrations` | Clave/modelo de Gemini (`gemini_api_key` vacío = conservar la actual) |
| `POST` | `/api/settings/integrations/import-env` | Migra `GEMINI_API_KEY` al YAML cifrado |
| `GET` | `/api/settings/tls/status` | Metadatos del certificado TLS (origen, vencimiento, SAN) |
| `POST` | `/api/settings/tls/upload` | Archivos PEM `cert` + `key` en multipart; establece `restart_required` |
| `POST` | `/api/settings/test/gemini` | Valida la clave de API de Gemini |
| `POST` | `/api/settings/test/webhook` | Envía por POST una carga de prueba a la URL del webhook configurado |

## Alertas (Sentinel) { #alerts-sentinel }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/api/alerts` | Lista las alertas de Sentinel/seguridad |
| `POST` | `/api/alerts/{id}/resolve` | Resuelve una alerta |

## Métricas (sin autenticación) { #metrics-unauthenticated }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/metrics` | Exposición en formato de texto de Prometheus |

## Autenticación { #authentication }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/login` | Página de inicio de sesión |
| `POST` | `/login` | Envía la contraseña |
| `POST` | `/logout` | Cierra la sesión |

## Vea también { #see-also }

- [API gRPC](grpc-api.md): protocolo de respaldo de los agentes
- [Dashboard](../admin-guide/dashboard.md): resumen de la interfaz
- [Tareas de respaldo](../admin-guide/backup-jobs.md): tipos de tareas y flujos de trabajo
