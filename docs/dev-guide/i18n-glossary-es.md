# Glosario de traducción (inglés → español)

Referencia para mantener una terminología uniforme en la documentación, el
repositorio y el dashboard de HitStor. Se trata al lector de **usted**.

| Inglés | Español | Notas |
|--------|---------|-------|
| backup (sustantivo) | respaldo | "Backup job" → "tarea de respaldo" |
| back up (verbo) | respaldar | "Back up a folder" → "Respaldar una carpeta" |
| restore | restauración (sust.), restaurar (verbo) | |
| snapshot | instantánea | La primera vez en cada página: "instantánea (snapshot)" |
| repository / repo | repositorio | |
| agent | agente | |
| server | servidor | |
| dashboard | dashboard | Se mantiene en inglés |
| storage | almacenamiento | "Storage page" → "página Almacenamiento" |
| retention policy | política de retención | |
| prune | depurar (verbo), depuración (sust.) | Primera vez: "depurar (prune)" |
| forget (snapshots) | olvidar | Primera vez: "olvidar (forget)" |
| deduplication | deduplicación | |
| chunk | fragmento | Primera vez: "fragmento (chunk)" |
| pack (file) | paquete | |
| blob | blob | Se mantiene |
| encryption / encrypted | cifrado | Nunca "encriptado" |
| key | clave | "API key" → "clave de API" |
| password | contraseña | |
| token | token | |
| schedule | programación (sust.), programar (verbo) | "Scheduled job" → "tarea programada" |
| job | tarea | |
| plugin | plugin | |
| hook | hook | |
| notifier | notificador | |
| scanner | escáner | |
| setup mode | modo de configuración inicial | |
| upgrade | actualización / actualizar | |
| bare-metal recovery (BMR) | recuperación bare-metal (BMR) | |
| disaster recovery (DR) | recuperación ante desastres (DR) | |
| Windows Service | servicio de Windows | |
| firewall rule | regla de firewall | |
| self-signed certificate | certificado autofirmado | |
| health check | verificación de estado | |
| log / logs | registro / registros | En comandos y rutas se mantiene "log" |
| settings | configuración | Nombre de página "Settings" → "Configuración" |
| overview | resumen | |
| stale (agent) | inactivo | "Stale threshold" → "umbral de inactividad" |
| stale (lock) | huérfano | "Stale lock" → "bloqueo huérfano" |
| legacy | heredado | |
| dry run | simulación | Primera vez: "simulación (dry run)" |
| staging | staging | "Staging root" → "raíz de staging" |
| workers | workers | Se mantiene |
| thick agent | agente grueso | Primera vez: "agente grueso (thick agent)" |

## Nombres en el dashboard

| Inglés | Español |
|--------|---------|
| Monitoring | Monitoreo |
| Overview | Resumen |
| Backups | Respaldos |
| Restores | Restauraciones |
| Disaster Recovery | Recuperación ante desastres |
| Agents | Agentes |
| Storage | Almacenamiento |
| System | Sistema |
| View Logs | Ver registros |
| Settings | Configuración |
| Account | Cuenta |
| Logout | Cerrar sesión |
| Backup Now | Respaldar ahora |
| Add Agent | Agregar agente |
| Sign In | Iniciar sesión |
| Snapshots | Instantáneas |
| New Backup Job | Nueva tarea de respaldo |
| New Repository | Nuevo repositorio |
| Backup Paths / Save Backup Paths | Rutas de respaldo / Guardar rutas de respaldo |
| Save Schedule | Guardar programación |
| Interval / Cron Expression | Intervalo / Expresión cron |
| Force full backup | Forzar respaldo completo |
| Network Discovery / Manual Entry / Scan | Descubrimiento de red / Entrada manual / Escanear |
| General / Security & TLS / Network / Integrations / Notifications | General / Seguridad y TLS / Red / Integraciones / Notificaciones |
| Import from environment | Importar desde el entorno |
| Save / Cancel / Delete / Edit / Close / Refresh | Guardar / Cancelar / Eliminar / Editar / Cerrar / Actualizar |

## Estados en el dashboard

Las etiquetas de estado (badges) se escriben en masculino singular, porque se
comparten entre tareas, sesiones, restauraciones y agentes: `completado`,
`fallido`, `omitido`, `pendiente`, `en ejecución`, `desactivado`, `atrasado`,
`inactivo`, `suspendido`, `nunca`. Los valores técnicos cortos se mantienen:
`ok`, `info`, `cli`.

## Reglas

- No traducir: bloques de código, comandos, flags, nombres de archivos, rutas,
  claves de configuración (`serve_dashboard_password`), variables de entorno,
  nombres de API ni salidas de la CLI (la CLI se mantiene en inglés).
- Los nombres de elementos de la interfaz (botones, páginas) van en el idioma
  del dashboard al que se refiere el texto: en español, los nombres en español.
- En la documentación (MkDocs), los encabezados traducidos conservan el anchor
  en inglés: `## Instalar { #install }`. En los README de GitHub no se usa esa
  sintaxis: los enlaces internos apuntan al anchor del encabezado en español.
