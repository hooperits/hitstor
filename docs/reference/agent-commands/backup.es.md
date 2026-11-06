# hitstor-agent backup

Envía un respaldo al servidor remoto.

## Sinopsis { #synopsis }

```
hitstor-agent backup [flags] [paths...]
```

## Descripción { #description }

Escanea las rutas indicadas, divide los archivos en fragmentos (chunks) definidos por contenido (FastCDC), comprime cada fragmento con Zstd y sube los fragmentos nuevos al servidor HitStor remoto mediante gRPC/TLS. El servidor se encarga de la deduplicación, el cifrado y el almacenamiento.

Se requiere al menos un argumento de ruta (o configurarlas mediante `agent_paths`).

## Flags { #flags }

| Flag | Tipo | Predeterminado | Descripción |
|------|------|----------------|-------------|
| `--exclude` | string | | Excluye los archivos que coincidan con este patrón glob (se puede repetir) |
| `--tag` | string | | Agrega una etiqueta a la instantánea (se puede repetir) |
| `--host` | string | nombre de host del sistema o `agent_hostname` | Reemplaza el nombre de host que se envía al servidor |
| `--parallel` | int | 0 | Workers de archivos en paralelo (0 = cantidad de CPU, como máximo 8) |
| `--use-vss` | bool | false | Lee desde una instantánea coherente: VSS en Windows, o LVM/btrfs en Linux cuando está disponible |

Además de todos los [flags globales](index.md#global-flags) del agente.

## Ejemplos { #examples }

Respaldar directorios específicos:

```bash
hitstor-agent backup --server backup.internal:8420 \
  --api-key YOUR_KEY /etc /home /var/lib
```

Usando variables de entorno:

```bash
export HITSTOR_AGENT_SERVER=backup.internal:8420
export HITSTOR_AGENT_API_KEY=YOUR_KEY
hitstor-agent backup /etc /home
```

Usando las rutas del archivo de configuración:

```bash
hitstor-agent backup
```
