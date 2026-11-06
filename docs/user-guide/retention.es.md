# Retención

A medida que realiza respaldos periódicos, la cantidad de instantáneas (snapshots) crece de forma constante. El comando `hitstor forget` olvida (forget), es decir, elimina, las instantáneas que ya no cumplen una política de retención, y `hitstor prune` depura (prune) el repositorio para recuperar el espacio en disco ocupado por datos que ya no referencia ninguna de las instantáneas restantes.

## Eliminar instantáneas individuales { #removing-individual-snapshots }

Puede eliminar instantáneas específicas pasando directamente sus ID:

```console
hitstor forget --repo /path/to/repo --password "secret" abc12345 def67890
removed snapshot abc12345
removed snapshot def67890
```

Esto es útil para limpiezas puntuales, pero para el mantenimiento habitual resulta más práctico un enfoque basado en políticas.

## Retención basada en políticas { #policy-based-retention }

En lugar de indicar ID individuales, defina reglas que describan **qué instantáneas conservar**. Todo lo que no coincida con ninguna regla se elimina.

### Flags de política disponibles { #available-policy-flags }

| Flag | Conserva |
|------|-------|
| `--keep-last N` | Las N instantáneas más recientes |
| `--keep-hourly N` | La instantánea más reciente de cada una de las últimas N horas que tengan una instantánea |
| `--keep-daily N` | La instantánea más reciente de cada uno de los últimos N días que tengan una instantánea |
| `--keep-weekly N` | La instantánea más reciente de cada una de las últimas N semanas que tengan una instantánea |
| `--keep-monthly N` | La instantánea más reciente de cada uno de los últimos N meses que tengan una instantánea |
| `--keep-yearly N` | La instantánea más reciente de cada uno de los últimos N años que tengan una instantánea |
| `--keep-tag TAG` | Todas las instantáneas que tengan la etiqueta indicada |

### Ejemplos { #examples }

Conservar las 7 instantáneas más recientes y una instantánea diaria de los últimos 30 días:

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --keep-last 7 \
    --keep-daily 30
```

Conservar una instantánea por mes durante un año, más todas las instantáneas con la etiqueta `important`:

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --keep-monthly 12 \
    --keep-tag important
```

## Semántica de unión { #union-semantics }

Las políticas de retención usan **semántica de unión**: una instantánea se conserva si coincide con **cualquiera** de las reglas activas. Las reglas no se aplican de forma secuencial ni excluyente; se evalúan de forma independiente y sus resultados se combinan.

Por ejemplo, con `--keep-last 3 --keep-daily 7`:

- Una instantánea se conserva si es una de las 3 más recientes **o** si es la última instantánea de uno de los últimos 7 días calendario.
- Una instantánea que cumple ambas reglas también se conserva (las reglas no se anulan entre sí).

## Cadenas de respaldo de cargas de trabajo { #workload-backup-chains }

Las instantáneas de cargas de trabajo (por ejemplo, de [SQL Server](sql-server.md))
llevan las etiquetas `plugin:<id>` y `kind:<tipo>`, y una etiqueta
`obj:<objeto>` por cada objeto (base de datos) que contienen. La política se
aplica a las instantáneas en conjunto y también a los respaldos propios de cada
objeto, de modo que cada base de datos conserva sus últimos N respaldos, sus
respaldos diarios, etc., sin importar lo que ocurra con las demás.

Los respaldos diferenciales y de log solo se pueden restaurar junto con los
respaldos en los que se basan. Cuando una política conserva uno, HitStor
también conserva, para el mismo host, plugin y objeto, los respaldos que
necesita, aunque la política por sí sola los olvidaría: un diferencial conserva
su respaldo completo base; un respaldo de log conserva los respaldos de log
anteriores hasta su respaldo completo base. Los respaldos completos de solo
copia (copy-only) nunca se usan como base.

## Filtrado por host y ruta { #host-and-path-filtering }

De forma predeterminada, `forget` aplica la política de retención a **todas** las instantáneas del repositorio como un solo conjunto: las instantáneas no se agrupan por host ni por ruta, así que `--keep-last 7` conserva las 7 instantáneas más recientes en total. Para aplicar una política a un host o una ruta, acote el alcance con filtros:

### Filtrar por host { #filter-by-host }

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --host webserver \
    --keep-daily 14
