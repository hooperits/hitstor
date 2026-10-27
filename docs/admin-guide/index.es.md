# Guía de administración

La Guía de administración de HitStor está dirigida a los administradores de sistemas responsables de desplegar, configurar y mantener la infraestructura de respaldo de HitStor. Abarca la configuración del servidor, la gestión de agentes, la configuración de TLS, el monitoreo y el endurecimiento de la seguridad.

Si busca instrucciones para crear y restaurar respaldos, consulte la [Guía del usuario](../user-guide/index.md).

## Temas { #topics }

| Guía | Descripción |
|-------|-------------|
| [Puesta en marcha del servidor](server-setup.md) | Inicializar un repositorio, iniciar el servidor, configurar systemd y gestionar las reglas de firewall. |
| [Certificados TLS](tls-certificates.md) | Configurar certificados TLS generados automáticamente o personalizados para la comunicación gRPC y del dashboard. |
| [Tareas de respaldo](backup-jobs.md) | Tareas de respaldo del sistema de archivos, programación y retención. |
| [Gestión de agentes](agent-management.md) | Registrar, configurar y eliminar agentes de respaldo. |
| [Descubrimiento de agentes](agent-discovery.md) | Escaneo de red en busca de agentes no registrados. |
| [Despliegue de agentes](agent-deployment.md) | Instalar y configurar el binario `hitstor-agent` en equipos Linux y Windows. |
| [Dashboard](dashboard.md) | Acceder al dashboard web, gestionar programaciones y usar la API JSON. |
| [Configuración del servidor](settings.md) | Centro de configuración: TLS, red, integraciones (Gemini), notificaciones. |
| [Monitoreo](monitoring.md) | Métricas de Prometheus, alertas por webhook e integración con Grafana. |
| [Programación](scheduling.md) | Configurar programaciones gestionadas por el servidor, el modo daemon y los temporizadores de systemd. |
| [Sentinel](sentinel.md) | Telemetría y alertas de ransomware. |
| [Seguridad](security.md) | Modelo de cifrado, derivación de claves, gestión de claves de API y endurecimiento de systemd. |
| [Solución de problemas](troubleshooting.md) | Errores comunes, comandos de diagnóstico y análisis de registros. |
| [Hoja de ruta](roadmap.md) | Prioridades actuales y áreas fuera del alcance. |

## Requisitos previos { #prerequisites }

Antes de seguir las guías de esta sección, asegúrese de tener HitStor instalado. Consulte la [Guía de instalación](../installation/index.md) para ver las instrucciones específicas de cada plataforma.

## Convenciones { #conventions }

En toda la Guía de administración se usan las siguientes convenciones:

- Los comandos precedidos por `$` se ejecutan en una terminal.
- El texto en `UPPER_CASE` (como `AGENT_NAME`) representa un valor que usted debe reemplazar por el suyo.
- Los flags opcionales se muestran entre corchetes: `[--flag VALUE]`.
- Los comandos largos se dividen en varias líneas con `\` para facilitar la lectura.
- Las rutas de los archivos de configuración suponen una instalación estándar de Linux. Ajuste las rutas en otras plataformas según sea necesario.
