# hitstor serve

Inicia el servidor de respaldos gRPC.

## Sinopsis { #synopsis }

```
hitstor serve [flags]
hitstor serve agent add|list|remove|re-register|assign-repo ...
hitstor serve user add|list|grant|passwd|reset-2fa|disable|enable|remove ...
```

## Descripción { #description }

Inicia el servidor gRPC de HitStor, que acepta respaldos de agentes remotos mediante TLS. El servidor se encarga de la deduplicación, el cifrado y el almacenamiento en nombre de los agentes remotos. También inicia el dashboard web en un puerto HTTP independiente.

Si no se proporciona un certificado TLS, se genera automáticamente un certificado autofirmado. El dashboard usa el mismo certificado: cuando hay un repositorio configurado, se sirve por HTTPS en el puerto del dashboard, y un servicio HTTP sin cifrar en el puerto siguiente (puerto del dashboard + 1) redirige a él. Sin repositorios configurados, el servidor arranca en modo de configuración inicial y solo sirve el dashboard, por HTTP sin cifrar.

El inicio de sesión en el dashboard usa cuentas personales con roles y verificación en dos pasos; consulte [Usuarios y acceso](../../admin-guide/users.md). El parámetro `serve_dashboard_password` solo se usa hasta que existe el primer administrador.

## Flags { #flags }

| Flag | Tipo | Predeterminado | Descripción |
|------|------|----------------|-------------|
| `--port` | int | 8420 | Puerto de escucha de gRPC |
| `--tls-cert` | string | | Ruta al archivo del certificado TLS |
| `--tls-key` | string | | Ruta al archivo de la clave privada TLS |
| `--dashboard-port` | int | 8421 | Puerto de escucha del dashboard |
| `--advertise-addr` | string | detección automática | Dirección que se envía a los agentes durante el registro (`host:port`) |

Además de todos los flags globales.

## Subcomandos { #subcommands }

### `serve agent add <name>`

Registra un nuevo agente de respaldo. Devuelve una clave de API hexadecimal de 64 caracteres que el agente usa para autenticarse. Guarde esta clave de inmediato: no se puede volver a mostrar.

### `serve agent list`

Lista todos los agentes registrados con sus nombres, fechas de creación y la hora de su último respaldo.

### `serve agent remove <name>`

Elimina un agente registrado. El agente ya no podrá conectarse. Sus tareas de respaldo también se eliminan, salvo que se indique `--keep-jobs`.

### `serve agent re-register <name> --token <token>` { #serve-agent-re-register-name-token-token }

Vuelve a registrar un agente existente, conservando sus tareas, con el token que muestra `hitstor-agent show-id`. `--addr` define la dirección del agente; `--skip-rpc` solo actualiza el registro sin contactar al agente.

### `serve agent assign-repo <name> <repo-name>` { #serve-agent-assign-repo-name-repo-name }

Define el repositorio que usarán las tareas futuras del agente.

`hitstor agents` es el mismo grupo de comandos; consulte [agents](agents.md).

### `serve user ...` { #serve-user }

Administra los usuarios del dashboard desde la consola del servidor, por ejemplo cuando ningún administrador puede iniciar sesión:

| Subcomando | Descripción |
|------------|-------------|
| `add <name> --grant <rol>[=agente1,agente2]` | Crea un usuario (repita `--grant`; `--display-name`, `--email`). Muestra una contraseña temporal |
| `list` | Lista los usuarios |
| `grant <name> --grant ...` | Reemplaza los roles de un usuario |
| `passwd <name>` | Restablece la contraseña a una temporal y cierra las sesiones del usuario |
| `reset-2fa <name>` | Elimina la app de autenticación y los códigos de recuperación del usuario; la vuelve a configurar en el siguiente inicio de sesión |
| `disable <name>` / `enable <name>` | Impide o permite el inicio de sesión; `enable` también levanta un bloqueo |
| `remove <name>` | Elimina un usuario |

Los roles son `admin`, `backup_operator`, `restore_operator` y `viewer`. Consulte [Usuarios y acceso](../../admin-guide/users.md#cli).

Los comandos `serve agent` y `serve user` trabajan sobre el repositorio donde el servidor guarda su registro de agentes y sus usuarios (el primer repositorio configurado por nombre), por lo que no se necesita `--repo-name`; si indica otro repositorio, se muestra una advertencia, porque el servidor en ejecución no lo lee.

## Ejemplos { #examples }

Iniciar el servidor con la configuración predeterminada:

```bash
hitstor serve --repo /var/lib/hitstor/repo --password "secret"
```

Iniciar con certificados TLS personalizados:

```bash
hitstor serve --repo /var/lib/hitstor/repo --password "secret" \
  --tls-cert /etc/hitstor/tls/server.crt \
  --tls-key /etc/hitstor/tls/server.key
```

Registrar un nuevo agente:

```bash
hitstor serve agent add webserver-01 \
  --repo /var/lib/hitstor/repo --password "secret"
```

Listar los agentes registrados:

```bash
hitstor serve agent list \
  --repo /var/lib/hitstor/repo --password "secret"
```

## Notas { #notes }

- El primer repositorio (por nombre) debe estar en un sistema de archivos local (no S3): el registro de agentes, los usuarios del dashboard y el registro de auditoría se guardan junto a él.
- Apagado ordenado al recibir SIGINT/SIGTERM.
- El dashboard se sirve como una aplicación HTML5 integrada, sin dependencias externas.
