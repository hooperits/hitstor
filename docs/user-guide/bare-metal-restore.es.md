# Restauración bare-metal

La [recuperación ante desastres guiada](disaster-recovery.md) reconstruye un host que **ya
arranca** y **ya ejecuta un agente registrado**. La restauración bare-metal cubre el
caso que aquella no puede: una máquina con el disco en blanco, donde no hay sistema operativo en el que instalar un
agente ni tabla de particiones donde colocar los archivos restaurados.

HitStor se apoya en [Relax-and-Recover (ReaR)](https://relax-and-recover.org/) para
las partes que no tienen que ver con los datos. ReaR genera el medio de rescate arrancable, recrea
el esquema de discos y reinstala el cargador de arranque; HitStor restaura el contenido.

!!! note "Por ahora, solo Linux"
    La recuperación bare-metal de Windows (medio WinPE, descriptores de seguridad NTFS,
    `bcdboot`) llegará en una versión posterior.

## Por qué el medio debe generarse por adelantado { #why-the-media-has-to-be-built-in-advance }

Una imagen de rescate incorpora el propio kernel del host, sus controladores y su esquema de discos.
Solo puede generarse **en la máquina que protege**, mientras esa máquina
sigue en funcionamiento, que es justamente lo que ya no tiene cuando la necesita.

Por eso, generar el medio de recuperación es una tarea deliberada y programada, no algo que un
respaldo haga por usted. Vuelva a generarlo cada vez que cambie el esquema de almacenamiento de un host: una
imagen desactualizada recrea el esquema *anterior*.

La imagen se guarda en caché en el servidor en lugar de obtenerse del host bajo demanda.
Un diseño que regenerara la ISO en el momento de la descarga necesitaría que el host de origen
estuviera en línea y tardaría varios minutos en hacerlo, lo que inutiliza la función justo en el
escenario para el que existe.

## Capturar lo que necesita una restauración bare-metal { #capturing-what-a-bare-metal-restore-needs }

El contenido de los archivos por sí solo no describe una máquina arrancable. Agregue `--bare-metal` a un
respaldo para capturar el resto:

```bash
hitstor backup /etc /home /var --bare-metal
```

Esto registra dos elementos además del árbol de archivos habitual:

- **Atributos extendidos**: en Linux, los contextos de SELinux, las capacidades de archivo y
  las ACL POSIX son todos xattrs, así que un solo mecanismo cubre los tres. Sin ellos, un
  sistema con SELinux se restaura en un sistema de archivos sin etiquetas cuyos servicios se niegan a
  iniciar.
- **El perfil del sistema**: tabla de particiones, UUID y etiquetas de los sistemas de archivos, LVM,
  RAID por software, `/etc/fstab` y el estado del cargador de arranque. Se almacena como un
  blob cifrado referenciado desde la instantánea (snapshot), de modo que el respaldo describe la forma de la
  máquina y no solo su contenido. El esquema puede reconstruirse incluso si se pierde el
  medio de rescate.

La captura funciona siempre según el mejor esfuerzo. Si falta una herramienta, esa sección queda vacía y
se registra una advertencia en lugar de hacer fallar el respaldo.

## Comprobar que un host puede generar el medio { #checking-a-host-can-build-media }

Ejecute esto en el host protegido:

```bash
sudo hitstor-agent bmr check
```

Informa de una sola vez todo lo que se interpone entre ese host y una imagen arrancable
(ReaR y su versión, herramientas auxiliares, privilegios) en lugar de detenerse en el
primer problema, y termina con un código distinto de cero cuando el host no está listo, por lo que puede servir de condición en un
script de aprovisionamiento.

El mismo estado de preparación aparece por host en **Recuperación ante desastres → Bare Metal** en
el dashboard.

## Generar el medio { #building-the-media }

Desde el dashboard, en **Recuperación ante desastres → Bare Metal**, pulse **Generar
medio** en un host. El servidor le pide al agente de ese host que ejecute ReaR y transmite de vuelta la
ISO resultante, mostrando la generación en tiempo real. Al terminar, la imagen
queda en caché en el servidor y aparece **Descargar**.

O bien, en el propio host:

```bash
sudo hitstor-agent bmr build-media --server backup.example.com:8420
```

Flags útiles:

| Flag | Propósito |
|------|---------|
| `--migration-mode` | Permite recuperar en hardware distinto o en una VM (P2V). ReaR reasigna los dispositivos y regenera el initramfs con los controladores que necesita el nuevo hardware. |
| `--copy PATH` | Incorpora un archivo adicional en la imagen de rescate; por ejemplo, un paquete de CA corporativo sin el cual el agente no puede verificar el servidor. |
| `--agent NAME` | Nombre del agente registrado que se precarga en el prompt de rescate, cuando difiere del nombre de host. |
| `--force` | Genera el medio a pesar de los impedimentos de preparación. |

`rear mkrescue` lee todos los discos de la máquina y tarda varios minutos, así que
ejecútelo en un momento en que la carga adicional sea aceptable.

Guarde la ISO en un lugar que sobreviva a la pérdida del host del que proviene.

## Qué no contiene el medio { #what-is-not-on-the-media }

**Ninguna credencial.** El token de recuperación y, cuando hace falta, la contraseña del repositorio
se ingresan en el momento de la recuperación.

Esto es intencional. Un medio extraíble que llevara una clave válida sería un
vector para comprometer el repositorio, y una ISO que sigue funcionando un año después de
generada, guardada en un estante, es justamente el riesgo que crea una credencial de larga duración.
El costo es que la recuperación desatendida no es posible a menos que usted la habilite expresamente.

## Recuperar una máquina { #recovering-a-machine }

1. **Descargue** la ISO de ese host desde la pestaña Bare Metal y arranque el
   hardware de reemplazo con ella.
2. De vuelta en el dashboard, pulse **Emitir token** para el host. El token aparece
   **una sola vez**: cópielo. Funciona una única vez, solo para ese host, y expira
   a las 24 horas de forma predeterminada. Si se filtra antes de usarse, **revóquelo**.
3. En la consola de rescate, confirme la dirección del servidor y el nombre del agente y luego escriba
   el token. ReaR recrea el esquema de discos, monta la nueva raíz en
   `/mnt/local` y el agente de recuperación se anuncia ante el servidor.
4. Vuelva a **Recuperación guiada** e inicie la restauración con el host anunciado
   como destino. Nada se inicia por sí solo: una restauración bare-metal la confirma
   un operador, como cualquier otra.
5. Cuando la restauración llega a un estado final, el agente de recuperación termina, ReaR
   instala el cargador de arranque y la máquina puede reiniciarse en el sistema restaurado.

### Cómo se llega al host de rescate { #how-the-rescue-host-is-reached }

Todas las demás restauraciones las inicia el servidor: el servidor ya conoce la
dirección de un agente y se conecta a él. Una máquina que acaba de arrancar desde un medio de rescate tiene la
dirección que le haya asignado DHCP, de la cual nadie le ha informado al servidor.

Por eso el sistema de rescate se comunica con el servidor, presentando su token de recuperación y su
dirección actual, y luego el servidor se conecta de vuelta por la ruta de restauración habitual,
sin cambios. El sistema de rescate genera su propio token de listener para esa
conexión de vuelta; el token del agente original nunca se revela y sigue siendo válido para
cuando arranque el sistema operativo restaurado.

!!! warning "El servidor debe poder alcanzar el host de rescate"
    La conexión de vuelta necesita una ruta directa hacia la dirección que anuncia el
    host de rescate. Eso se cumple en los casos de misma LAN y P2V que admite esta versión.
    Todavía no se admite un host de rescate detrás de un NAT agresivo a través de una WAN; la
    restauración falla al iniciar indicando la dirección que intentó, en lugar de quedarse colgada.

## API { #api }

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/bmr/hosts` | Estado de preparación por host y medio en caché |
| POST | `/api/bmr/media/{agent}/build` | Inicia una generación; devuelve un id de operación |
| GET | `/api/operations/{id}/events` | Progreso de la generación (SSE) |
| GET | `/api/bmr/media/{agent}/download` | Descarga la ISO en caché |
| DELETE | `/api/bmr/media/{agent}` | Descarta la ISO en caché |
| POST | `/api/bmr/prepare/{agent}` | Emite un token de recuperación (se devuelve una sola vez) |
| GET | `/api/bmr/tokens` | Lista los tokens pendientes |
| DELETE | `/api/bmr/tokens/{id}` | Revoca un token sin usar |

```bash
# Emitir un token con un plazo más corto que el predeterminado de 24h.
curl -sS -X POST "$SERVER/api/bmr/prepare/web-01" \
  -H 'Content-Type: application/json' \
  -d '{"ttl_hours": 2}'
```

## Solución de problemas { #troubleshooting }

**`bmr check` informa impedimentos.** Instale lo que indica. En la mayoría de las distribuciones
eso es `rear` más un generador de ISO (`xorriso` o `genisoimage`). El comando debe
ejecutarse como root: ReaR lee los dispositivos de bloque sin procesar.

**La generación falla con un error de ReaR.** El mensaje incluye el diagnóstico del propio ReaR.
Una imagen previamente en caché no se modifica: una regeneración que falla no le hace perder
el medio que ya tenía.

**El host de rescate se anuncia, pero la restauración no inicia.** El servidor no pudo
alcanzar la dirección anunciada. Revise el enrutamiento y los firewalls entre ambos,
y que el puerto en el que escucha el agente de rescate (8422 de forma predeterminada) esté abierto.

**El token es rechazado.** Los tokens son de un solo uso, expiran y están vinculados a un
host. Emita uno nuevo; el error indica cuál de las tres condiciones se aplicó.

## Relacionado { #related }

- [Recuperación ante desastres](disaster-recovery.md): restauración guiada en un host que arranca
- [Restaurar archivos](restore.md): restauración granular a partir de una sola instantánea
- [Agentes](../admin-guide/agent-deployment.md): instalar y registrar un agente
