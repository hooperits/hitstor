# Recuperación ante desastres (restauración guiada del sistema completo)

HitStor protege **sistemas de archivos**, no discos sin formato. La recuperación ante desastres consiste en reconstruir un host a partir de sus respaldos de archivos en hardware nuevo o reparado mediante un **asistente guiado del dashboard**.

## Qué es (y qué no es) { #what-this-is-and-is-not }

| Es | No es |
|----|--------|
| Restauración completa de todas las tareas de respaldo de un sistema | Creación de imágenes de disco a nivel de bloque |
| Restauración secuencial de la instantánea (snapshot) más reciente de cada tarea | Reinstalación automática del sistema operativo ni reparación del gestor de arranque |
| Modos de raíz de staging o de rutas originales | Importación de VM del hipervisor |

Después de instalar el sistema operativo y el agente de HitStor en el host de recuperación, el asistente restaura las rutas de aplicaciones y datos desde el repositorio.

Para una máquina con un **disco en blanco** (sin sistema operativo en el cual instalar
un agente ni tabla de particiones sobre la cual restaurar), consulte
[Restauración bare-metal](bare-metal-restore.md), que primero recrea la estructura del
disco y el gestor de arranque, y luego devuelve la restauración de los datos a este asistente.

## Requisitos previos { #prerequisites }

1. El host de recuperación tiene un sistema operativo compatible (Windows o Linux).
2. El **agente** de HitStor está instalado y **registrado** en el servidor de respaldos.
3. Hay suficiente espacio libre en disco para los datos (o un volumen de staging grande).
4. Las tareas de respaldo del sistema de origen se completaron al menos una vez (existen instantáneas).

Opcional, pero recomendado: tareas con la **verificación de restauración** en verde (consulte la página Respaldos).

## Asistente del dashboard { #dashboard-wizard }

Abra **Recuperación ante desastres** en la barra lateral (`/disaster`).

1. **Seleccionar sistema**: agente protegido (origen) con tareas e instantáneas.
2. **Revisar plan**: la instantánea más reciente de cada tarea, rutas, tamaño y estado de la última verificación.
3. **Agente de destino**: normalmente el host nuevo; puede ser el mismo agente después de reconstruirlo.
4. **Modo**
   - **Raíz de staging** (predeterminado): restaura en una ruta como `/recover/hostname/job-name` para revisarla y luego hacer la transición.
   - **Rutas originales**: escribe en las rutas en uso (detenga antes los servicios que bloquean archivos).
5. **Iniciar recuperación del sistema completo**: las restauraciones se ejecutan **una tarea tras otra** en el agente de destino.

El progreso aparece en la misma página (y en Restauraciones para cada sesión individual).

## API (operadores / automatización) { #api-operators-automation }

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/disaster/plans` | Resumen de los sistemas y su preparación |
| GET | `/api/disaster/plans/{agent}` | Plan completo + lista de verificación |
| POST | `/api/disaster/recover` | Iniciar la recuperación |
| GET | `/api/disaster/recoveries` | Listar las recuperaciones orquestadas |
| GET | `/api/disaster/recoveries/{id}` | Estado de una recuperación |

Ejemplo:

```bash
curl -sS -X POST "$SERVER/api/disaster/recover" \
  -H 'Content-Type: application/json' \
  -d '{
    "source_agent": "web-01",
    "target_agent": "web-01-new",
    "mode": "staging",
    "staging_root": "/recover/web-01"
  }'
```

## Después de la recuperación { #after-recovery }

1. Valide los datos de las aplicaciones en staging (o en las rutas en uso).
2. Haga la transición de DNS y servicios según sea necesario.
3. Reasigne o vuelva a crear las tareas de respaldo en el agente de recuperación.
4. Opcionalmente, ejecute `hitstor verify-restore <snapshot>` para una verificación de integridad adicional.

## Relacionado { #related }

- [Restauración bare-metal](bare-metal-restore.md): recuperar una máquina con un disco en blanco
- [Restaurar archivos](restore.md): restauración granular desde una sola instantánea
- [Agentes](../admin-guide/agent-deployment.md): instalar y registrar el host de recuperación
- [Benchmarks](../dev-guide/benchmarks.md): rendimiento esperado de la restauración
