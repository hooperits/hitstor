# Configuración

HitStor se puede configurar mediante flags de la CLI, variables de entorno y un archivo de configuración YAML. En esta página se explica cómo interactúan estas fuentes, cómo administrar el archivo de configuración y cómo usar el asistente interactivo de configuración. Para la lista completa de claves, consulte la [Referencia de configuración](../reference/config-reference.md).

## Ubicación del archivo de configuración { #config-file-location }

Cuando no se indica `--config`, HitStor usa el primero de estos archivos que exista y pueda leerse:

1. `/etc/hitstor/config.yaml`: el archivo de todo el sistema que usan el servicio del servidor, los paquetes y los instaladores.
2. `~/.config/hitstor/config.yaml`: el archivo por usuario (`%AppData%\hitstor\config.yaml` en Windows).

Si no existe ninguno, se usa la ruta por usuario, y `hitstor config init` o `hitstor config set` lo crean allí. Indique `--config <ruta>` para usar cualquier otro archivo; `hitstor config path` muestra el archivo en uso.

## Precedencia { #precedence }

Cuando el mismo ajuste se especifica en varios lugares, HitStor aplica la siguiente precedencia (de mayor a menor):

1. **Flags de la CLI** -- siempre prevalecen.
2. **Archivo de configuración** -- proporciona valores predeterminados persistentes.
3. **Variables de entorno** -- se usan cuando ni un flag ni el archivo de configuración definen el valor.
4. **Valores predeterminados integrados** -- se usan cuando no se especifica nada más.

Por ejemplo, si el archivo de configuración define `repo: /backups/local` pero usted pasa `--repo /backups/remote` en la línea de comandos, se aplica el flag de la CLI. La variable de entorno `HITSTOR_REPO` solo se usa cuando ninguno de los dos define el repositorio.

La contraseña del repositorio no tiene una clave propia en el archivo de configuración. Se toma de `--password`, luego de `HITSTOR_PASSWORD`, luego de `--password-file` y, por último, del `password_file` del repositorio seleccionado con `--repo-name`.

## Variables de entorno { #environment-variables }

La CLI `hitstor` reconoce las siguientes variables de entorno:

| Variable | Propósito | Ejemplo |
|----------|-----------|---------|
| `HITSTOR_REPO` | Ubicación predeterminada del repositorio | `export HITSTOR_REPO=/path/to/repo` |
| `HITSTOR_PASSWORD` | Contraseña del repositorio | `export HITSTOR_PASSWORD=secret` |

Consulte [Variables de entorno](../reference/environment-variables.md) para las variables que leen el servidor y el agente.

!!! warning "Advertencia"
    Evite definir `HITSTOR_PASSWORD` en perfiles de shell o en scripts guardados en el control de versiones. Prefiera `--password-file` (o el `password_file` de un repositorio) apuntando a un archivo con permisos estrictos (por ejemplo, `chmod 600`).

## Comandos de configuración { #config-commands }

HitStor ofrece un conjunto de subcomandos para administrar el archivo de configuración sin editar el YAML a mano.

### `config show`

Muestra la configuración actual:

```console
hitstor config show
Config file: /home/user/.config/hitstor/config.yaml

Configured repositories (1):
  [0] default
      Type:           local
      Path:           /backups/local
      Password file:  /etc/hitstor/password.txt [exists, readable]

Server settings (from this file):
  (none customized in this file)

Legacy single-repo keys (for backward compatibility):
  exclude                  *.tmp, node_modules  config
  parallel                 4                    config
```

Los valores sensibles, como las claves de S3, aparecen enmascarados en la salida. `config show --json` lista cada clave con su valor y su origen.

### `config set`

Define una sola clave de configuración:

```console
hitstor config set repo /backups/local
hitstor config set parallel 4
```

Las claves son nombres planos (`repo`, `password_file`, `parallel`, `exclude`, `s3_region`, `serve_port`, etc.); si indica una clave desconocida, el comando lista las claves válidas.

### `config unset`

Elimina una clave de configuración:

```console
hitstor config unset parallel
```

### `config add`

Agrega un valor a una clave de tipo lista:

```console
hitstor config add exclude "*.log"
hitstor config add exclude ".cache"
```

### `config remove`

Quita un valor de una clave de tipo lista:

```console
hitstor config remove exclude "*.log"
```

