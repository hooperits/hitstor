# hitstor diff

Compara dos instantáneas.

## Sinopsis { #synopsis }

```
hitstor diff [flags] <snapshot-id-1> <snapshot-id-2>
```

## Descripción { #description }

Compara dos instantáneas (snapshots) y muestra las diferencias entre ellas. Muestra los archivos que se agregaron, eliminaron o modificaron entre ambas instantáneas.

Ambos ID de instantánea son obligatorios y se pueden especificar como ID completos, prefijos cortos o `latest`.

Admite `--repo-name` para configuraciones con varios repositorios (consulte los flags globales en index.md).

## Ejemplos { #examples }

Comparar dos instantáneas:

```bash
hitstor diff --repo /backups/hitstor --password "secret" a1b2c3d4 b5c6d7e8
```

Comparar una instantánea específica con la más reciente:

```bash
hitstor diff --repo /backups/hitstor --password "secret" a1b2c3d4 latest
```
