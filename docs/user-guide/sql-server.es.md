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
| Autenticación | Por instancia: la cuenta del agente (predeterminada), una cuenta de Windows o un inicio de sesión SQL |

## Requisitos { #requirements }

- El agente de HitStor se ejecuta **en el mismo servidor que SQL Server**. VDI
  no funciona a través de la red.
- El paquete del plugin `hitstor-plugin-mssql-<versión>-windows-amd64.hpk`.
- Un inicio de sesión en el rol **`sysadmin`**. SQL Server lo exige para
  cualquier `BACKUP` o `RESTORE` hacia un dispositivo virtual (de lo contrario
  da el error 3265); roles más acotados como `db_backupoperator` no alcanzan
  para VDI.
- Para respaldos de log y restauración a un punto en el tiempo, bases en el
  modelo de recuperación **FULL** (o BULK_LOGGED). Las bases en SIMPLE solo
  admiten respaldos completos y diferenciales.

## Instalar el plugin { #install-the-plugin }

Desde el dashboard: cargue el paquete en **Configuración → Plugins** y luego,
en la página del agente, elija **Instalar** en la sección **Plugins**. El
agente verifica la firma del paquete antes de instalarlo.

O bien, en el servidor de SQL Server, en una consola con privilegios elevados:

```powershell
hitstor-agent plugin install hitstor-plugin-mssql-0.1.0-windows-amd64.hpk
hitstor-agent plugin browse mssql MSSQLSERVER
```

`browse` lista las bases de la instancia. Cuando el agente reporta el plugin
en su siguiente consulta de programación, el servidor ya puede crear tareas de
SQL Server para él.

## Autenticación { #authentication }

Como otras herramientas de respaldo empresariales, HitStor guarda las
credenciales de cada instancia de SQL Server **en el agente**, no en las
tareas: una tarea solo indica qué respaldar. La configuración se prueba antes
de guardarse, se almacena cifrada (DPAPI) en el almacén de plugins del agente,
y la contraseña nunca se vuelve a mostrar ni se envía al servidor.

En el dashboard, abra el árbol de objetos (en **Nueva tarea de respaldo →
Aplicación (plugin)** o en una restauración con plugin) y elija **Configurar**
junto a la instancia:

| Campo | Descripción |
|-------|-------------|
| **Autenticación** | `integrated`: la cuenta del servicio del agente (`LocalSystem`, que SQL Server ve como `NT AUTHORITY\SYSTEM`). `windows`: una cuenta de Windows. `sql`: un inicio de sesión de SQL Server |
| **Usuario** | El inicio de sesión SQL, o `DOMINIO\usuario` para una cuenta de Windows (`.\usuario` si es local) |
| **Contraseña** | Se guarda cifrada en el agente; déjela vacía para conservar la guardada |

La cuenta debe tener el rol **`sysadmin`** (vea [Requisitos](#requirements)).
**Probar y guardar** se conecta a la instancia y lista sus bases; la
configuración solo se guarda si eso funciona. **Usar la cuenta del agente** la
quita. Una instancia sin configuración guardada usa la cuenta del agente, así
que agregue `NT AUTHORITY\SYSTEM` a `sysadmin` o configure la instancia.

En el servidor, lo mismo con la CLI del agente (la contraseña se lee de la
entrada estándar):

```powershell
hitstor-agent plugin configure mssql MSSQLSERVER --set auth=sql --set username=hitstor_backup --secret password
```

Otras opciones del plugin, para la línea de comandos (`--setting`) y las tareas
creadas antes de que existiera la configuración por instancia:

| Clave | Predeterminado | Descripción |
|-------|----------------|-------------|
| `instance` | `MSSQLSERVER` | Instancia que se lista cuando no se encuentra ninguna en el registro |
| `server` | `localhost` | Equipo al que se conecta |
| `auth`, `username` | `integrated` | Credenciales para las instancias sin configuración guardada |
| `password_env` | `HITSTOR_MSSQL_PASSWORD` | Variable de entorno (en `agent.env`) con la contraseña para esas instancias |
| `encrypt` | `true` | Cifrado de la conexión |
| `trust_server_certificate` | `true` | Confía en el certificado de la instancia (la conexión es local) |

## Respaldar { #back-up }

El árbol de objetos lista las instancias de SQL Server instaladas en el
equipo (según el registro), con su versión del producto, edición y número de
versión. Al expandir una instancia se listan sus bases con su modelo de
recuperación y tamaño; las bases de sistema van al final, y las bases que no
están en línea se muestran pero no se pueden seleccionar.

Los objetos se escriben `<instancia>/<base de datos>`, por ejemplo
`MSSQLSERVER/Sales`, o solo `<instancia>` para respaldar **la instancia
completa**: todas las bases en línea en cada ejecución, incluidas las que se
creen después de la tarea. Los respaldos diferenciales de una instancia
completa omiten `master`, y los de log omiten las bases en el modelo de
recuperación SIMPLE y las que aún no tienen un respaldo completo (que inicia
su cadena de log). Una tarea respalda sus bases con un tipo de respaldo;
programe tareas separadas por tipo. Un plan habitual:

| Tarea | Tipo | Programación |
|-------|------|--------------|
| Completo semanal | `full` | Domingo 01:00 |
| Diferencial diario | `differential` | Todos los días 01:00 |
| Log | `log` | Cada 15 minutos |

En el dashboard, elija **Nueva tarea de respaldo → Aplicación (plugin)**,
seleccione el agente y el plugin `mssql`, marque la instancia o bases individuales en el
árbol de objetos y defina el tipo de respaldo, las opciones y la programación
cron.

La misma tarea con la API del dashboard:

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

En el dashboard, abra **Restauraciones → Restaurar desde un respaldo con
plugin**: elija el agente, la base y el punto más reciente o un punto en el
tiempo (haga clic en un respaldo de la lista para usar su fecha) y defina las
opciones de restauración.

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

- La [retención](retention.md#workload-backup-chains) conserva cada
  diferencial o log que se mantiene junto con los respaldos hasta el completo
  anterior. Trata cada respaldo completo como inicio de cadena, así que no
  mezcle completos `copy_only` en un plan de tareas que también toma
  diferenciales.
- Un stream VDI por base de datos; las bases de una tarea se respaldan una
  tras otra.

## Consulte también { #see-also }

- [Arquitectura de plugins de carga de trabajo](../dev-guide/plugin-architecture.md)
- [hitstor-agent plugin](../reference/agent-commands/plugin.md)
- [API HTTP del dashboard](../reference/dashboard-api.md)
