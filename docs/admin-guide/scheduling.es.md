# Programación de respaldos

HitStor admite varios enfoques para programar respaldos automáticos. Elija el método que mejor se adapte a su infraestructura.

## Programación gestionada por el servidor { #server-managed-scheduling }

El enfoque recomendado es la programación gestionada por el servidor, en la que las programaciones de respaldo se configuran de forma centralizada y los agentes consultan al servidor la hora de su próxima ejecución.

### Configurar desde el dashboard { #configure-from-the-dashboard }

1. Abra el [Dashboard](dashboard.md) en `http://SERVER_IP:8421`.
2. Vaya a **Respaldos** y haga clic en **+ Nueva tarea de respaldo** (o abra una tarea existente).
3. Elija un modo de programación:
    - **Intervalo**: ejecutar cada N horas (sencillo, sin tener en cuenta la zona horaria).
    - **Expresión cron**: usar la sintaxis cron estándar de 5 campos para una programación precisa con soporte de zona horaria.
4. Si usa el modo cron, ingrese la expresión y seleccione una zona horaria. El dashboard valida la expresión y muestra una descripción legible junto con las próximas 3 ejecuciones.
5. Guarde la configuración.

El servidor almacena la configuración de la programación en el registro de agentes. Los agentes que se ejecutan en modo daemon aplican los cambios automáticamente.

### Modos de programación { #schedule-modes }

#### Modo intervalo { #interval-mode }

La opción de programación más sencilla. Defina el número de horas entre respaldos:

- **Cómo funciona**: `next_run = last_backup + interval_hours`
- **Zona horaria**: no aplica (relativo a la hora del último respaldo)
- **Ideal para**: configuraciones sencillas en las que la hora exacta de ejecución no importa

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

**Ejemplos comunes**:

| Expresión | Descripción |
|------------|-------------|
| `0 2 * * *` | Diariamente a las 2:00 a. m. |
| `0 3 * * 1-5` | Días laborables a las 3:00 a. m. |
| `30 1 * * 0` | Domingos a la 1:30 a. m. |
| `0 */6 * * *` | Cada 6 horas |
| `0 22 1 * *` | El día 1 de cada mes a las 10:00 p. m. |
| `*/15 * * * *` | Cada 15 minutos |

**Manejo de zonas horarias**:

- Seleccione una zona horaria en la lista desplegable (formato IANA, p. ej. `America/New_York`).
- Si no se selecciona ninguna zona horaria, se usa la zona horaria local del servidor.
- Las expresiones cron se evalúan en la zona horaria seleccionada y los cambios de horario de verano se manejan automáticamente:
    - **Adelanto de hora**: si una hora programada cae en la hora que se omite, el respaldo se ejecuta en la siguiente hora válida.
    - **Atraso de hora**: si una hora programada cae en la hora que se repite, el respaldo se ejecuta en la primera ocurrencia.

### Cómo funciona { #how-it-works }

1. El agente llama al RPC `GetSchedule` del servidor.
2. El servidor evalúa la programación (expresión cron o intervalo) y responde con la hora de la próxima ejecución (`next_run`).
3. El agente espera hasta `next_run` y luego ejecuta el respaldo.
4. Cuando el respaldo termina, el agente vuelve a llamar a `GetSchedule` para obtener la siguiente ejecución.

Este enfoque permite a los administradores cambiar las programaciones desde el dashboard sin reiniciar ni reconfigurar los agentes.

### Recuperación de ventanas perdidas { #missed-window-recovery }

Si el servidor o el agente estaban fuera de línea cuando correspondía un respaldo:

- En la siguiente consulta, el servidor detecta que `now > next_run` e indica al agente que se ejecute de inmediato.
- Después del respaldo de recuperación, la programación avanza a la siguiente ocurrencia futura (sin ejecuciones dobles).
- Esto funciona tanto en el modo intervalo como en el modo cron.

## Modo daemon del agente { #agent-daemon-mode }

El modo daemon es el componente del lado del cliente de la programación gestionada por el servidor. Inicie el agente en modo daemon:

```bash
hitstor-agent daemon
```

En modo daemon, el agente:

- Se ejecuta como un proceso de larga duración
- Consulta al servidor su programación de respaldo mediante el RPC `GetSchedule` (predeterminado: cada 5 minutos)
- Ejecuta respaldos cuando `now >= next_run_unix` para cualquier tarea habilitada
- Informa al servidor los resultados de los respaldos
- Maneja fallos transitorios de conexión con reintentos automáticos

### Servicio systemd (Linux) { #systemd-service-linux }

Habilite el daemon como servicio de systemd:

```bash
sudo systemctl enable hitstor-agent
sudo systemctl start hitstor-agent
```

La unidad de servicio ejecuta `hitstor-agent daemon` con reinicio en caso de fallo.

### Servicio de Windows { #windows-service }

Registre el daemon como servicio de Windows:

```powershell
hitstor-agent service install
hitstor-agent service start
```

El servicio se inicia automáticamente al arrancar el equipo. Consulte [Despliegue de agentes](agent-deployment.md) para ver las instrucciones completas de configuración en Windows.

## Temporizador de systemd (alternativa) { #systemd-timer-alternative }

Si prefiere una programación de estilo cron gestionada a nivel del sistema operativo en lugar de la programación gestionada por el servidor, use un temporizador de systemd.

### Habilitar el temporizador { #enable-the-timer }

```bash
sudo systemctl enable hitstor-agent.timer
sudo systemctl start hitstor-agent.timer
```

El temporizador activa la unidad de servicio `hitstor-agent`, que ejecuta un único respaldo y termina.

### Personalizar la programación { #customize-the-schedule }

Sobrescriba la programación predeterminada del temporizador:

```bash
sudo systemctl edit hitstor-agent.timer
```

Agregue una sección `[Timer]` con el valor de `OnCalendar` que desee:

```ini
[Timer]
OnCalendar=*-*-* 02:00:00
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
| Conexión con el servidor | Persistente | Bajo demanda |

Se recomienda el modo daemon para la mayoría de los despliegues porque permite gestionar las programaciones de forma centralizada.

## Programador de tareas de Windows { #windows-task-scheduler }

En sistemas Windows donde el agente no está registrado como servicio de Windows, use el Programador de tareas como alternativa:

1. Abra el Programador de tareas (`taskschd.msc`).
2. Cree una nueva tarea básica.
3. Configure el desencadenador con la programación deseada (por ejemplo, diariamente a las 2:00 a. m.).
4. Configure la acción para iniciar un programa:
    - Programa: `C:\Program Files\HitStor\hitstor-agent.exe`
    - Argumentos: `backup C:\Users C:\Data`
5. Guarde la tarea.

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