```

Solo se evalúan las instantáneas registradas desde `webserver`. Las instantáneas de otros hosts no se modifican.

### Filtrar por ruta { #filter-by-path }

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --path /var/www \
    --keep-last 5
```

Solo se evalúan las instantáneas cuya ruta de respaldo es `/var/www`.

Ambos flags se pueden combinar para apuntar a una combinación específica de host y ruta.

## Simulación (dry run) { #dry-run }

Antes de adoptar una política de retención, previsualice su efecto con `--dry-run`:

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --keep-last 7 \
    --keep-daily 30 \
    --dry-run
```

El comando muestra cada instantánea con `keep` (conservar) o `forget` (olvidar), sin borrar nada realmente. Ejecute siempre `--dry-run` primero cuando ajuste una política nueva.

Ejemplo de salida de una simulación:

```
keep   abc12345 2026-02-22 08:00:11
keep   bcd23456 2026-02-21 08:00:09
...
keep   cde34567 2026-01-24 08:00:12
forget def45678 2026-01-23 08:00:10
forget ef567890 2026-01-22 08:00:07
snapshots: 30 kept, 2 forgotten
```

## Depuración automática { #automatic-prune }

Puede combinar `forget` y `prune` en un solo paso agregando el flag `--prune`:

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --keep-last 7 \
    --keep-daily 30 \
    --prune
```

Esto elimina las instantáneas que quedan fuera de la política de retención y recupera de inmediato el espacio en disco de los datos sin referencias. Equivale a ejecutar `forget` seguido de `prune` como comandos separados.

## Depuración como paso separado { #separate-prune-step }

Si prefiere separar la eliminación de instantáneas de la recuperación de espacio, ejecute `prune` como un comando independiente:

```console
hitstor prune --repo /path/to/repo --password "secret"
```

`prune` busca en el repositorio los fragmentos (chunks) de datos que ya no referencia ninguna instantánea y los elimina. También reempaqueta los archivos de paquete parcialmente usados para reducir la fragmentación.

### Limitar el tamaño del reempaquetado { #limiting-repack-size }

En repositorios muy grandes, el reempaquetado puede implicar una E/S considerable. Use `--max-repack-size` para limitar la cantidad de datos reempaquetados en una sola ejecución:

```console
hitstor prune --repo /path/to/repo --password "secret" \
    --max-repack-size 5GiB
```

Esto procesa hasta 5 GiB de datos de paquetes por invocación. Ejecute `prune` de nuevo más tarde para continuar donde se quedó. Este enfoque es útil para repartir el costo de E/S entre varias ventanas de mantenimiento.

## Ejemplos de estrategias de retención { #example-retention-strategies }

### Estación de trabajo de desarrollo { #development-workstation }

Conserve el historial reciente para reversiones rápidas, pero sin acumular meses de instantáneas:

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --keep-last 3 \
    --keep-daily 7 \
    --keep-weekly 4 \
    --prune
```

- Siempre tiene disponibles las últimas 3 instantáneas.
- Una instantánea por día durante la última semana.
- Una instantánea por semana durante el último mes.

### Servidor de producción { #production-server }

Mantenga un historial más largo con granularidad mensual para auditorías:

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --host production-db \
    --keep-last 7 \
    --keep-daily 30 \
    --keep-weekly 12 \
    --keep-monthly 24 \
    --prune
```

- Las últimas 7 instantáneas para recuperación inmediata.
- Instantáneas diarias del último mes.
- Instantáneas semanales de los últimos 3 meses.
- Instantáneas mensuales de los últimos 2 años.

### Cumplimiento normativo / archivo a largo plazo { #compliance-long-term-archival }

Las regulaciones pueden exigir conservar los datos durante varios años:

```console
hitstor forget --repo /path/to/repo --password "secret" \
    --keep-daily 90 \
    --keep-monthly 36 \
    --keep-yearly 7 \
    --keep-tag compliance \
    --prune
```

- Instantáneas diarias de los últimos 90 días.
- Instantáneas mensuales de los últimos 3 años.
- Instantáneas anuales de los últimos 7 años.
- Toda instantánea con la etiqueta `compliance` se conserva indefinidamente.

!!! tip
    Programe `forget --prune` como tarea de cron o temporizador de systemd para que las políticas de retención se apliquen automáticamente. Consulte la [Guía de administración](../admin-guide/index.md) para ver recetas de automatización.
