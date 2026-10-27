# Configuración

HitStor se puede configurar mediante flags de la CLI, variables de entorno y un archivo de configuración YAML. En esta página se explica cómo interactúan estas fuentes, cómo administrar el archivo de configuración y cómo usar el asistente interactivo de configuración.

## Ubicación del archivo de configuración { #config-file-location }

El archivo de configuración predeterminado se encuentra en:

```
~/.config/hitstor/config.yaml
```

HitStor crea este directorio automáticamente la primera vez que usted ejecuta `hitstor config init`. También puede crear el archivo manualmente.

## Precedencia { #precedence }

Cuando el mismo ajuste se especifica en varios lugares, HitStor aplica la siguiente precedencia (de mayor a menor):

1. **Flags de la CLI** -- siempre prevalecen.
2. **Variables de entorno** -- sobrescriben el archivo de configuración.
3. **Archivo de configuración** -- proporciona valores predeterminados persistentes.
4. **Valores predeterminados integrados** -- se usan cuando no se especifica nada más.

Por ejemplo, si el archivo de configuración define `repo: /backups/local` pero usted pasa `--repo /backups/remote` en la línea de comandos, se aplica el flag de la CLI.

## Variables de entorno { #environment-variables }

Se reconocen las siguientes variables de entorno:

| Variable | Propósito | Ejemplo |
|----------|---------|---------|
| `HITSTOR_REPO` | Ubicación predeterminada del repositorio | `export HITSTOR_REPO=/path/to/repo` |
| `HITSTOR_PASSWORD` | Contraseña del repositorio | `export HITSTOR_PASSWORD=secret` |
| `HITSTOR_PASSWORD_FILE` | Ruta a un archivo que contiene la contraseña | `export HITSTOR_PASSWORD_FILE=/etc/hitstor/password.txt` |

Cuando están definidas tanto `HITSTOR_PASSWORD` como `HITSTOR_PASSWORD_FILE`, `HITSTOR_PASSWORD` tiene prioridad.

!!! warning
    Evite definir `HITSTOR_PASSWORD` en perfiles de shell o en scripts almacenados en control de versiones. Para mayor seguridad, use `HITSTOR_PASSWORD_FILE` con permisos de archivo estrictos (p. ej., `chmod 600`).

## Comandos de configuración { #config-commands }

HitStor ofrece un conjunto de subcomandos para administrar el archivo de configuración sin editar YAML a mano.

### `config show`

Muestra la configuración actual:

```console
hitstor config show
repo: /backups/local
password: ********
backup:
  exclude:
    - "*.tmp"
    - "node_modules"
  parallel: 4
```

Los valores sensibles, como las contraseñas, se enmascaran en la salida.

### `config set`

Establece una sola clave de configuración:

```console
hitstor config set repo /backups/local
hitstor config set backup.parallel 4
```

Las claves anidadas se indican con notación de puntos.

### `config unset`

Elimina una clave de configuración:

```console
hitstor config unset backup.parallel
```

### `config add`

Agrega un valor a una clave de tipo lista:

```console
hitstor config add backup.exclude "*.log"
hitstor config add backup.exclude ".cache"
```

### `config remove`

Quita un valor de una clave de tipo lista:

```console
hitstor config remove backup.exclude "*.log"
```

### `config init`

Inicia el asistente interactivo de configuración (vea más abajo).

### `config edit`

Abre el archivo de configuración en su editor predeterminado (`$EDITOR` o `vi`):

```console
hitstor config edit
```

### `config path`

Imprime la ruta absoluta del archivo de configuración:

```console
hitstor config path
/home/user/.config/hitstor/config.yaml
```

## Asistente interactivo de configuración { #interactive-setup-wizard }

El comando `config init` lo guía de forma interactiva por los ajustes esenciales:

```console
hitstor config init
Welcome to HitStor configuration.

Repository path []: /backups/local
Password []: ********
Default number of parallel workers (0 = auto) [0]: 4

Exclude patterns (one per line, empty line to finish):
> *.tmp
> node_modules
>

Configuration saved to /home/user/.config/hitstor/config.yaml
```

El asistente crea el directorio y el archivo de configuración si no existen. Si ya existe un archivo de configuración, el asistente precarga los valores actuales para que pueda actualizarlos de forma selectiva.

## Configuraciones de ejemplo { #example-configurations }

### Repositorio local { #local-repository }

Una configuración mínima para respaldar en un disco local:

```yaml
repo: /mnt/external/hitstor-repo
password_file: /etc/hitstor/password.txt

backup:
  exclude:
    - "*.tmp"
    - ".cache"
    - "node_modules"
  parallel: 4
```

### Repositorio S3 { #s3-repository }

Respaldo en un bucket compatible con S3:

```yaml
repo: s3:s3.amazonaws.com/my-bucket/hitstor-repo
password_file: /etc/hitstor/password.txt

backup:
  exclude:
    - "*.tmp"
    - ".cache"
  parallel: 8
```

Las credenciales de AWS se configuran mediante las variables de entorno estándar (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`) o el archivo de credenciales compartido.

### Configuración con agente remoto { #remote-agent-setup }

Cuando HitStor se comunica con un agente remoto para el procesamiento del lado del servidor:

```yaml
repo: agent:user@backupserver:/data/hitstor-repo
password_file: /etc/hitstor/password.txt

backup:
  exclude:
    - "*.tmp"
  parallel: 4
  host: webserver-01

alerting:
  alert_notifier_plugins:
    - slack-notifier
```

## Valores sensibles { #sensitive-values }

HitStor procura no exponer los valores de configuración sensibles:

- `config show` enmascara las contraseñas y los tokens con `********`.
- El archivo de configuración en sí no está cifrado. Protéjalo con los permisos adecuados del sistema de archivos (`chmod 600 ~/.config/hitstor/config.yaml`).
- Es preferible usar `password_file` en lugar de guardar la contraseña directamente en el archivo de configuración. El archivo al que apunta `password_file` también debe tener permisos estrictos.

!!! tip
    Puede combinar el archivo de configuración con variables de entorno en un enfoque por capas: guarde los valores predeterminados no sensibles en el archivo de configuración e inyecte las contraseñas mediante `HITSTOR_PASSWORD_FILE` en tiempo de ejecución.
