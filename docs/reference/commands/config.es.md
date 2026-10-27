# hitstor config

Administra la configuración.

## Sinopsis { #synopsis }

```
hitstor config show [--json]
hitstor config set <key> <value>
hitstor config unset <key>
hitstor config add <key> <value>
hitstor config remove <key> <value>
hitstor config init
hitstor config edit
hitstor config path
hitstor config repositories list
```

## Descripción { #description }

Administra el archivo de configuración de HitStor en `~/.config/hitstor/config.yaml` (o `/etc/hitstor/config.yaml` en servidores con --config).

Para la administración moderna de múltiples repositorios (lista de repositorios, credenciales S3, etc.), es preferible usar el subcomando `hitstor repo`. Las claves planas heredadas de esta página existen por compatibilidad con versiones anteriores.

Los valores de configuración se pueden establecer, eliminar o consultar. Las claves de tipo lista admiten las operaciones `add` y `remove`.

## Subcomandos { #subcommands }

### `config show`

Muestra todas las claves de configuración con sus valores efectivos y su origen (default, config, env, flag). Los valores sensibles se enmascaran. Use `--json` para obtener una salida legible por máquinas.

### `config set <key> <value>`

Asigna un valor a una clave de configuración. El valor se valida antes de guardarse.

### `config unset <key>`

Elimina una clave de configuración, con lo que vuelve a su valor predeterminado.

### `config add <key> <value>`

Agrega un valor a una clave de tipo lista (p. ej., `exclude`, `agent_paths`).

### `config remove <key> <value>`

Quita un valor específico de una clave de tipo lista.

### `config init`

Asistente interactivo de configuración. Recorre las opciones de configuración esenciales con preguntas y ejemplos.

### `config edit`

Abre el archivo de configuración en su editor predeterminado (`$EDITOR`).

### `config path`

Imprime la ruta del archivo de configuración. En servidores, da preferencia al archivo del sistema `/etc/hitstor/config.yaml` cuando existe y es legible.

### `config repositories list`

Lista los repositorios modernos definidos en la configuración (equivalente a `hitstor repo list`). Para administrarlos, use los subcomandos de `repo`.

## Ejemplos { #examples }

Mostrar todos los valores de configuración:

```bash
hitstor config show
```

Establecer la ubicación del repositorio:

```bash
hitstor config set repo /var/lib/hitstor/repo
```

Agregar un patrón de exclusión:

```bash
hitstor config add exclude "*.tmp"
```

Ejecutar la configuración interactiva:

```bash
hitstor config init
```

## Notas { #notes }

- El archivo de configuración usa formato YAML.
- Precedencia: flags de la CLI > variables de entorno > archivo de configuración > valores predeterminados.
- Consulte la [Referencia de configuración](../config-reference.md) para ver la lista completa de claves.
