# hitstor restore

Restaura archivos desde una instantánea (snapshot).

## Sinopsis { #synopsis }

```
hitstor restore [flags] <snapshot-id> <target-path>
```

## Descripción { #description }

Restaura todos los archivos de la instantánea especificada en el directorio de destino. La instantánea se puede identificar por su ID hexadecimal completo de 64 caracteres, por un prefijo corto único o por la palabra clave `latest` (que corresponde a la instantánea más reciente según su marca de tiempo).

Ambos argumentos son obligatorios: el ID de la instantánea y la ruta de destino donde se escribirán los archivos.

## Flags { #flags }

| Flag | Tipo | Valor predeterminado | Descripción |
|------|------|---------|-------------|
| `--include` | string | | Restaura solo los archivos que coinciden con esta ruta (se puede repetir) |

Además de todos los [flags globales](index.md#global-flags), incluido `--repo-name` para configuraciones con varios repositorios.

## Resolución del ID de instantánea { #snapshot-id-resolution }

HitStor admite tres formas de identificar una instantánea:

- **ID completo**: la cadena hexadecimal completa de 64 caracteres
- **Prefijo corto**: un prefijo único del ID de la instantánea (p. ej., `a1b2c3d4`). Si el prefijo es ambiguo (coincide con varias instantáneas), se devuelve un error.
- **`latest`**: corresponde a la instantánea con la marca de tiempo más reciente

## Ejemplos { #examples }

Restaurar la instantánea más reciente:

```bash
hitstor restore --repo /backups/hitstor --password "secret" latest /tmp/restore
```

Restaurar una instantánea específica por su ID corto:

```bash
hitstor restore --repo /backups/hitstor --password "secret" a1b2c3d4 /tmp/restore
```

Restaurar solo archivos específicos:

```bash
hitstor restore --repo /backups/hitstor --password "secret" latest /tmp/restore \
  --include "/home/user/documents/important.pdf"
```
