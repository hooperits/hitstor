# API HTTP del dashboard

El dashboard integrado que sirve `hitstor serve` expone una API JSON en el puerto HTTP (predeterminado **8421**). Las rutas se definen en `internal/dashboard/dashboard.go`.

Salvo que se indique lo contrario, los endpoints requieren una sesión iniciada (consulte [Autenticación](#authentication)), y cada ruta necesita un rol: las lecturas requieren el rol de visualización, los cambios en tareas el rol de operador de respaldo, las restauraciones el rol de operador de restauración, y los usuarios, el almacenamiento, la configuración y los demás cambios de todo el servidor, el de administrador. Las rutas sobre un agente aceptan un rol sobre ese agente; las que abarcan varios agentes necesitan el rol sobre todos. Los cambios sensibles además requieren un inicio de sesión reciente. Consulte [Usuarios y acceso](../admin-guide/users.md#roles).

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
| `GET /settings` | Configuración del servidor (General, Seguridad/TLS, Red, Integraciones, Notificaciones, Correo, Plugins) |
| `GET /disaster` | Recuperación ante desastres |
| `GET /logs` | Registros del servidor y de los agentes |
| `GET /reports` | Reportes programados |
| `GET /users`, `GET /users/{name}` | Usuarios y acceso (administradores) |
| `GET /audit` | Registro de auditoría (administradores) |
| `GET /account`, `GET /account/password` | Cuenta del usuario que inició sesión y cambio de contraseña |

`GET /snapshots` redirige a `/storage`.

## Agentes { #agents }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/api/agents` | Lista los agentes con su estado, información de programación y plugins de carga de trabajo instalados (`plugins`) |
| `POST` | `/api/agents` | Registra un agente (entrada manual: `name`, `agent_addr`, `api_key` opcional) |
| `DELETE` | `/api/agents/{name}` | Elimina un agente |
| `POST` | `/api/agents/{name}/schedule` | Define la programación (intervalo, cron, zona horaria, tareas) |
| `POST` | `/api/agents/{name}/repo` | Define el repositorio de destino |
| `POST` | `/api/agents/{name}/browse` | Explora el sistema de archivos del agente |
| `POST` | `/api/agents/{name}/configure` | Define las rutas de respaldo y las exclusiones |
| `GET` | `/api/agents/{name}/info` | Información del agente |
| `GET` | `/api/agents/{name}/snapshots` | Instantáneas de un agente |
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
| `POST` | `/api/backups` | Crea una tarea. `job_type` es `files` (con `paths`) o `workload` (con `plugin`, `selection` y, opcionalmente, `plan` (un `{kind, enabled, cron_expression, timezone}` por tipo de respaldo), `backup_kind` y `plugin_settings`; el agente debe haber reportado el plugin) |
| `GET` | `/api/backups/{id}` | Obtiene una tarea |
| `PUT` | `/api/backups/{id}` | Actualiza una tarea |
| `DELETE` | `/api/backups/{id}` | Elimina una tarea |
| `POST` | `/api/backups/{id}/run` | Ejecuta la tarea ahora; en tareas de workload, `{"kind": "full"}` elige el tipo del plan |
| `GET` | `/api/backups/{id}/chains` | Cadenas de respaldo de cada base de una tarea de workload |
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
| `GET` | `/api/repos/{name}`, `/api/repos/{name}/health`, `/api/repos/{name}/snapshots` | Métricas, estado operativo e instantáneas agrupadas por agente de un repositorio |
| `GET` | `/api/repos/growth` | Datos de crecimiento de varios repositorios en una sola petición |
| `POST` | `/api/repos/{name}/forget` | Aplica una política de retención (administradores, inicio de sesión reciente) |
| `POST` | `/api/repos/detect-s3`, `/api/repos/validate-path` | Detecta un repositorio existente en un bucket; valida una ruta local |

## Instantáneas y restauración { #snapshots-restore }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/api/snapshots?host=&limit=` | Lista las instantáneas (las más recientes primero) |
| `GET` | `/api/snapshots/{id}/tree?path=` | Explora el árbol de archivos de una instantánea |
| `POST` | `/api/snapshots/{id}/restore` | Inicia una restauración en un agente (`agent_name`, `target_path`, `includes` opcional) |
| `GET` | `/api/restores` | Lista las sesiones de restauración |
| `GET` | `/api/restores/sources` | Orígenes que se pueden restaurar, tal como se listan en la página Restauraciones |
| `GET` | `/api/restore/{id}/events` | Progreso de la restauración vía SSE |
| `GET` | `/api/agents/{name}/plugins/{plugin}/browse?path=` | Lista los objetos que un plugin de carga de trabajo del agente puede respaldar, con `detail`, `status` y si cada uno admite configuración (`configurable`) y la tiene guardada (`configured`) |
| `GET` | `/api/agents/{name}/plugins/{plugin}/object-config?path=` | Configuración guardada en el agente para un objeto (`configured`, `values` sin contraseñas, `secrets_set`) |
| `PUT` | `/api/agents/{name}/plugins/{plugin}/object-config` | Prueba y guarda en el agente la configuración de un objeto (`path`, `values`; una contraseña vacía conserva la guardada), o la quita (`path`, `remove: true`). El servidor no guarda nada |
| `GET` | `/api/workload/points?agent=&plugin=&object=` | Elementos disponibles para restaurar un objeto de carga de trabajo, del más antiguo al más reciente |
| `GET` | `/api/workload/catalog?agent=&plugin=&snapshot=` | Todos los elementos que un plugin respaldó en un agente, del más reciente al más antiguo, con su `object`; `snapshot` (un prefijo de ID) deja solo los de una instantánea |
| `POST` | `/api/workload/restore-plan` | Vista previa de una restauración con plugin sin ejecutarla (mismo cuerpo que `POST /api/workload/restores`): los elementos que aplica, los archivos que escribe (`key`, `kind`, `source`, `target`), `warnings` y un `summary` |
| `POST` | `/api/workload/restores` | Inicia una restauración de carga de trabajo (`agent_name`, `plugin`, `object`; opcionales: `source_agent` para restaurar respaldos de otro agente, `target` para el punto en el tiempo, `options` y `plugin_settings`) |
| `GET` | `/api/workload/restores/{id}` | Estado de una restauración de carga de trabajo: plan, bytes enviados y resultado |
| `GET` | `/api/plugin-packages` | Lista los paquetes de plugins cargados |
| `POST` | `/api/plugin-packages` | Carga un paquete de plugin (el archivo `.hpk` como cuerpo `application/octet-stream`); se rechaza si no está firmado por una clave de confianza |
| `DELETE` | `/api/plugin-packages/{file}` | Elimina un paquete cargado |
| `POST` | `/api/agents/{name}/plugins` | Instala un paquete cargado en un agente administrado (`package`; opcionales `upgrade` y `allow_downgrade`); el agente lo vuelve a verificar |
| `DELETE` | `/api/agents/{name}/plugins/{id}` | Desinstala un plugin de un agente administrado; se rechaza mientras una tarea lo usa |

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
| `PUT` | `/api/settings/email` | Servidor de correo saliente para invitaciones, enlaces de restablecimiento de contraseña y reportes (inicio de sesión reciente) |
| `POST` | `/api/settings/email/test` | Envía un correo de prueba con la configuración de la petición |
| `PUT` | `/api/settings/integrations` | Clave/modelo de Gemini (`gemini_api_key` vacío = conservar la actual) |
| `POST` | `/api/settings/integrations/import-env` | Migra `GEMINI_API_KEY` al YAML cifrado |
| `GET` | `/api/settings/tls/status` | Metadatos del certificado TLS (origen, vencimiento, SAN) |
| `POST` | `/api/settings/tls/upload` | Archivos PEM `cert` + `key` en multipart; establece `restart_required` |
| `POST` | `/api/settings/test/gemini` | Valida la clave de API de Gemini |
| `POST` | `/api/settings/test/webhook` | Envía por POST una carga de prueba a la URL del webhook configurado |

## Usuarios, cuenta y auditoría { #users-account-and-audit }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/api/users` | Lista los usuarios (administradores) |
| `POST` | `/api/users` | Crea un usuario: envía una invitación si el correo está configurado; si no, devuelve una contraseña temporal |
| `GET` / `PUT` / `DELETE` | `/api/users/{name}` | Lee, modifica (roles, estado, perfil) o elimina un usuario |
| `POST` | `/api/users/{name}/password` | Restablece la contraseña de un usuario (contraseña temporal o enlace por correo) |
| `DELETE` | `/api/users/{name}/second-factor` | Restablece la verificación en dos pasos de un usuario |
| `GET` | `/api/users/{name}/sessions` | Sesiones de un usuario |
| `DELETE` | `/api/users/{name}/sessions/{id}` | Cierra una de las sesiones de un usuario |
| `POST` | `/api/account/password` | Cambia la contraseña del usuario que inició sesión |
| `POST` | `/api/account/recovery-codes` | Genera nuevos códigos de recuperación (inicio de sesión reciente) |
| `PUT` | `/api/account/profile` | Cambia el nombre visible del usuario que inició sesión |
| `GET` / `DELETE` | `/api/account/sessions` | Lista las sesiones del usuario que inició sesión, o cierra todas salvo la actual |
| `DELETE` | `/api/account/sessions/{id}` | Cierra una de las sesiones del usuario que inició sesión |
| `POST` | `/api/reauth` | Confirma la contraseña y un código antes de una acción sensible |
| `GET` | `/api/session` | Tiempo restante de la sesión (no cuenta como actividad) |
| `POST` | `/api/session/activity` | Registra actividad del usuario para que la sesión siga abierta |
| `GET` | `/api/audit`, `/api/audit/export` | Registro de auditoría y su exportación a CSV (administradores) |

Todos los cambios de usuarios requieren un administrador y un inicio de sesión reciente.

## Reportes { #reports }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/api/reports` | Lista los reportes programados |
| `POST` | `/api/reports` | Crea un reporte programado |
| `PUT` / `DELETE` | `/api/reports/{id}` | Modifica o elimina un reporte |
| `POST` | `/api/reports/{id}/send` | Envía un reporte ahora |
| `GET` | `/api/reports/pdf` | Descarga un reporte en PDF, tal como se enviaría ahora |

Los reportes son para administradores. Consulte [Reportes](../admin-guide/reports.md).

## Recuperación ante desastres y bare-metal { #disaster-recovery-and-bare-metal }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/api/disaster/plans`, `/api/disaster/plans/{agent}` | Preparación para la recuperación de todos los sistemas, o el plan de uno |
| `POST` | `/api/disaster/recover` | Inicia una recuperación del sistema completo (administradores, inicio de sesión reciente) |
| `GET` | `/api/disaster/recoveries`, `/api/disaster/recoveries/{id}` | Recuperaciones orquestadas y su estado |
| `GET` | `/api/bmr/hosts` | Agentes con el estado de su medio de recuperación |
| `POST` | `/api/bmr/media/{agent}/build` | Crea el medio de recuperación en un agente |
| `GET` / `DELETE` | `/api/bmr/media/{agent}/download`, `/api/bmr/media/{agent}` | Descarga o descarta la imagen de recuperación en caché de un host |
| `POST` | `/api/bmr/prepare/{agent}` | Emite un token de recuperación de un solo uso (administradores, inicio de sesión reciente) |
| `GET` / `DELETE` | `/api/bmr/tokens`, `/api/bmr/tokens/{id}` | Lista los tokens de recuperación sin usar, o revoca uno |

Consulte [Recuperación ante desastres](../user-guide/disaster-recovery.md) y [Restauración bare-metal](../user-guide/bare-metal-restore.md).

## Registros { #logs }

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET` | `/api/logs` | Entradas de registro del servidor y de los agentes |
| `GET` | `/api/logs/export` | Exporta entradas de registro |

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
| `GET` / `POST` | `/login` | Página de inicio de sesión; envía el nombre de usuario y la contraseña (o, antes de que exista alguna cuenta, la contraseña del dashboard) |
| `GET` / `POST` | `/login/code` | Segundo paso: un código de la app de autenticación o un código de recuperación |
| `GET` / `POST` | `/login/enroll` | Configura la app de autenticación en el primer inicio de sesión |
| `GET` / `POST` | `/login/forgot` | Solicita un enlace de restablecimiento de contraseña por correo |
| `GET` / `POST` | `/login/reset` | Elige una contraseña nueva desde un enlace recibido por correo |
| `GET` / `POST` | `/setup` | Crea el primer administrador con la contraseña del dashboard |
| `POST` | `/logout` | Cierra la sesión |

La sesión es la cookie `hitstor_session`, que se emite solo después de los dos pasos del inicio de sesión. No hay tokens de API: los scripts llaman a la API con la cookie de un usuario que inició sesión, y la sesión termina tras el tiempo de inactividad como cualquier otra (consulte [Sesiones y bloqueo](../admin-guide/users.md#sessions)). Una petición que requiere un inicio de sesión reciente responde `403` con `{"error": "reauth_required"}` y el encabezado `X-HitStor-Reauth: 1`; confirme con `POST /api/reauth` y vuelva a intentarlo. Las peticiones que cambian el estado enviadas por un navegador desde otro origen se rechazan.

## Vea también { #see-also }

- [API gRPC](grpc-api.md): protocolo de respaldo de los agentes
- [Dashboard](../admin-guide/dashboard.md): resumen de la interfaz
- [Tareas de respaldo](../admin-guide/backup-jobs.md): tipos de tareas y flujos de trabajo
