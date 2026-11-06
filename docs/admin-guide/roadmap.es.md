# Hoja de ruta

HitStor es un **producto de respaldo y restauración para Windows y Linux**: sistemas de
archivos en el núcleo y aplicaciones mediante
[plugins de carga de trabajo](../dev-guide/plugin-architecture.md) instalables. La hoja de
ruta existe para que sea excelente en eso. Por defecto, queda fuera del alcance todo trabajo
que no mejore la capacidad de respaldar y restaurar datos de forma rápida, segura y
verificable.

## Entregado { #shipped }

| Elemento | Notas |
|----------|-------|
| Motor de deduplicación y cifrado | Fragmentación (chunking) FastCDC, Zstd, AES-256-GCM, KDF scrypt |
| Arquitectura servidor / agente | gRPC sobre TLS; el agente grueso (thick agent) fragmenta y comprime localmente |
| Dashboard web | Tareas, agentes, almacenamiento, restauraciones, progreso vía SSE |
| Usuarios, roles y verificación en dos pasos | Cuentas personales con roles limitados por agente, TOTP obligatorio, registro de auditoría encadenado por hash |
| Correo | Invitaciones y enlaces para restablecer la contraseña por SMTP o Microsoft 365 (OAuth 2.0) |
| Reportes PDF programados | Resumen de protección y estado diario de los respaldos, enviados por correo según una programación o descargados |
| Respaldos incrementales y full sintético | Reutilización de la instantánea (snapshot) padre, reescaneo completo periódico |
| Programación avanzada | Cron de 5 campos con `L`, `D#N` y `DL`, constructor de programación, con zona horaria y manejo del horario de verano (DST) |
| Descubrimiento automático de agentes | Escaneo de red en busca de agentes no registrados |
| Restauración de instantáneas | Exploración del árbol de archivos, restauración granular y de rutas completas |
| Verificación de restauración | Comprobaciones automáticas de restauración después de cada respaldo, por tarea |
| Instantáneas consistentes | VSS (Windows), LVM y btrfs (Linux) |
| Recuperación ante desastres guiada | Restauración completa del sistema con varias tareas sobre un host reconstruido |
| [Restauración bare-metal (Linux)](../user-guide/bare-metal-restore.md) | Medios de rescate de ReaR generados en cada host; atributos extendidos y perfil del sistema capturados con `--bare-metal` |
| Benchmarks | Arnés reproducible de respaldo/restauración, comparación opcional con restic |
| Ransomware Sentinel | Telemetría de Gemini sobre instantáneas del sistema de archivos (opcional) |
| Plugins de carga de trabajo | Paquetes `.hpk` firmados que se instalan por agente, localmente o desde el dashboard; respaldo y restauración a un punto en el tiempo a través del agente; retención que respeta las cadenas de respaldo |
| Planes de respaldo | Las tareas de carga de trabajo programan por separado los respaldos completos, diferenciales y de log de una misma selección; resultados por base de datos |
| [SQL Server](../user-guide/sql-server.md) | Plugin `mssql` sobre VDI: respaldos completos, diferenciales y de log; restauración a un punto en el tiempo |

## Ahora { #now }

| Elemento | Objetivo |
|----------|----------|
| **Restauración bare-metal en Windows** | Medios de recuperación WinPE, descriptores de seguridad de NTFS y reconstrucción de BCD; vea más abajo |

## Próximamente { #next }

| Elemento | Objetivo |
|----------|----------|
| **Restauración remota de sistemas de archivos** | Restaurar en un agente de otra red o en una VM en la nube |
| **Rendimiento de la restauración** | Flujos de restauración en paralelo y reanudación tras una interrupción |

## Restauración bare-metal { #bare-metal-restore }

La [restauración bare-metal](../user-guide/bare-metal-restore.md) reconstruye un host
Linux a partir de un disco vacío. Relax-and-Recover (ReaR) genera medios de rescate
arrancables en cada host, recrea el diseño de disco y reinstala el cargador de arranque;
HitStor restaura los archivos. Los respaldos hechos con `--bare-metal` capturan además
los atributos extendidos (contextos de SELinux, capacidades de archivo, ACL POSIX) y el
perfil del sistema (tabla de particiones, UUID de los sistemas de archivos, LVM,
`/etc/fstab`).

Falta Windows, que requiere:

1. **Medios de arranque**: una imagen WinPE que incluya el agente.
2. **Metadatos de archivo más completos**: descriptores de seguridad de Windows, que el
   formato actual de instantáneas no captura.
3. **Reconstrucción del cargador de arranque**: BCD en Windows.

Restaurar solo los archivos en un disco vacío produce un sistema **que no arranca**, por lo
que estas piezas se entregarán juntas y no como funcionalidades independientes.

## Eliminado de forma permanente { #permanently-removed }

Estos elementos se eliminaron del producto antes de la primera versión pública (v0.1.0)
y no está previsto que regresen en su forma original:

- Respaldo de Microsoft 365 y Google Workspace
- Las integraciones de bases de datos incorporadas (PostgreSQL, MySQL, MariaDB, SQL Server, Oracle)
- Respaldo sin agente de VM en la nube (GCP, AWS, Azure) y VMware/vSphere
- El tipo de plugin recolector de cargas de trabajo (workload collector) que los atendía

El respaldo de bases de datos regresa ahora como
[plugins de carga de trabajo](../dev-guide/plugin-architecture.md) instalables que se ejecutan
en el servidor de la base de datos, empezando por [SQL Server](../user-guide/sql-server.md).

Los archivos `config.yaml` existentes que contengan `collector_plugins` o las claves de
integración relacionadas siguen cargándose; esas claves simplemente se ignoran (campos desconocidos).

## Tampoco está en la hoja de ruta { #also-not-on-the-roadmap }

La recuperación ante desastres autónoma (conmutación por error automática, scripts de arranque
generados por LLM, promoción de DNS, simulacros programados de RPO/RTO) sigue fuera del alcance.
Si la demanda regresa, debería construirse sobre la restauración basada en agentes y no sobre
un aprovisionamiento a medida en la nube.

## Vea también { #see-also }

- [Recuperación ante desastres](../user-guide/disaster-recovery.md)
- [Restauración](../user-guide/restore.md)
- [Sentinel](sentinel.md)
- [Resumen del código fuente](../dev-guide/codebase-overview.md)
