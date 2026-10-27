# SQL Server

El plugin de carga de trabajo `mssql` respalda y restaura bases de datos de
Microsoft SQL Server mediante la **Virtual Device Interface (VDI)**. SQL Server
envía el respaldo directamente al pipeline de HitStor: no se escribe nada
intermedio en disco, y cada respaldo se fragmenta, deduplica, comprime y
cifra como cualquier otra instantánea (snapshot).

| Capacidad | Detalle |
|-----------|---------|
| Tipos de respaldo | Completo (opcionalmente `COPY_ONLY`), diferencial y log de transacciones |
| Restauración | Al punto más reciente o a un punto en el tiempo; con el nombre original o con otro |
| Plataformas | Windows (VDI solo existe en Windows); SQL Server 2016 o posterior |
| Autenticación | Integrada de Windows (predeterminada) o un inicio de sesión SQL |

## Requisitos { #requirements }

- El agente de HitStor se ejecuta **en el mismo servidor que SQL Server**. VDI
  no funciona a través de la red.
- El paquete del plugin `hitstor-plugin-mssql-<versión>-windows-amd64.hpk`.
- Una cuenta que pueda respaldar y restaurar: `sysadmin`, o
  `db_backupoperator` en cada base más `dbcreator` para restaurar.
- Para respaldos de log y restauración a un punto en el tiempo, bases en el
  modelo de recuperación **FULL** (o BULK_LOGGED). Las bases en SIMPLE solo
  admiten respaldos completos y diferenciales.

## Instalar el plugin { #install-the-plugin }

En el servidor de SQL Server, en una consola con privilegios elevados:

```powershell
hitstor-agent plugin install hitstor-plugin-mssql-0.1.0-windows-amd64.hpk
hitstor-agent plugin browse mssql MSSQLSERVER
```

`browse` lista las bases de la instancia. Cuando el agente reporta el plugin
en su siguiente consulta de programación, el servidor ya puede crear tareas de
SQL Server para él.

## Autenticación { #authentication }

El servicio del agente se ejecuta como `LocalSystem`, que SQL Server ve como
el inicio de sesión `NT AUTHORITY\SYSTEM`. Elija una opción:

- **Integrada (predeterminada).** Otorgue a `NT AUTHORITY\SYSTEM` los roles
  necesarios, o ejecute el servicio del agente con una cuenta de Windows
  dedicada que los tenga.
- **Inicio de sesión SQL.** Cree un inicio de sesión para HitStor y configure
  el plugin con `auth=sql` y `username=<inicio de sesión>`. Ponga la
  contraseña en el archivo de entorno del agente
  (`C:\ProgramData\HitStor\agent.env`):

  ```
  HITSTOR_MSSQL_PASSWORD=<contraseña>
  ```

  El nombre de la variable se puede cambiar con `password_env`. Nunca se
  aceptan contraseñas en la configuración de las tareas.

Configuración del plugin:

| Clave | Predeterminado | Descripción |
|-------|----------------|-------------|
| `instance` | `MSSQLSERVER` | Nombre de la instancia (`MSSQLSERVER` es la instancia predeterminada) |
| `server` | `localhost` | Equipo al que se conecta |
| `auth` | `integrated` | `integrated` o `sql` |
| `username` | | Inicio de sesión SQL, con `auth=sql` |
| `password_env` | `HITSTOR_MSSQL_PASSWORD` | Variable de entorno que contiene la contraseña |
| `encrypt` | `true` | Cifrado de la conexión |
| `trust_server_certificate` | `true` | Confía en el certificado de la instancia (la conexión es local) |

## Respaldar { #back-up }

Los objetos se escriben `<instancia>/<base de datos>`, por ejemplo
`MSSQLSERVER/Sales`. Una tarea respalda una o más bases con un tipo de
respaldo; programe tareas separadas por tipo. Un plan habitual:

| Tarea | Tipo | Programación |
|-------|------|--------------|
| Completo semanal | `full` | Domingo 01:00 |
| Diferencial diario | `differential` | Todos los días 01:00 |
| Log | `log` | Cada 15 minutos |

Cree las tareas con la API del dashboard (las páginas del dashboard llegan en
una versión posterior):