### `config init`

Inicia el asistente interactivo de configuración (vea más abajo).

### `config edit`

Abre el archivo de configuración en su editor (`$EDITOR`, luego `$VISUAL` y, si no hay ninguno, `vi`):

```console
hitstor config edit
```

### `config path`

Muestra la ruta del archivo de configuración:

```console
hitstor config path
/home/user/.config/hitstor/config.yaml
```

## Asistente interactivo de configuración { #interactive-setup-wizard }

El comando `config init` lo guía de forma interactiva por los ajustes esenciales. Necesita una terminal interactiva:

```console
hitstor config init
Welcome to HitStor configuration setup!

Backend type (local/s3) [local]:
Repository path: /backups/local
Password file (optional): /etc/hitstor/password.txt
Parallel workers (0 = auto) [0]: 4
Exclude patterns (comma-separated, optional): *.tmp, node_modules

Configuration saved to /home/user/.config/hitstor/config.yaml
```

Para `s3` pide el repositorio (`s3:endpoint/bucket`), la clave de acceso, la clave secreta y la región de S3 en lugar de una ruta. Si ya existe un archivo de configuración, el asistente pregunta antes de sobrescribirlo y ofrece los valores actuales como predeterminados.

## Configuraciones de ejemplo { #example-configurations }

### Repositorio local { #local-repository }

Una configuración mínima para respaldar en un disco local:

```yaml
repo: /mnt/external/hitstor-repo
password_file: /etc/hitstor/password.txt
parallel: 4
exclude:
  - "*.tmp"
  - ".cache"
  - "node_modules"
```

Cuando se definen `repo` y `password_file`, HitStor los trata como un repositorio llamado `default`. Selecciónelo con `--repo-name default` para que la contraseña se lea del archivo, o indique usted mismo `--password-file`:

```console
hitstor --repo-name default backup /data
```

### Repositorio S3 { #s3-repository }

Respaldo en un bucket compatible con S3:

```yaml
repo: s3:https://s3.amazonaws.com/my-bucket
s3_access_key: AKIAIOSFODNN7EXAMPLE
s3_secret_key: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
s3_region: us-east-1
parallel: 8
exclude:
  - "*.tmp"
  - ".cache"
```

El repositorio usa el bucket completo. Las credenciales de S3 provienen de las claves `s3_*` o de los flags `--s3-access-key`, `--s3-secret-key` y `--s3-region`; no se leen las variables de entorno de AWS ni el archivo de credenciales compartido. Indique la contraseña del repositorio con `--password-file` o `HITSTOR_PASSWORD`.

### Configuración con agente remoto { #remote-agent-setup }

El agente (`hitstor-agent`) lee las claves `agent_*`. Un agente administrado desde el dashboard no necesita ninguna: el registro guarda la dirección del servidor y la clave de API en `agent.env`. Para un agente registrado a mano:

```yaml
agent_server: backup.example.com:8420
agent_api_key: a1b2c3d4...   # hex de 64 caracteres, de `hitstor serve agent add <nombre>`
agent_ca_cert: /etc/hitstor/ca.pem
agent_hostname: webserver-01
agent_paths:
  - /etc
  - /var/www
agent_excludes:
  - "*.tmp"
```

En el servidor, `alert_notifier_plugins` y `alert_webhook_url` configuran el envío de alertas; consulte [Plugins](plugins.md#notifier-plugins).

## Valores sensibles { #sensitive-values }

HitStor procura no exponer los valores de configuración sensibles:

- `config show` enmascara los secretos, como las claves de S3.
- El archivo de configuración en sí no está cifrado, salvo los valores que el servidor guarda cifrados con la clave del repositorio (por ejemplo, las claves de S3 de los repositorios creados desde el dashboard y los secretos de correo e integraciones). Protéjalo con permisos adecuados del sistema de archivos (`chmod 600 ~/.config/hitstor/config.yaml`).
- Prefiera `password_file` (o `--password-file`) antes que pasar la contraseña en la línea de comandos. El archivo también debe tener permisos estrictos.

!!! tip "Consejo"
    Puede combinar el archivo de configuración con variables de entorno en un enfoque por capas: guarde los valores predeterminados no sensibles en el archivo de configuración e inyecte la contraseña mediante `HITSTOR_PASSWORD` en tiempo de ejecución.
