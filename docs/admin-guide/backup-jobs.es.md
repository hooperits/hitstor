# Tareas de respaldo

La página **Respaldos** del dashboard (`/backups`) es el lugar central para definir qué se respalda y cuándo. Cada tarea de respaldo tiene un `job_type` que determina cómo se recopilan los datos.

## Tipos de tarea { #job-types }

| Tipo | Recolector | Se ejecuta en |
|------|-----------|---------|
| `files` | Escaneo del sistema de archivos del agente mediante el plugin integrado (hitstor-scanner-builtin, incluido con el agente) o un plugin de escáner personalizado | Agente remoto |

`files` es el único tipo de tarea soportado. Los tipos de tarea SaaS, de base de datos y de VM en la nube
se eliminaron de forma permanente en v2.0.0; consulte la [Hoja de ruta](roadmap.md#permanently-removed-v200).
Las tareas heredadas de esos tipos se cargan, pero se deshabilitan al iniciar.

## Tareas de sistema de archivos { #filesystem-jobs }

Es el tipo de tarea predeterminado. El servidor asigna rutas y programación a un agente; el agente escanea, divide en fragmentos (chunks) y sube los datos nuevos.

- Configure las rutas desde la página de detalle de la tarea o desde la interfaz de exploración del agente
- Habilite **VSS** en agentes Windows para obtener instantáneas (snapshots) consistentes
- Defina el **intervalo de reescaneo completo** (días) para protegerse contra corrupción silenciosa
- Use **Forzar respaldo completo** desde el dashboard cuando sea necesario

## Programación { #scheduling }

Cada tarea admite:

- **Intervalo**: cada N horas
- **Cron**: cron de 5 campos con zona horaria IANA
- **Constructor visual de programación**: plantillas diaria / semanal / mensual

El daemon del agente consulta `GetSchedule` cada 5 minutos (valor predeterminado). "Respaldar ahora" envía un RPC `WakeUp` para que se tome de inmediato.

## Cancelar respaldos en ejecución { #cancel-running-backups }

`POST /api/backups/{id}/cancel` aborta una sesión activa. Las sesiones obsoletas (sin actividad durante 10 minutos o más) muestran una insignia de advertencia en el dashboard.

## API { #api }

Consulte [API HTTP del dashboard: tareas de respaldo](../reference/dashboard-api.md#backup-jobs).
