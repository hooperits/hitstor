# Programación de respaldos

HitStor admite varios enfoques para programar respaldos automáticos. Elija el método que mejor se adapte a su infraestructura.

## Programación gestionada por el servidor { #server-managed-scheduling }

El enfoque recomendado es la programación gestionada por el servidor, en la que las programaciones de respaldo se configuran de forma centralizada y los agentes consultan al servidor la hora de su próxima ejecución.

### Configurar desde el dashboard { #configure-from-the-dashboard }

1. Abra el [Dashboard](dashboard.md) en `https://SERVER_IP:8421`.
2. Vaya a **Respaldos** y haga clic en **+ Nueva tarea de respaldo**. El paso 2 del asistente
   define la programación con el selector de programación (vea [Selector de programación](#plugin-jobs-schedule-picker)); un
   origen de plugin recibe un [plan de respaldo](backup-jobs.md#backup-plans), con una programación por tipo de respaldo.
3. Para cambiar la programación de una tarea existente, ábrala y despliegue **Editar programación** (o **Editar plan**
   en una tarea de carga de trabajo). Usa el mismo selector; **Editar como expresión cron** cambia a la expresión directa.
4. El dashboard valida la expresión y la muestra en palabras junto con sus próximas ejecuciones.
5. Haga clic en **Guardar programación** (o **Guardar plan**).

El servidor guarda la programación de cada tarea junto con la tarea, en el registro de agentes. Los agentes que se ejecutan en modo daemon aplican los cambios automáticamente.

### Modos de programación { #schedule-modes }

#### Modo intervalo (heredado) { #interval-mode-legacy }

Las tareas creadas antes de las programaciones cron (o mediante la API con `schedule_hours`) se ejecutan cada N horas:

- **Cómo funciona**: `next_run = last_backup + interval_hours`; tras una ejecución fallida, el siguiente intento es 15 minutos después
- **Zona horaria**: no aplica (relativo a la hora del último respaldo)

El dashboard ya no crea tareas por intervalo. Al guardar una programación nueva en una de esas tareas, el intervalo se reemplaza por una expresión cron; el selector parte del equivalente más cercano.

#### Selector de programación { #plugin-jobs-schedule-picker }

Las tareas de respaldo, de archivos o de un plugin, definen su programación con un selector inspirado en los
conjuntos de programación de las herramientas de respaldo empresariales, y
guardan el resultado como una expresión cron:

| Opción | Configuración |
|--------|---------------|
| **Solo a demanda** | La tarea solo se ejecuta con **Respaldar ahora** |
| **Repetitiva → Todos los días** | Hora de inicio |
| **Repetitiva → Días de la semana** | Días, y opcionalmente semanas del mes (1.ª a 5.ª, Última); hora de inicio |
| **Repetitiva → Días del mes** | Días 1 a 31 y **Último día**; hora de inicio. Los días que un mes no tiene se omiten |
| **Repetitiva → Cada…** | Cada 5 a 30 minutos, o cada 1 a 12 horas en un minuto dado; opcionalmente solo entre dos horas y solo de lunes a viernes |

Cada programación tiene una zona horaria (en una tarea nueva, la del navegador
por defecto; **Hora local del servidor** usa la del servidor). El selector
muestra la programación en palabras y sus próximas ejecuciones, y **Editar
como expresión cron** cambia a la expresión directa. Una tarea nueva de
sistema de archivos parte de todos los días a las 2:00 a. m. En un plan de
respaldo, cada tipo parte de su programación habitual: completos los domingos
a la 1:00 a. m., diferenciales de lunes a sábado a la 1:00 a. m. (ambos
activados) y respaldos de log cada 15 minutos (desactivados hasta que usted
los active). En un plan, cada tipo es repetitivo o está desactivado (solo a
demanda).

#### Modo expresión cron { #cron-expression-mode }

Sintaxis cron estándar de 5 campos para una programación precisa y con zona horaria:

```
┌───────────── minute (0-59)
│ ┌───────────── hour (0-23)
│ │ ┌───────────── day of month (1-31)
│ │ │ ┌───────────── month (1-12)
│ │ │ │ ┌───────────── day of week (0-6, Sun=0)
│ │ │ │ │
* * * * *
```

**Sintaxis admitida**:

| Sintaxis | Ejemplo | Significado |
|--------|---------|---------|
| Comodín | `*` | Cualquier valor |
| Valor específico | `5` | Solo en 5 |
| Lista | `1,15` | En 1 y en 15 |
| Rango | `1-5` | Del 1 al 5 |
| Paso | `*/15` | Cada 15 unidades |
| Rango con paso | `0-30/10` | 0, 10, 20, 30 |
| Último día del mes | `L` (día del mes) | El último día de cada mes |
| N-ésimo día de la semana | `0#1` (día de la semana) | El primer domingo del mes (`#1` a `#5`) |
| Último día de la semana | `5L` (día de la semana) | El último viernes del mes |

No se aceptan nombres (`MON`, `JAN`), `7` para el domingo ni macros como `@daily`. Cuando el día del mes y el día de la semana están restringidos a la vez, se ejecuta cualquier día que cumpla **uno de los dos**, como en el cron estándar (`0 2 1 * 1` se ejecuta el día 1 y todos los lunes).

**Ejemplos comunes**:

| Expresión | Descripción |
|------------|-------------|
| `0 2 * * *` | Diariamente a las 2:00 a. m. |
| `0 3 * * 1-5` | Días laborables a las 3:00 a. m. |
| `30 1 * * 0` | Domingos a la 1:30 a. m. |
| `0 */6 * * *` | Cada 6 horas |
| `0 22 1 * *` | El día 1 de cada mes a las 10:00 p. m. |
| `*/15 * * * *` | Cada 15 minutos |
| `0 23 L * *` | El último día de cada mes a las 11:00 p. m. |
| `0 2 * * 0#1` | El primer domingo de cada mes a las 2:00 a. m. |

**Manejo de zonas horarias**:

- Seleccione una zona horaria en la lista desplegable (formato IANA, p. ej. `America/New_York`).
- Si no se selecciona ninguna zona horaria (**Hora local del servidor**), se usa la zona horaria local del servidor. Una zona horaria desconocida se reemplaza por UTC.
- Las expresiones cron se evalúan en la zona horaria seleccionada y los cambios de horario de verano se manejan automáticamente:
    - **Adelanto de hora**: si una hora programada cae en la hora que se omite, la ejecución de ese día se omite; la siguiente es el próximo día que coincida.
    - **Atraso de hora**: si una hora programada cae en la hora que se repite, el respaldo se ejecuta en la primera ocurrencia.

### Cómo funciona { #how-it-works }

1. El agente llama al RPC `GetSchedule` del servidor.
2. El servidor evalúa la programación (expresión cron, plan de respaldo o intervalo) y responde con la hora de la próxima ejecución (`next_run`). En un plan de respaldo indica además qué tipo de respaldo corresponde.
3. El agente espera hasta `next_run` y luego ejecuta el respaldo.
4. Cuando el respaldo termina, el agente vuelve a llamar a `GetSchedule` para obtener la siguiente ejecución.

Este enfoque permite a los administradores cambiar las programaciones desde el dashboard sin reiniciar ni reconfigurar los agentes.

### Recuperación de ventanas perdidas { #missed-window-recovery }

Si el servidor o el agente estaban fuera de línea cuando correspondía un respaldo:

- En la siguiente consulta, el servidor detecta que `now > next_run` e indica al agente que se ejecute de inmediato.
- Después del respaldo de recuperación, la programación avanza a la siguiente ocurrencia futura (sin ejecuciones dobles).
- Esto funciona en las programaciones por intervalo, cron y plan de respaldo.

Una ejecución fallida no se reintenta en cada consulta: una programación cron o un plan esperan su siguiente horario, y una tarea por intervalo se reintenta a los 15 minutos.

## Modo daemon del agente { #agent-daemon-mode }

El modo daemon es el componente del lado del cliente de la programación gestionada por el servidor. Inicie el agente en modo daemon:

```bash
hitstor-agent daemon
```

En modo daemon, el agente:

- Se ejecuta como un proceso de larga duración
- Consulta al servidor su programación de respaldo mediante el RPC `GetSchedule` (predeterminado: cada 5 minutos; `--poll-interval`, mínimo 1 minuto)
- Ejecuta respaldos cuando `now >= next_run_unix` para cualquier tarea habilitada
- Informa al servidor los resultados de los respaldos
- Maneja fallos transitorios de conexión con reintentos automáticos

### Servicio systemd (Linux) { #systemd-service-linux }

Habilite el daemon como servicio de systemd:

```bash
sudo systemctl enable hitstor-agent
sudo systemctl start hitstor-agent
```

La unidad de servicio ejecuta `hitstor-agent daemon --config /etc/hitstor/agent-config.yaml` con reinicio en caso de fallo.

### Servicio de Windows { #windows-service }

Registre el daemon como servicio de Windows:

```powershell
hitstor-agent service install
hitstor-agent service start
```

El instalador de Windows ya lo hace. El servicio se inicia automáticamente al arrancar el equipo. Consulte [Despliegue de agentes](agent-deployment.md) para ver las instrucciones completas de configuración en Windows.

## Temporizador de systemd (alternativa) { #systemd-timer-alternative }

Si prefiere una programación de estilo cron gestionada a nivel del sistema operativo en lugar de la programación gestionada por el servidor, use un temporizador de systemd que ejecute un `hitstor-agent backup` puntual.

!!! note "Nota"
    El paquete del agente también instala un `hitstor-agent.timer` (diario a las 02:00). Este activa `hitstor-agent.service`, que ejecuta el **daemon**, no un respaldo puntual: si el daemon ya está habilitado, el temporizador no tiene efecto. Para respaldos puntuales, use sus propias unidades, como se muestra a continuación.

### Crear las unidades { #create-the-units }

El agente ya debe poder conectarse al servidor: `/etc/hitstor/agent.env` contiene `HITSTOR_AGENT_SERVER` y `HITSTOR_AGENT_API_KEY` una vez registrado el agente.

```ini
# /etc/systemd/system/hitstor-backup.service
[Unit]
Description=HitStor one-shot backup
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
EnvironmentFile=/etc/hitstor/agent.env
ExecStart=/usr/bin/hitstor-agent backup /etc /home
```

```ini
# /etc/systemd/system/hitstor-backup.timer
[Unit]
Description=Run a HitStor backup daily at 2 AM

[Timer]
OnCalendar=*-*-* 02:00:00
Persistent=true

[Install]
WantedBy=timers.target
```

### Habilitar el temporizador { #enable-the-timer }

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now hitstor-backup.timer
```

### Personalizar la programación { #customize-the-schedule }

Cambie `OnCalendar` en el temporizador, o sobrescríbalo:

```bash
sudo systemctl edit hitstor-backup.timer
```

```ini
[Timer]
OnCalendar=
OnCalendar=*-*-* 03:00:00
```

### Sintaxis de OnCalendar { #oncalendar-syntax }

La directiva `OnCalendar` usa la sintaxis de eventos de calendario de systemd. Ejemplos comunes:

| Expresión | Programación |
|------------|----------|
| `*-*-* 02:00:00` | Diariamente a las 2:00 a. m. |
| `*-*-* 00/6:00:00` | Cada 6 horas (medianoche, 6 a. m., mediodía, 6 p. m.) |
| `*-*-* *:00:00` | Cada hora |
| `Mon *-*-* 03:00:00` | Todos los lunes a las 3:00 a. m. |
| `*-*-01 04:00:00` | El primer día de cada mes a las 4:00 a. m. |
| `*-*-* 02:00:00,14:00:00` | Dos veces al día, a las 2:00 a. m. y a las 2:00 p. m. |

Verifique la programación con `systemd-analyze`:

```bash
systemd-analyze calendar "*-*-* 02:00:00"
```

### Temporizador frente a modo daemon { #timer-vs-daemon-mode }

| Característica | Modo daemon | Modo temporizador |
|---------|-------------|------------|
| Origen de la programación | Servidor (dashboard) | Temporizador local de systemd |
| Gestión centralizada | Sí | No (por equipo) |
| Cambios dinámicos de programación | Sí (desde el dashboard) | Requiere editar systemd |
| Ciclo de vida del proceso | De larga duración | Se inicia en cada respaldo |
| Tareas de carga de trabajo (plugins) y restauraciones desde el dashboard | Sí | No |
| Conexión con el servidor | Persistente | Bajo demanda |

Se recomienda el modo daemon para la mayoría de los despliegues porque permite gestionar las programaciones de forma centralizada.

## Programador de tareas de Windows { #windows-task-scheduler }

En sistemas Windows donde el agente no está registrado como servicio de Windows, use el Programador de tareas como alternativa:

1. Abra el Programador de tareas (`taskschd.msc`).
2. Cree una nueva tarea básica.
3. Configure el desencadenador con la programación deseada (por ejemplo, diariamente a las 2:00 a. m.).
4. Configure la acción para iniciar un programa:
    - Programa: `hitstor-agent.exe` en la carpeta de instalación del agente
    - Argumentos: `backup C:\Users C:\Data`
5. Guarde la tarea.

La línea de comandos no lee `agent.env` (solo lo leen el daemon y el servicio), así que indique a la tarea el servidor y la clave: agregue `--server HOST:8420 --api-key KEY` a los argumentos, o defina `agent_server` y `agent_api_key` en un archivo de configuración que pase con `--config`.

Para la programación gestionada por el servidor en Windows, use en su lugar el modo daemon mediante el servicio de Windows.

## Buenas prácticas de programación { #scheduling-best-practices }

- **Use expresiones cron** en los despliegues de producción. Ofrecen una programación precisa y con zona horaria que maneja correctamente los cambios de horario de verano y los reinicios del servidor.
- **Escalone los horarios de respaldo** entre agentes para no sobrecargar el servidor con respaldos simultáneos. Con expresiones cron, use minutos distintos (p. ej., `0 2 * * *` para un agente y `15 2 * * *` para otro).
- **Use horas de baja actividad** (por ejemplo, de 01:00 a 05:00) para minimizar el impacto en las cargas de trabajo de producción.
- **Defina siempre una zona horaria** al usar expresiones cron para evitar ambigüedades entre servidores ubicados en distintos lugares.
- **Ajuste el umbral de inactividad** (`alert_stale_threshold`) a un valor ligeramente mayor que el intervalo de respaldo. Para respaldos diarios, 25 horas es un valor predeterminado razonable.
- **Monitoree la programación** mediante el dashboard y las métricas de Prometheus para asegurarse de que los respaldos se ejecuten a tiempo.

## Próximos pasos { #next-steps }

- [Despliegue de agentes](agent-deployment.md) -- instale agentes y configure las unidades de systemd
- [Monitoreo](monitoring.md) -- configure alertas para respaldos no realizados
- [Dashboard](dashboard.md) -- gestione las programaciones desde la interfaz web
