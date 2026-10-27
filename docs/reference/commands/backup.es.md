# hitstor backup

Respalda archivos en el repositorio.

## Sinopsis { #synopsis }

```
hitstor backup [flags] [paths...]
```

## Descripción { #description }

Explora las rutas indicadas, divide los archivos en fragmentos (chunks) definidos por contenido (FastCDC), los deduplica contra los datos existentes en el repositorio y una caché local de SQLite, los comprime (Zstd), los cifra (AES-256-GCM) y escribe los datos en el repositorio como archivos de paquete. Se crea una instantánea (snapshot) que registra las rutas respaldadas, el nombre de host, la marca de tiempo y la estructura del árbol.

Se requiere al menos un argumento de ruta.

Se adquiere un bloqueo exclusivo sobre el repositorio mientras dura el respaldo.

## Flags { #flags }

| Flag | Tipo | Predeterminado | Descripción |
|------|------|---------|-------------|
| `--exclude` | string | | Excluye los archivos que coinciden con este patrón glob (repetible) |
| `--tag` | string | | Agrega una etiqueta a la instantánea (repetible) |
| `--host` | string | nombre de host del sistema | Reemplaza el nombre de host almacenado en la instantánea |
| `--dry-run` | bool | false | Muestra una vista previa de lo que se respaldaría sin escribir datos |

Además de todos los [flags globales](index.md#global-flags), incluido `--repo-name` para configuraciones con varios repositorios.

### Combinación de exclusiones { #exclude-merging }

Los patrones de `--exclude` se combinan con los patrones `exclude` definidos en el archivo de configuración. Ambas fuentes se aplican durante el respaldo.

## Ejemplos { #examples }

Respaldar un solo directorio:

```bash
hitstor backup --repo /backups/hitstor --password "secret" /home/user/documents

# Usar un repositorio con nombre de una configuración con varios repositorios
hitstor --config /etc/hitstor/config.yaml --repo-name Production backup /data
```

Respaldar varias rutas con exclusiones:

```bash
hitstor backup --repo /backups/hitstor --password "secret" \
  /home/user/documents /home/user/photos \
  --exclude "*.tmp" --exclude ".cache/**"
```

Etiquetar un respaldo para identificarlo:

```bash
hitstor backup --repo /backups/hitstor --password "secret" /data \
  --tag "pre-upgrade" --tag "production"
```

## Notas { #notes }

- Los archivos que cambian durante el respaldo (detectados al volver a comprobar mtime/tamaño después de leerlos) se omiten con una advertencia.
- Los archivos sin cambios se detectan mediante la caché de archivos de SQLite y sus fragmentos no se vuelven a leer del disco.
- Las rutas se resuelven como rutas absolutas antes de almacenarse en la instantánea.
