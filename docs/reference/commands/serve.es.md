# hitstor serve

Inicia el servidor de respaldos gRPC.

## Sinopsis { #synopsis }

```
hitstor serve [flags]
hitstor serve agent add <name>
hitstor serve agent list
hitstor serve agent remove <name>
```

## Descripción { #description }

Inicia el servidor gRPC de HitStor, que acepta respaldos de agentes remotos mediante TLS. El servidor se encarga de la deduplicación, el cifrado y el almacenamiento en nombre de los agentes remotos. También inicia el dashboard web en un puerto HTTP independiente.

Si no se proporciona un certificado TLS, se genera automáticamente un certificado autofirmado.

Durante toda la vida del servidor se mantiene un bloqueo exclusivo sobre el repositorio.

## Flags { #flags }

| Flag | Tipo | Predeterminado | Descripción |
|------|------|----------------|-------------|
| `--port` | int | 8420 | Puerto de escucha de gRPC |
| `--tls-cert` | string | | Ruta al archivo del certificado TLS |
| `--tls-key` | string | | Ruta al archivo de la clave privada TLS |
| `--dashboard-port` | int | 8421 | Puerto de escucha HTTP del dashboard |

Además de todos los flags globales.

## Subcomandos { #subcommands }

### `serve agent add <name>`

Registra un nuevo agente de respaldo. Devuelve una clave de API hexadecimal de 64 caracteres que el agente usa para autenticarse. Guarde esta clave de inmediato: no se puede volver a mostrar.

### `serve agent list`

Lista todos los agentes registrados con sus nombres, fechas de creación y la hora de su último respaldo.

### `serve agent remove <name>`

Elimina un agente registrado. El agente ya no podrá conectarse.

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

- El servidor requiere un backend en el sistema de archivos local (no S3), porque el registro de agentes se almacena junto al repositorio.
- Apagado ordenado al recibir SIGINT/SIGTERM.
- El dashboard se sirve como una aplicación HTML5 integrada, sin dependencias externas.
