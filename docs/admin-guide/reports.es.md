# Reportes

HitStor envía reportes en PDF por correo según una programación, y cualquiera de ellos se puede descargar en todo momento. Los administradores los gestionan en **Administración → Reportes**. Para enviarlos hace falta tener configurado el [correo](users.md#email); para descargarlos, no.

## Tipos { #types }

| Reporte | Abarca | Para |
|---------|--------|------|
| **Resumen de protección** | Los últimos 7 o 30 días | La dirección: ¿están protegidos los servidores? |
| **Estado diario de respaldos** | Las últimas 24 horas | El equipo que opera los respaldos: qué se ejecutó, qué falló y por qué |

Ambos empiezan con un veredicto, **Todo está protegido**, **Protegido, con advertencias** o **Se requiere acción**, y el motivo en una frase; siguen las cifras y la lista de lo que requiere atención.

El **resumen de protección** muestra la tasa de éxito, cuántas tareas están al día, los datos respaldados, el almacenamiento usado, un gráfico de ejecuciones por día (correctas, parciales, fallidas), lo que requiere atención, cada agente y tarea, y el uso y espacio libre de cada repositorio.

El **estado diario** muestra las ejecuciones de las últimas 24 horas, las fallas con su motivo (una misma falla repetida, como respaldos de log que fallan toda la noche, es una sola fila con su cantidad y su rango de horas), el estado de cada tarea, y cada ejecución con su duración y tamaño.

## Cuándo una tarea está al día { #objective }

Cada tipo de respaldo de una tarea (los archivos de una tarea de archivos; los respaldos completos, diferenciales y de log de un plan de base de datos) tiene un objetivo que sale de su programación: la espera más larga entre dos ejecuciones programadas, más la mitad. Un tipo que se respalda cada día debe tener un respaldo correcto de las últimas 36 horas; un respaldo de log cada hora, de los últimos 90 minutos. Una ejecución perdida ya se nota; una ejecución lenta, no.

| Estado | Significado |
|--------|-------------|
| **Protegido** | El último respaldo correcto está dentro del objetivo |
| **Advertencia** | Dentro del objetivo, pero la última ejecución falló, dejó objetos fuera, o falló su restauración de prueba |
| **Atrasado** | El último respaldo correcto es más antiguo que el objetivo |
| **Nunca respaldado** | Programado, pero todavía sin un respaldo correcto |
| **A demanda** | Sin programación, por lo tanto sin objetivo |

Un reporte también señala las alertas de ransomware activas y los discos de repositorio con menos del 10 % libre.

## Historial de ejecuciones { #history }

Las tareas recuerdan solo su último resultado, así que el servidor guarda para los reportes un historial de cada ejecución (correcta, parcial o fallida): un archivo por mes junto al registro de agentes, cifrado con la clave del repositorio, conservado 13 meses. Las ejecuciones correctas anteriores al historial se leen de los snapshots; las fallas se conocen solo desde que empezó, y los reportes lo indican.

## Reportes programados { #scheduled }

**Nuevo reporte programado** pide:

- **Tipo** y, para el resumen de protección, el **periodo** que abarca (últimos 7 o 30 días).
- **Destinatarios**: una dirección por línea, hasta 50. Cada reporte programado tiene los suyos, así que el estado diario puede ir al equipo de respaldos y el resumen semanal a la dirección.
- **Cuándo enviarlo**: el mismo selector de horarios de las tareas de respaldo, con su zona horaria.
- **Agentes**: algunos, o ninguno marcado para incluir todos, también los que se agreguen después.
- **Idioma** del reporte y de su correo.

El correo dice el veredicto en el asunto y en el cuerpo, y lleva el PDF adjunto. **Enviar ahora** lo envía de inmediato; **Ver** abre el PDF tal como se enviaría en ese momento. Si el servidor estaba detenido cuando tocaba enviar un reporte, se envía al arrancar si el atraso es menor de seis horas; los turnos más antiguos se omiten, y la lista muestra por qué no se envió.

Los reportes se guardan en el `config.yaml` del servidor, bajo `reports:` (vea la [referencia de configuración](../reference/config-reference.md#reports-reports)).
