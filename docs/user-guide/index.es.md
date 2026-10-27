# Guía del usuario

La Guía del usuario de HitStor cubre todo lo que necesita saber para proteger sus datos con respaldos cifrados y deduplicados. Ya sea que respalde una sola laptop o que administre políticas de retención en varios servidores, esta guía lo acompaña en cada paso.

## Temas { #topics }

| Guía | Descripción |
|------|-------------|
| [Primeros pasos](getting-started.md) | Tutorial completo que lo lleva desde la inicialización de un repositorio hasta la restauración de archivos. |
| [Respaldo](backup.md) | Cree respaldos, excluya archivos, use etiquetas, ajuste el paralelismo y comprenda la deduplicación. |
| [Restauración](restore.md) | Restaure instantáneas (snapshots) completas o archivos individuales, resuelva IDs de instantáneas y verifique los datos restaurados. |
| [Instantáneas](snapshots.md) | Liste, filtre, compare e inspeccione las instantáneas almacenadas en su repositorio. |
| [Retención](retention.md) | Defina políticas de retención, elimine instantáneas antiguas con `forget` y recupere espacio con `prune`. |
| [Configuración](configuration.md) | Administre el archivo de configuración, las variables de entorno y el asistente interactivo de configuración. |
| [Plugins](plugins.md) | Descubra, instale y configure plugins de backends de almacenamiento, notificadores, hooks y escáneres. |

## Requisitos previos { #prerequisites }

Antes de seguir las guías de esta sección, asegúrese de tener HitStor instalado. Consulte la [Guía de instalación](../installation/index.md) para ver las instrucciones específicas de cada plataforma.

## Convenciones { #conventions }

En toda la Guía del usuario se usan las siguientes convenciones:

- Los comandos precedidos por `$` se ejecutan en una terminal.
- El texto en `UPPER_CASE` (como `SNAPSHOT_ID`) representa un valor que debe reemplazar por el suyo.
- Los flags opcionales se muestran entre corchetes: `[--flag VALUE]`.
- Los comandos largos se dividen en varias líneas con `\` para facilitar la lectura.
