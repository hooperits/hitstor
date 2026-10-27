# Variables de entorno

HitStor lee las siguientes variables de entorno. Las variables de entorno tienen prioridad sobre los valores del archivo de configuración, pero los flags de la CLI tienen prioridad sobre ellas.

## Servidor (`hitstor`) { #server-hitstor }

| Variable | Clave de configuración | Descripción |
|----------|-----------|-------------|
| `HITSTOR_REPO` | `repo` | Ubicación del repositorio (ruta local, `s3:endpoint/bucket` o `plugin:name/args`) |
| `HITSTOR_PASSWORD` | -- | Contraseña del repositorio (sin clave de configuración: solo variable de entorno o solicitud interactiva) |
| `HITSTOR_PASSWORD_FILE` | `password_file` | Ruta al archivo que contiene la contraseña del repositorio |

## Agente (`hitstor-agent`) { #agent-hitstor-agent }

| Variable | Clave de configuración | Descripción |
|----------|-----------|-------------|
| `HITSTOR_AGENT_LISTEN_PORT` | `agent_listen_port` | Puerto gRPC de escucha para la gestión desde el dashboard (predeterminado: 8422) |
| `HITSTOR_AGENT_SERVER` | `agent_server` | Dirección del servidor remoto (`host:port`); se define automáticamente en los agentes gestionados desde el dashboard |
| `HITSTOR_AGENT_API_KEY` | `agent_api_key` | Clave de API del agente para la autenticación; se define automáticamente en los agentes gestionados desde el dashboard |

## Ejemplos de uso { #usage-examples }

### Servidor con variables de entorno { #server-with-environment-variables }

```bash
export HITSTOR_REPO=/var/lib/hitstor/repo
export HITSTOR_PASSWORD=mysecretpassword
hitstor serve --port 8420 --tls-cert cert.pem --tls-key key.pem
```

### Agente con variables de entorno { #agent-with-environment-variables }

```bash
export HITSTOR_AGENT_SERVER=backup.internal:8420
export HITSTOR_AGENT_API_KEY=a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2
hitstor-agent backup /etc /home
```

### Archivo de entorno de systemd { #systemd-environment-file }

Las unidades de systemd del servidor y del agente admiten archivos de entorno:

```bash
# /etc/hitstor/server.env
HITSTOR_REPO=/var/lib/hitstor/repo
HITSTOR_PASSWORD=mysecretpassword

# /etc/hitstor/agent.env (gestionado desde el dashboard)
HITSTOR_AGENT_LISTEN_PORT=8422

# /etc/hitstor/agent.env (heredado)
# HITSTOR_AGENT_SERVER=backup.internal:8420
# HITSTOR_AGENT_API_KEY=a1b2c3d4...
```

### Docker { #docker }

```bash
docker run -e HITSTOR_REPO=/data/repo \
           -e HITSTOR_PASSWORD=mysecretpassword \
           hitstor:latest serve
```

## Integraciones (servidor) { #integrations-server }

Cuando están definidas, estas variables sobrescriben las claves `integrations.*`
correspondientes de `config.yaml`. En producción, prefiera el YAML cifrado; use
**Configuración → Importar desde el entorno** para migrar los valores al archivo
de configuración.

| Variable | Clave de configuración | Descripción |
|----------|-----------|-------------|
| `GEMINI_API_KEY` | `integrations.gemini_api_key_encrypted` | Clave de API de Gemini para Sentinel (en texto plano en el entorno; se almacena cifrada al importarla) |

Cuando se definen tanto las variables de entorno como el YAML, las variables de
entorno tienen prioridad. Después de la importación, los secretos del YAML
quedan cifrados y las variables de entorno se pueden eliminar.

### Variables heredadas ignoradas { #ignored-legacy-variables }

`HITSTOR_M365_*`, `HITSTOR_GCP_PROJECT`, `HITSTOR_AWS_ROLE_ARN`,
`HITSTOR_AWS_REGION` y `HITSTOR_AZURE_SUB` **ya no se leen** desde la
v2.0.0, cuando se eliminaron del producto los respaldos de SaaS, bases de datos
y máquinas virtuales en la nube. Definirlas no tiene ningún efecto.

