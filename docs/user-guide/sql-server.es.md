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

En el dashboard, abra el árbol de selección (en **Nueva tarea de respaldo**,
expanda la máquina y **Microsoft SQL Server**, o en una restauración con plugin)
y elija **Configurar** junto a la instancia:

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
recuperación SIMPLE.

### Plan de respaldo { #backup-plan }

Una tarea respalda una selección con un **plan de respaldo**: cada tipo de
respaldo tiene su propia programación, y los diferenciales y logs se basan en
los respaldos completos de la propia tarea. El asistente propone un plan
habitual:

| Tipo | Programación por defecto | Notas |
|------|--------------------------|-------|
| Completo | Domingo 01:00 | Inicia la cadena |
| Diferencial | Lunes a sábado 01:00 | Cambios desde el último completo de la tarea |
| Log de transacciones | Apagado; p. ej. cada 15 minutos | Requiere el modelo de recuperación FULL |

Cada tipo se puede apagar (solo a demanda). Si dos tipos coinciden a la misma
hora, se ejecuta el completo y cubre al diferencial. **Respaldar ahora**
pregunta qué tipo ejecutar, y cada fila del plan en la página de la tarea tiene
su propio **Ejecutar ahora**.

En el dashboard, elija **Nueva tarea de respaldo**, expanda la máquina y
**Microsoft SQL Server** en el árbol de selección, y marque la instancia o bases
individuales. El paso siguiente define el repositorio y el plan. Vea
[Tareas de respaldo](../admin-guide/dashboard.md#backups).

La misma tarea con la API del dashboard:

```bash
curl -X POST https://<servidor>:8420/api/backups -H 'Content-Type: application/json' -d '{
  "name": "Sales", "agent_name": "sql01", "repo_name": "default",
  "job_type": "workload", "plugin": "mssql", "selection": ["MSSQLSERVER/Sales"],
  "plan": [
    {"kind": "full", "enabled": true, "cron_expression": "0 1 * * 0", "timezone": "America/Panama"},
    {"kind": "differential", "enabled": true, "cron_expression": "0 1 * * 1,2,3,4,5,6", "timezone": "America/Panama"},
    {"kind": "log", "enabled": true, "cron_expression": "*/15 * * * *", "timezone": "America/Panama"}
  ]
}'
```

`POST /api/backups/<id>/run` con `{"kind": "full"}` ejecuta un tipo de inmediato.

### Se requiere un respaldo completo { #full-required }

Antes de un diferencial o un log, el plugin comprueba en `msdb` que el respaldo
se podrá restaurar desde HitStor. Si no es así, la ejecución falla **antes de
respaldar nada** y la página de la tarea ofrece **Ejecutar respaldo completo
ahora**:

| Motivo | Significado |
|--------|-------------|
| `no-full` | No hay un completo desde que se creó la base o se pasó a FULL |
| `external-full` | El completo del que depende lo hizo otra herramienta (la página muestra cuándo, quién y dónde) |
| `external-log` | Otra herramienta hizo el último respaldo de log y dejó un hueco en la cadena de HitStor |
| `unknown-base` | `msdb` no tiene registro del completo del que depende (por ejemplo, una copia restaurada) |

Si la selección tiene varias bases, primero se revisan todas y el mensaje lista
cada una que necesita un completo. SQL Server aceptaría muchos de estos
respaldos (un diferencial sobre el completo de otra herramienta, por ejemplo),
pero HitStor no podría restaurarlos con sus propios respaldos.

### Cadena de respaldos { #backup-chain }

La página de la tarea muestra la **cadena de respaldos** de cada base: cada
completo, los diferenciales basados en él (su LSN base es el LSN de checkpoint
del completo) y los logs posteriores, con hasta cuándo restaura la cadena. Los
huecos en la cadena de log se marcan, y los respaldos que dependen de un
completo que HitStor no tiene se listan aparte. La pantalla de restauración
muestra el mismo vínculo en su columna **Basado en**.

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

Abra la instantánea de un respaldo de SQL Server (**Restauraciones**, o las
instantáneas de la tarea) y elija **Restaurar con el plugin**, o abra
**Restauraciones → Restaurar desde un respaldo con plugin**. La restauración
funciona como la de SQL Server Management Studio:

1. **Árbol de restauración.** El agente, sus instancias y las bases
   respaldadas, cada una con su último respaldo. Elija la base.
2. **Conjuntos de respaldo.** Todos los respaldos de esa base. Restaure al
   respaldo seleccionado, al punto más reciente o a un punto en el tiempo.
3. **Destino.** El agente y la instancia donde restaurar (por defecto la
   original), y **Restaurar como**: con otro nombre, la base existente no se
   toca.
4. **Archivos.** Cada archivo de la base con su ruta original y dónde se
   restaurará. Un nombre nuevo u otra instancia llevan los archivos a las
   carpetas de datos y de log predeterminadas de la instancia, con el nombre de
   la nueva base; escriba una ruta para poner un archivo en otro lugar.
5. **Resumen.** Antes de ejecutar nada, el plugin comprueba la restauración en
   la instancia de destino: los respaldos que aplicará y las advertencias, como
   una base con ese nombre que ya existe (active **Reemplazar** para
   sobrescribirla) o un archivo que ya usa otra base.

El servidor reúne los respaldos de la base desde las instantáneas; el plugin
elige el respaldo completo más reciente terminado antes del punto objetivo, el
diferencial más reciente basado en él y la cadena continua de logs hasta el
objetivo, que aplica con `STOPAT`. **Restaurar como archivos**, en la página de
la instantánea, restaura en cambio los archivos `.bak`.

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
| `instance` | Instancia original | Instancia donde se restaura |
| `database` | Nombre original | Nombre con el que se restaura |
| `replace` | `false` | Sobrescribe una base existente con ese nombre |
| `recovery` | `true` | Deja la base en línea; `false` la deja en RESTORING para restauraciones manuales posteriores |
| `relocate` | `true` al cambiar el nombre o la instancia | Mueve los archivos a las carpetas de datos y log predeterminadas de la instancia, con el nombre de la nueva base |
| `data_path`, `log_path` | Valores de la instancia | Carpetas para los archivos reubicados |
| `file:<nombre lógico>` | | Ruta completa de un archivo, p. ej. `file:Sales_log` = `L:\Logs\Sales_copy_log.ldf` |

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
