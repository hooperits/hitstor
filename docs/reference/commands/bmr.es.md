# hitstor bmr

Crea e inspecciona medios de recuperación bare-metal.

## Sinopsis { #synopsis }

```
hitstor bmr check [flags]
hitstor bmr generate-config [flags]
hitstor bmr build-media [flags]
```

## Descripción { #description }

Los medios de recuperación bare-metal permiten que una máquina con un disco vacío arranque, recree la distribución de sus discos y recupere sus datos desde HitStor.

Estos comandos se ejecutan **en el propio host protegido**: la imagen incluye el kernel, los controladores y la distribución de discos de ese host, por lo que no se puede generar solo a partir de un respaldo. Crear los medios es una acción deliberada, no algo que un respaldo haga por usted: ejecútela después de que cambie la distribución del almacenamiento del host.

Solo para Linux; internamente, HitStor utiliza [Relax-and-Recover (ReaR)](https://relax-and-recover.org/).

El mismo árbol de comandos está disponible como `hitstor-agent bmr`; consulte [hitstor-agent bmr](../agent-commands/bmr.md) para conocer la única diferencia de comportamiento.

## Subcomandos { #subcommands }

### `check` { #check }

Informa todo lo que separa a este host de una imagen de rescate arrancable: si ReaR está instalado y en qué versión, qué herramientas auxiliares están presentes y si el comando tiene los privilegios que ReaR necesita.

Informa todas las carencias a la vez en lugar de detenerse en la primera, y termina con un código distinto de cero cuando el host no está listo, de modo que puede servir como condición en un script de aprovisionamiento.

```bash
hitstor bmr check
```

### `generate-config` { #generate-config }

Escribe `/etc/rear/local.conf` y el hook `EXTERNAL_RESTORE` que el sistema de rescate ejecuta una vez que ha recreado la distribución de discos.

No se escriben credenciales: el token de recuperación y, cuando corresponde, la contraseña del repositorio se solicitan en el momento de la recuperación, de modo que los medios se pueden almacenar sin que se conviertan en una vía de acceso al repositorio.

`build-media` vuelve a generar ambos archivos, por lo que ejecutar este comando por separado solo es necesario si desea revisarlos o editarlos manualmente antes.

```bash
hitstor bmr generate-config --server backup.internal:8420 --agent web-01
```

| Flag | Tipo | Predeterminado | Descripción |
|------|------|---------|-------------|
| `--stdout` | bool | false | Imprime la configuración en lugar de escribirla |

Además de los [flags compartidos de medios](#shared-media-flags) que se describen más abajo.

### `build-media` { #build-media }

Ejecuta ReaR para generar una imagen de rescate arrancable para este host, después de escribir la configuración que hace que su proceso de recuperación se dirija de vuelta a HitStor.

La creación tarda varios minutos y lee todos los discos de la máquina, por lo que es deliberadamente manual. Vuelva a crearla cada vez que cambie la distribución de discos del host: una imagen desactualizada recrea la distribución anterior.

```bash
hitstor bmr build-media --server backup.internal:8420 --agent web-01
```

| Flag | Tipo | Predeterminado | Descripción |
|------|------|---------|-------------|
| `--force` | bool | false | Crea la imagen aunque la verificación de preparación informe bloqueos |

Además de los [flags compartidos de medios](#shared-media-flags) que se describen más abajo.

## Flags compartidos de medios { #shared-media-flags }

Tanto `generate-config` como `build-media` aceptan:

| Flag | Tipo | Predeterminado | Descripción |
|------|------|---------|-------------|
| `--output-dir` | string | `/var/lib/hitstor/bmr` | Directorio en el que ReaR escribe la ISO terminada |
| `--agent-binary` | string | `/usr/bin/hitstor-agent` | Binario de `hitstor-agent` que se incluye en la imagen de rescate |
| `--hook-script` | string | `/usr/share/hitstor/bmr/hitstor-recover.sh` | Ruta donde se escribe el hook `EXTERNAL_RESTORE` de ReaR |
| `--config-path` | string | `/etc/rear/local.conf` | Ruta donde se escribe la configuración de ReaR generada |
| `--server` | string | | Dirección del servidor HitStor que se precarga en la solicitud de recuperación (`host:port`) |
| `--agent` | string | | Nombre del agente registrado que se precarga en la solicitud de recuperación |
| `--migration-mode` | bool | false | Permite recuperar en hardware distinto o en una máquina virtual (`MIGRATION_MODE` de ReaR) |
| `--copy` | stringArray | | Ruta absoluta adicional que se incluye en la imagen de rescate (repetible) |

## Notas { #notes }

- `check`, `generate-config` y `build-media` admiten el flag global `--json` para obtener una salida legible por máquinas.
- La recuperación en sí la realiza `hitstor-agent recover`, que se invoca automáticamente desde los medios de rescate una vez que ReaR ha recreado la distribución de discos; consulte [hitstor-agent recover](../agent-commands/recover.md).
- Consulte [Restauración bare-metal](../../user-guide/bare-metal-restore.md) para ver el flujo de trabajo completo, incluido lo que los medios de recuperación capturan y lo que no.