`HITSTOR_AZURE_TENANT` y `HITSTOR_AZURE_CLIENT` todavía se leen, pero su
significado cambió: ya no configuran el descubrimiento de máquinas virtuales en
la nube, sino que ahora autentican el bloqueo de retención opcional de Azure
Blob de Sentinel (consulte más abajo).

## Sentinel (servidor) { #sentinel-server }

Sentinel se configura completamente mediante variables de entorno, que se leen
una sola vez al iniciar `hitstor serve`; no existe un equivalente con
`hitstor config set`. Cuando no se definen aquí, la clave de API y el modelo de
Gemini se toman de los valores `integrations.*` indicados arriba.

| Variable | Valor predeterminado | Descripción |
|----------|---------|-------------|
| `HITSTOR_SENTINEL_ENABLED` | `true` | Defínala como `false` para deshabilitar Sentinel por completo |
| `HITSTOR_SENTINEL_MODEL` | `integrations.gemini_model`; si no, `gemini-1.5-flash` | Modelo de Gemini usado para el análisis de amenazas |
| `HITSTOR_SENTINEL_API_KEY` | `integrations.gemini_api_key_encrypted` | Clave de API de Gemini para Sentinel (si no se define, se usa la clave de Gemini compartida) |
| `HITSTOR_SENTINEL_THRESHOLD` | `0.8` | Confianza (0-1) por encima de la cual una amenaza detectada activa la contención |
| `HITSTOR_SENTINEL_RETENTION_DAYS` | `30` | Período de retención que se aplica cuando Sentinel bloquea un bucket de almacenamiento |
| `HITSTOR_SENTINEL_BACKEND_TYPE` | `gcs` | Backend de almacenamiento cuyo bloqueo de retención se usa: `gcs`, `s3` o `azure` |
| `HITSTOR_GCS_BUCKET` | | Bucket de GCS que se bloquea (cuando `BACKEND_TYPE=gcs`) |
| `HITSTOR_S3_BUCKET` | | Bucket de S3 que se bloquea (cuando `BACKEND_TYPE=s3`) |
| `HITSTOR_S3_REGION` | | Región del bucket de S3 |
| `HITSTOR_S3_ROLE_ARN` | | Rol de AWS que se asume para la llamada a la API de S3 Object Lock |
| `HITSTOR_AZURE_CONTAINER` | | Contenedor de Azure Blob que se bloquea (cuando `BACKEND_TYPE=azure`) |
| `HITSTOR_AZURE_STORAGE_ACCOUNT` | | Nombre de la cuenta de almacenamiento de Azure |
| `HITSTOR_AZURE_TENANT` | | ID de inquilino (tenant) de Azure AD para la llamada a la API de bloqueo de retención |
| `HITSTOR_AZURE_CLIENT` | | ID de cliente de Azure AD para la llamada a la API de bloqueo de retención |

El bloqueo de retención solo se activa cuando Sentinel detecta una amenaza por
encima del umbral de confianza; no se ejecuta en cada respaldo. Consulte
[Sentinel contra ransomware](../admin-guide/sentinel.md).

## Notas { #notes }

- La contraseña del repositorio se puede proporcionar mediante `--password`, `--password-file`, `HITSTOR_PASSWORD`, `HITSTOR_PASSWORD_FILE` o la clave `password_file` del archivo de configuración. Los flags de la CLI tienen la máxima prioridad.
- `HITSTOR_PASSWORD_FILE` tiene prioridad sobre `HITSTOR_PASSWORD` si ambas están definidas.
- Los valores sensibles (`s3_access_key`, `s3_secret_key`, `agent_api_key`, `serve_dashboard_password`) no tienen variables de entorno dedicadas: use el archivo de configuración o los flags de la CLI.

## Véase también { #see-also }

- [Referencia de configuración](config-reference.md) -- todas las claves de configuración
- [Guía de configuración](../user-guide/configuration.md) -- tutorial de uso
