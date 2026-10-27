# Hoja de ruta

HitStor es un **producto de respaldo y restauración de sistemas de archivos para Windows y Linux**,
y la hoja de ruta existe para que sea excelente exactamente en eso. Por defecto, queda fuera
del alcance todo trabajo que no mejore la capacidad de respaldar y restaurar archivos de forma
rápida, segura y verificable.

## Entregado { #shipped }

| Elemento | Notas |
|----------|-------|
| Motor de deduplicación y cifrado | Fragmentación (chunking) FastCDC, Zstd, AES-256-GCM, KDF scrypt |
| Arquitectura servidor / agente | gRPC sobre TLS; el agente grueso (thick agent) fragmenta y comprime localmente |
| Dashboard web | Tareas, agentes, almacenamiento, restauraciones, progreso vía SSE |
| Respaldos incrementales y full sintético | Reutilización de la instantánea (snapshot) padre, reescaneo completo periódico |
| Programación avanzada | Intervalo, cron de 5 campos, con zona horaria y manejo del horario de verano (DST) |
| Descubrimiento automático de agentes | Escaneo de red en busca de agentes no registrados |
| Restauración de instantáneas | Exploración del árbol de archivos, restauración granular y de rutas completas |
| Verificación de restauración | Comprobaciones automáticas de restauración después de cada respaldo, por tarea |
| Instantáneas consistentes | VSS (Windows), LVM y btrfs (Linux) |
| Recuperación ante desastres guiada | Restauración completa del sistema con varias tareas sobre un host reconstruido |
| Benchmarks | Arnés reproducible de respaldo/restauración, comparación opcional con restic |
| Ransomware Sentinel | Telemetría de Gemini sobre instantáneas del sistema de archivos (opcional) |

## Ahora { #now }

| Elemento | Objetivo |
|----------|----------|
| **Restauración bare-metal** | Medios de recuperación arrancables más los metadatos necesarios para reconstruir un sistema arrancable; vea más abajo |

## Próximamente { #next }

| Elemento | Objetivo |
|----------|----------|
| **Restauración remota de sistemas de archivos** | Restaurar en un agente de otra red o en una VM en la nube |
| **Rendimiento de la restauración** | Flujos de restauración en paralelo y reanudación tras una interrupción |

## Restauración bare-metal (en diseño) { #bare-metal-restore-in-design }

El asistente actual de [recuperación ante desastres](../user-guide/disaster-recovery.md)
restaura archivos en un host que **ya arranca** y que ya ejecuta un agente registrado.
Una verdadera restauración bare-metal requiere además:

1. **Medios de arranque**: una imagen arrancable de Linux/WinPE que incluya el agente, para
   poder recuperar una máquina con el disco vacío.
2. **Metadatos de archivo más completos**: descriptores de seguridad de Windows y
   ACL/xattrs POSIX (incluidos los contextos de SELinux), que el formato actual de
   instantáneas no captura.
3. **Captura del diseño de disco**: tabla de particiones, UUID de los sistemas de archivos y
   configuración de LVM, para poder recrear las particiones y que `/etc/fstab` siga resolviendo.
4. **Reconstrucción del cargador de arranque**: GRUB/EFI en Linux y BCD en Windows.

Restaurar solo los archivos en un disco vacío produce un sistema **que no arranca**, por lo
que estas piezas se entregarán juntas y no como funcionalidades independientes.

## Eliminado de forma permanente (v2.0.0) { #permanently-removed-v200 }

Estos elementos **no** están pospuestos: se eliminaron del producto y no está previsto
que regresen:

- Respaldo de Microsoft 365 y Google Workspace
- Respaldo de bases de datos (PostgreSQL, MySQL, MariaDB, SQL Server, Oracle)
- Respaldo sin agente de VM en la nube (GCP, AWS, Azure) y VMware/vSphere
- El tipo de plugin recolector de cargas de trabajo (workload collector) que los atendía

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