```bash
curl -X POST https://<servidor>:8420/api/backups -H 'Content-Type: application/json' -d '{
  "name": "Sales log", "agent_name": "sql01", "repo_name": "default",
  "job_type": "workload", "plugin": "mssql",
  "selection": ["MSSQLSERVER/Sales"], "backup_kind": "log",
  "schedule_enabled": true, "cron_expression": "*/15 * * * *"
}'
```

Cada respaldo se guarda en la instantánea como
`mssql/<instancia>/<base>/<tipo>-<fecha>.bak` (`.trn` para los logs), junto a
un archivo `.meta.json` con sus LSN, horas y archivos de la base.

Por defecto los respaldos se toman **sin compresión** para que los completos
consecutivos se dedupliquen; HitStor comprime los fragmentos de todos modos.

!!! warning "Otra herramienta de respaldo sobre la misma base"
    Los respaldos completos y de log que toma HitStor forman parte de la
    cadena de respaldos de la base. Si otra herramienta también la respalda,
    sus diferenciales y logs dependerán de los respaldos de HitStor y
    viceversa. En ese caso, deje que una sola herramienta sea dueña de la
    cadena y configure `copy_only` en la otra.

## Restaurar { #restore }

El servidor reúne los respaldos de la base desde las instantáneas; el plugin
elige el respaldo completo más reciente terminado antes del punto objetivo, el
diferencial más reciente basado en él y la cadena continua de logs hasta el
objetivo, que aplica con `STOPAT`.

Liste los respaldos disponibles:

```bash
curl 'https://<servidor>:8420/api/workload/points?agent=sql01&plugin=mssql&object=MSSQLSERVER/Sales'
```

Restaure una copia a un punto en el tiempo, con otro nombre:

```bash
curl -X POST https://<servidor>:8420/api/workload/restores -H 'Content-Type: application/json' -d '{
  "agent_name": "sql01", "plugin": "mssql", "object": "MSSQLSERVER/Sales",
  "target": "2026-03-01T14:30:00Z",
  "options": {"database": "Sales_restored"}
}'
```

Omita `target` para restaurar el punto más reciente. Use `source_agent` para
restaurar en `agent_name` los respaldos de otro servidor.

| Opción | Predeterminado | Descripción |
|--------|----------------|-------------|
| `database` | Nombre original | Nombre con el que se restaura |
| `replace` | `false` | Sobrescribe una base existente con ese nombre |
| `recovery` | `true` | Deja la base en línea; `false` la deja en RESTORING para restauraciones manuales posteriores |
| `relocate` | `true` al cambiar el nombre | Mueve los archivos a las carpetas de datos y log predeterminadas de la instancia, con el nombre de la nueva base |
| `data_path`, `log_path` | Valores de la instancia | Carpetas para los archivos reubicados |

Una restauración con el nombre original y sin `replace` falla si la base
existe, de modo que nunca se sobrescribe una base por accidente.

## Probar en el servidor { #try-it-on-the-host }

Sin servidor de HitStor, ejecute un respaldo hacia una carpeta y restaure
desde ella:

```powershell
hitstor-agent plugin test-backup mssql --select MSSQLSERVER/Sales --kind full `
    --option copy_only=true --out C:\Temp\sales-backup
hitstor-agent plugin test-restore mssql --from C:\Temp\sales-backup `
    --object MSSQLSERVER/Sales --option database=Sales_check
```

## Limitaciones { #limitations }

- La retención todavía no mantiene unidas las cadenas de respaldo: una
  política que olvide un respaldo completo puede dejar huérfanos los
  diferenciales y logs que dependen de él. Mantenga una retención que cubra un
  ciclo completo (por ejemplo, más de una semana con completos semanales).
- Un stream VDI por base de datos; las bases de una tarea se respaldan una
  tras otra.
- Las páginas del dashboard para tareas y restauraciones de SQL Server aún no
  están disponibles; use la API.

## Consulte también { #see-also }

- [Arquitectura de plugins de carga de trabajo](../dev-guide/plugin-architecture.md)
- [hitstor-agent plugin](../reference/agent-commands/plugin.md)
- [API HTTP del dashboard](../reference/dashboard-api.md)
