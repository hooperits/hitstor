# hitstor forget

Elimina instantáneas por ID o según una política de retención.

## Sinopsis { #synopsis }

```
hitstor forget [flags] [snapshot-id...]
```

## Descripción { #description }

Elimina instantáneas (snapshots) del repositorio. Hay dos modos de operación:

- **ID explícitos**: pase uno o más ID de instantánea (completos o un prefijo) como argumentos para eliminar esas instantáneas específicas.
- **Política de retención**: use los flags `--keep-*` para determinar automáticamente qué instantáneas conservar y cuáles olvidar (forget).

Estos dos modos no se pueden combinar. Indicar a la vez ID de instantáneas y flags `--keep-*` produce un error.

Olvidar una instantánea elimina solo sus metadatos. Los paquetes de datos reales permanecen en el disco hasta que se ejecuta [prune](prune.md). Use `--prune` para depurar (prune) automáticamente después de olvidar.

Durante la operación se adquiere un bloqueo exclusivo sobre el repositorio.

## Flags { #flags }

| Flag | Tipo | Predeterminado | Descripción |
|------|------|----------------|-------------|
| `--keep-last` | int | 0 | Conserva las N instantáneas más recientes |
| `--keep-hourly` | int | 0 | Conserva N instantáneas por hora |
| `--keep-daily` | int | 0 | Conserva N instantáneas diarias |
| `--keep-weekly` | int | 0 | Conserva N instantáneas semanales |
| `--keep-monthly` | int | 0 | Conserva N instantáneas mensuales |
| `--keep-yearly` | int | 0 | Conserva N instantáneas anuales |
| `--keep-tag` | string | | Conserva las instantáneas con esta etiqueta (se puede repetir) |
| `--host` | string | | Aplica la política solo a las instantáneas de este host |
| `--path` | string | | Aplica la política solo a las instantáneas con esta ruta |
| `--dry-run` | bool | false | Solo muestra lo que se eliminaría |
| `--prune` | bool | false | Depura después de olvidar para recuperar espacio |

Además de todos los [flags globales](index.md#global-flags), incluido `--repo-name` para configuraciones con varios repositorios.

## Semántica de retención { #retention-semantics }

Los flags de retención usan **semántica de unión**: una instantánea se conserva si coincide con **cualquiera** de las reglas `--keep-*` activas. Por ejemplo, con `--keep-last 3 --keep-daily 7`, una instantánea se conserva si está entre las 3 más recientes **o** entre las 7 instantáneas diarias más recientes.

Los flags `--host` y `--path` filtran a qué instantáneas se aplica la política. Las instantáneas que no coinciden con el filtro no se modifican.

## Ejemplos { #examples }

Olvidar una instantánea específica:

```bash
hitstor forget --repo /backups/hitstor --password "secret" a1b2c3d4
```

Aplicar una política de retención:

```bash
hitstor forget --repo /backups/hitstor --password "secret" \
  --keep-last 5 --keep-daily 7 --keep-weekly 4 --keep-monthly 12
```

Previsualizar lo que se eliminaría:

```bash
hitstor forget --repo /backups/hitstor --password "secret" \
  --keep-last 3 --dry-run
```

Olvidar y depurar en un solo paso:

```bash
hitstor forget --repo /backups/hitstor --password "secret" \
  --keep-last 5 --prune
```

## Notas { #notes }

- Después de olvidar instantáneas sin `--prune`, HitStor muestra un recordatorio para ejecutar `hitstor prune`.
- `--keep-tag` conserva cualquier instantánea que tenga la etiqueta indicada, independientemente de las demás reglas de retención.
