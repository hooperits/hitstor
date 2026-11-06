# Arquitectura de plugins de carga de trabajo

> **Estado: implementado** (fases 1 a 4): framework de plugins e instalación
> remota, respaldo y restauración de cargas de trabajo, retención de cadenas
> de respaldo, el plugin de SQL Server y las páginas del dashboard basadas en
> los campos que declara cada plugin. El primer plugin construido sobre esta
> arquitectura respalda Microsoft SQL Server mediante la Virtual Device
> Interface (VDI). Reemplaza la entrada "tipo de plugin
> recolector de cargas de trabajo" de la sección
> [Eliminado definitivamente](../admin-guide/roadmap.md#permanently-removed-v200)
> de la hoja de ruta.

## Objetivos { #goals }

- **El núcleo no depende de ninguna carga de trabajo.** Un agente de HitStor
  sin plugins respalda archivos exactamente igual que hoy. El núcleo nunca
  interpreta datos de SQL Server ni de ninguna otra aplicación.
- **Los plugins son software opcional con su propia versión.** Un plugin se
  instala solo en los agentes que lo necesitan, se puede actualizar o
  desinstalar sin reinstalar el agente y se publica como un artefacto aparte.
- **Instalar un plugin agrega capacidades solo a ese agente.** El servidor y
  el dashboard ofrecen las tareas y restauraciones de una carga de trabajo
  únicamente para los agentes que reportan el plugin.
- **Agregar una nueva carga de trabajo no requiere cambios en el núcleo ni en
  el dashboard.** Los formularios, el selector de objetos y la planificación
  de la restauración los aporta el plugin.

Quedan fuera de la primera versión el respaldo sin agente, los plugins que se
ejecutan en el servidor y los mercados de plugins de terceros.

## Visión general { #overview }

```
            ┌──────────────── Servidor HitStor ───────────────┐
            │  Almacén de paquetes de plugins                  │
            │  Inventario por agente: plugins instalados y     │
            │  sus capacidades                                 │
            │  Tareas y restauraciones, dibujadas a partir     │
            │  del esquema que declara cada plugin             │
            └───────▲──────────────────────────┬───────────────┘
      inventario    │                          │ InstallPlugin (envío del paquete)
      (heartbeat)   │                          │ tareas, restauraciones
                    │                          ▼
┌───────────────────┴──────────── Agente ─────────────────────────────┐
│  Pipeline del núcleo: fragmentar · deduplicar · comprimir · cifrar ·  │
│  subir                                                                │
│  Gestor de plugins: instala, verifica la firma, versiona, desinstala  │
│     <directorio de datos>/plugins/<id>/<versión>/                     │
│                          │ gRPC sobre go-plugin, un proceso por tarea │
│               ┌──────────▼───────────┐                                │
│               │  hitstor-plugin-<id> │──► aplicación (p. ej. SQL VDI) │
│               └──────────────────────┘                                │
└───────────────────────────────────────────────────────────────────────┘
```

Un plugin nunca habla con el servidor. Lee y escribe en la aplicación local;
todo lo que toca el repositorio lo hace el agente.

## Paquete de plugin { #plugin-package }

Un plugin se distribuye como un archivo de paquete:
`hitstor-plugin-<id>-<versión>-<os>-<arch>.hpk`. Es un archivo zip con:

| Entrada | Propósito |
|---------|-----------|
| `plugin.yaml` | Manifiesto (ver abajo) |
| `bin/hitstor-plugin-<id>[.exe]` | Ejecutable del plugin |
| `LICENSE`, `README.md` | Documentación opcional |
| `SIGNATURE` | Firma ed25519 separada sobre el resumen canónico del paquete |

### Manifiesto { #manifest }

```yaml
id: mssql                      # único, en minúsculas, [a-z0-9-]
name: Microsoft SQL Server
version: 1.0.0                 # semver; independiente de la versión del agente
type: workload
protocol_version: 1            # versión del protocolo de carga de trabajo
min_agent_version: 0.2.0
platforms: [windows/amd64]
entrypoint: bin/hitstor-plugin-mssql.exe
publisher: hitstor             # selecciona la clave de confianza
```

Las capacidades (tipos de respaldo, opciones de restauración y campos de
formulario) no se duplican en el manifiesto. El agente se las pide al plugin
con `GetMetadata`, de modo que el binario es la única fuente de verdad.

### Firma y confianza { #signing-and-trust }

Los plugins se ejecutan con los privilegios del agente (`LocalSystem` en
Windows, `root` en Linux) y suelen tener credenciales administrativas de la
aplicación. Por eso el agente verifica cada paquete antes de instalarlo:

1. La firma debe validar contra una clave de publicador de confianza. La clave
   de publicación de HitStor viene incluida en el agente; los operadores pueden
   agregar sus propias claves (`plugin_trusted_keys`) para plugins internos.
2. `platforms` debe incluir el sistema operativo y la arquitectura del agente.
3. `min_agent_version` y `protocol_version` deben estar soportados.

Los paquetes sin firma se rechazan, salvo que el agente active de forma
explícita `plugin_allow_unsigned` (pensado solo para desarrollar plugins).

## Gestor de plugins del agente { #agent-plugin-manager }

- **Estructura:** `<directorio de datos>/plugins/<id>/<versión>/` contiene el
  paquete descomprimido. En Windows el directorio de datos es
  `C:\ProgramData\HitStor`. Solo los administradores y la cuenta de servicio
  del agente pueden escribir allí.
- **Registro:** `<directorio de datos>/plugins/installed-plugins.json` guarda
  de cada plugin el id, la versión activa, el estado (`active`, `degraded`,
  `unregistered`) y los metadatos que devolvió `GetMetadata` la última vez.
- **Instalación y actualización:** el paquete se descomprime en un directorio
  de versión nuevo y se verifica. Luego el plugin se lanza una vez para el
  intercambio `GetMetadata`, y solo entonces cambia la versión activa. Una
  actualización fallida deja activa la versión anterior. Las versiones viejas
  se eliminan después de un cambio exitoso, nunca mientras una tarea las use.
- **Desinstalación:** se rechaza mientras haya una tarea en curso. Las
  instantáneas (snapshots) creadas por el plugin permanecen en el repositorio
  y se pueden restaurar como archivos normales.
- **Inventario:** el agente informa al servidor los plugins instalados y sus
  metadatos cuando consulta su programación, así el servidor siempre sabe qué
  puede hacer cada agente.
- **Ejecución:** el plugin arranca como subproceso en cada tarea y termina al
  acabar. Si falla, solo falla esa tarea; el agente sigue funcionando.

### Canales de instalación { #installation-channels }

| Canal | Cómo |
|-------|------|
| Local | `hitstor-agent plugin install <archivo.hpk>`, además de `list`, `upgrade` y `remove` |
| Remoto | Se sube el paquete al servidor una vez y se instala desde el dashboard en los agentes elegidos. El servidor envía el paquete por un nuevo RPC `AgentService.InstallPlugin` y el agente aplica las mismas verificaciones que en una instalación local. |

## Protocolo de carga de trabajo { #workload-protocol }

Nuevo servicio `Workload` en `api/proto/hitstor/plugin/v1/workload.proto`,
servido sobre el mismo intercambio de go-plugin que los demás tipos:

| RPC | Propósito |
|-----|-----------|
| `GetMetadata` | Id, versión y versión de protocolo del plugin, capacidades y campos de formulario para tareas, restauraciones y configuración |
| `Configure` | Ajustes de conexión (instancia, autenticación) |
| `Browse` | Árbol de objetos seleccionables, p. ej. instancia → bases de datos, para el selector de objetos del dashboard |
| `Backup` | Recibe la selección y el tipo de respaldo; devuelve por stream uno o más **elementos**: nombre, bytes y un blob de metadatos opaco |
| `PlanRestore` | Recibe los metadatos de los elementos candidatos y un punto objetivo; devuelve la lista ordenada de elementos a aplicar (p. ej. completo → diferencial → logs) |
| `Restore` | Recibe en orden los bytes de los elementos planificados y los aplica con las opciones de restauración del formulario |

### Elementos en las instantáneas { #items-in-snapshots }

Cada elemento que produce un plugin se guarda en la instantánea como un
archivo virtual, bajo `/<id del plugin>/<ruta del objeto>/<nombre del elemento>`.
Por ejemplo:

```
/mssql/<instancia>/<base de datos>/full-20260101T020000Z.bak
/mssql/<instancia>/<base de datos>/full-20260101T020000Z.meta.json
```

El agente pasa el stream de cada elemento por el mismo pipeline de
fragmentación, deduplicación, compresión y cifrado que los archivos normales.
Así, la deduplicación, el cifrado, la retención, la verificación y la
replicación funcionan sin cambios. El blob de metadatos, que el núcleo nunca
interpreta, se guarda junto al elemento. La instantánea se etiqueta con
`plugin:<id>` y el tipo de respaldo.

Como los elementos son archivos comunes, también se pueden restaurar a una
carpeta con la restauración normal de archivos. Esa es la alternativa cuando
el plugin no está disponible.

### Flujo de restauración { #restore-flow }

1. El operador elige en el dashboard un agente, un objeto y un punto objetivo.
   El formulario de restauración proviene del esquema del plugin.
2. El servidor reúne los metadatos de los elementos candidatos de las
   instantáneas del plugin y se los envía al agente.
3. El agente le pide al plugin `PlanRestore` y devuelve el plan al servidor.
4. El servidor envía los elementos planificados al agente, que los pasa en
   orden al `Restore` del plugin.

El núcleo solo transporta bytes en el orden que pide el plugin. Toda la lógica
propia de la aplicación queda en el plugin, como las cadenas de respaldo, los
LSN y las opciones de recuperación.

### Retención { #retention }

Una política de retención no debe eliminar un elemento del que depende otro
elemento que se conserva (por ejemplo, un respaldo completo que necesitan
diferenciales o logs posteriores). El agente etiqueta las instantáneas de
carga de trabajo con `plugin:<id>` y `kind:<tipo>`; cuando una política
conserva una instantánea cuyo tipo no es `full`, `forget` también conserva las
instantáneas anteriores del mismo host y plugin hasta el completo previo. La
regla no necesita conocer nada de cada plugin y, ante la duda, conserva de
más.

## Servidor y dashboard { #server-and-dashboard }

- Las tareas de carga de trabajo (`job_type: workload`) guardan el id del
  plugin, los objetos seleccionados, el tipo de respaldo, la configuración del
  plugin y las opciones de respaldo como valores opacos; solo el plugin los
  interpreta.
- El dashboard dibuja los formularios de tareas, restauraciones y
  configuración a partir de los campos que cada plugin declara en
  `GetMetadata` (`FieldSpec`: clave, alcance, tipo `string`, `bool`, `number`
  o `choice`, etiquetas y ayuda por idioma, valor por defecto y opciones). Los
  plugins nunca traen HTML.
- La página del agente muestra los plugins instalados en ese agente, con
  acciones para instalar, actualizar y desinstalar. **Configuración →
  Plugins** administra los paquetes subidos. Se rechaza quitar un plugin que
  todavía usa una tarea.

## Primer plugin: SQL Server (VDI) { #first-plugin-sql-server-vdi }

| Aspecto | Decisión |
|---------|----------|
| Paquete | `hitstor-plugin-mssql-<versión>-windows-amd64.hpk` |
| Ubicación | `plugins/mssql/` en este repositorio, publicado como artefacto propio |
| Ruta de datos | VDI (`sqlvdi.dll`, `IClientVirtualDeviceSet2`) mediante COM en Go puro, sin cgo, más T-SQL con `go-mssqldb` |
| Tipos de respaldo | Completo (opcionalmente `COPY_ONLY`), diferencial y log de transacciones |
| Restauración | Cadena completa con `NORECOVERY`/`RECOVERY`, punto en el tiempo (`STOPAT`) y restauración con otro nombre usando `MOVE` |
| Autenticación | Por instancia, desde el árbol de objetos y guardada cifrada en el agente: la cuenta del servicio del agente, una cuenta de Windows o un inicio de sesión SQL (todas necesitan `sysadmin`). Nunca en las tareas ni en `config.yaml` |
| Metadatos | LSN (`first`, `last`, `checkpoint`, `database_backup_lsn`), horas del respaldo y modelo de recuperación, leídos de `msdb.dbo.backupset` |

## Fases de entrega { #delivery-phases }

1. **Framework de plugins:** formato de paquete, firma, gestor de plugins del
   agente, CLI local y reporte de inventario.
2. **Protocolo de carga de trabajo:** elementos como archivos virtuales en el
   pipeline del agente y tareas genéricas de plugin en el servidor.
3. **Plugin de SQL Server:** respaldo y restauración por VDI de todos los
   tipos, más punto en el tiempo.
4. **Dashboard y documentación:** instalación remota, selector de objetos,
   formularios basados en esquemas y actualización de la hoja de ruta.

## Consulte también { #see-also }

- [Guía de desarrollo de plugins](../reference/plugin-development.md): tipos
  de plugin existentes (almacenamiento, notificador, hook, escáner)
- [Panorama del código fuente](codebase-overview.md)
- [Hoja de ruta](../admin-guide/roadmap.md)
