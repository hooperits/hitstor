# hitstor-agent recover

Ejecuta el agente en modo de recuperación bare-metal (se invoca desde el medio de rescate).

## Sinopsis { #synopsis }

```
hitstor-agent recover --token TOKEN [flags]
```

## Descripción { #description }

El modo de recuperación es lo que ejecuta el sistema de rescate después de que ReaR ha recreado el esquema de discos de la máquina y montado la nueva raíz.

No es un agente normal: no inicia ninguna programación, no escribe configuración y termina en cuanto la restauración para la que existe llega a un estado final.

Todas las demás restauraciones en HitStor las inicia el servidor conectándose a un agente cuya dirección ya conoce. Una máquina que acaba de arrancar desde un medio de rescate tiene la dirección que le haya asignado DHCP, de la cual nadie le ha informado al servidor; por eso este comando se comunica con el servidor, presenta un token de recuperación de un solo uso, y el servidor se conecta de vuelta por la ruta de restauración habitual.

ReaR instala un cargador de arranque solo si este comando termina con `0`.

## Flags { #flags }

| Flag | Tipo | Predeterminado | Descripción |
|------|------|---------|-------------|
| `--token` | string | | **Obligatorio.** Token de recuperación de un solo uso, emitido desde el dashboard (Recuperación ante desastres → Bare Metal) |
| `--agent` | string | nombre de host de este equipo | Nombre del agente registrado que se está recuperando |
| `--target` | string | `/mnt/local` | Directorio donde está montada la raíz recreada |
| `--listen-port` | int | 8422 | Puerto gRPC al que el servidor envía la restauración |
| `--timeout` | duration | 12h | Desistir si ninguna restauración se completa dentro de este plazo |

Además de `--server` y `--ca-cert` de los [flags globales](index.md) del agente.

## Ejemplos { #examples }

```bash
hitstor-agent recover --token abc123... --server backup.internal:8420
```

## Notas { #notes }

- Se niega a ejecutarse si `--target` no es un directorio existente: así se detecta el caso en que ReaR no montó realmente la raíz recreada, lo que de otro modo haría que la restauración escribiera silenciosamente en el ramdisk del sistema de rescate y desapareciera al reiniciar.
- El listener se autentica con un token generado para esta ejecución de recuperación; el token del agente original estaba en el disco que ya no existe.
- Los errores no fatales de restauración por archivo se informan como advertencias al terminar la restauración en lugar de hacer fallar el comando, ya que el código de salida determina si ReaR instala un cargador de arranque; revíselos antes de reiniciar.
- Consulte [Restauración bare-metal](../../user-guide/bare-metal-restore.md) para ver el flujo completo de recuperación y cómo se emite el token de recuperación.
