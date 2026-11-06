# Variables de entorno

HitStor lee las siguientes variables de entorno. Los flags de la CLI tienen prioridad sobre ellas. Para la ubicación del repositorio y para el servidor, la clave de API y el certificado de CA del agente, un valor del archivo de configuración también tiene prioridad: la variable de entorno solo se usa cuando ni un flag ni el archivo de configuración lo definen. Las variables `HITSTOR_LOG_*` reemplazan sus claves de configuración.

## Servidor (`hitstor`) { #server-hitstor }

| Variable | Clave de configuración | Descripción |
|----------|-----------|-------------|
| `HITSTOR_REPO` | `repo` | Ubicación del repositorio (ruta local, `s3:endpoint/bucket` o `plugin:name/args`) |
| `HITSTOR_PASSWORD` | -- | Contraseña del repositorio (sin clave de configuración). Se usa cuando no se indica `--password`; tiene prioridad sobre `--password-file` |
| `HITSTOR_SYNC_WRITES` | `sync_writes` | `1` sincroniza a disco (fsync) los archivos del repositorio local después de cada cambio de nombre atómico |
| `HITSTOR_LOG_MAX_ENTRIES`, `HITSTOR_LOG_FILE_ENABLED`, `HITSTOR_LOG_FILE_PATH`, `HITSTOR_LOG_ROTATE_MAX_MB`, `HITSTOR_LOG_ROTATE_MAX_FILES` | `log_*` | Retención y rotación de registros (también las lee el agente); consulte la [Referencia de configuración](config-reference.md#logging) |
| `PORT` | -- | Si está definida, `hitstor serve` sirve gRPC y el dashboard juntos en este único puerto (para plataformas que exponen un solo puerto por contenedor) |

### Imagen de Docker e instalador de Linux { #docker-image-and-linux-installer }

Estas variables las lee el entrypoint del contenedor o el instalador `.run`, no el binario `hitstor`:

| Variable | La lee | Descripción |
|----------|--------|-------------|
| `HITSTOR_DASHBOARD_PASSWORD` | Entrypoint de Docker, `.run --no-wizard` del servidor | Contraseña de configuración inicial del dashboard, que se escribe en `serve_dashboard_password` |
| `HITSTOR_AUTO_INIT` | Entrypoint de Docker | `false` omite el `hitstor init` automático de `HITSTOR_REPO` (predeterminado `true`) |
| `HITSTOR_DATA_DIR`, `HITSTOR_GRPC_PORT`, `HITSTOR_DASHBOARD_PORT`, `HITSTOR_ADVERTISE_ADDR` | `.run --no-wizard` del servidor | Directorio de datos, puertos y dirección anunciada; consulte [Instalación rápida en Linux](../installation/linux-installer.md#non-interactive-install) |

## Agente (`hitstor-agent`) { #agent-hitstor-agent }

| Variable | Clave de configuración | Descripción |
|----------|-----------|-------------|
| `HITSTOR_AGENT_LISTEN_PORT` | -- | Puerto gRPC de escucha para la gestión desde el dashboard (predeterminado: 8422; `--listen-port` en `daemon`) |
| `HITSTOR_AGENT_SERVER` | `agent_server` | Dirección del servidor remoto (`host:port`); se define automáticamente en `agent.env` en los agentes gestionados desde el dashboard |
| `HITSTOR_AGENT_API_KEY` | `agent_api_key` | Clave de API del agente para la autenticación; se define automáticamente en `agent.env` en los agentes gestionados desde el dashboard |
| `HITSTOR_AGENT_CA_CERT` | `agent_ca_cert` | Certificado de CA para verificar el servidor; se define en `agent.env` al registrar el agente cuando el servidor envía su certificado (se guarda como `server-ca.crt`) |
| `HITSTOR_MSSQL_PASSWORD` | -- | La lee el plugin de carga de trabajo `mssql`: la contraseña del inicio de sesión SQL para las instancias sin configuración guardada (el parámetro `password_env` del plugin indica esta variable) |

En Windows, el servicio del agente las lee de `%ProgramData%\HitStor\agent.env`; en Linux, systemd carga `/etc/hitstor/agent.env` en el entorno del servicio.

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
docker run -e HITSTOR_REPO=/var/lib/hitstor/repo \
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

`HITSTOR_M365_*`, `HITSTOR_GCP_PROJECT` y `HITSTOR_AWS_REGION` **ya no se
leen**: los respaldos de SaaS y de máquinas virtuales en la nube se eliminaron
del producto en una versión anterior. Definirlas no tiene ningún efecto.

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
| `HITSTOR_AWS_ROLE_ARN` | | Alternativa a `HITSTOR_S3_ROLE_ARN` si esta no está definida |
| `HITSTOR_AZURE_CONTAINER` | | Contenedor de Azure Blob que se bloquea (cuando `BACKEND_TYPE=azure`) |
| `HITSTOR_AZURE_STORAGE_ACCOUNT` | | Nombre de la cuenta de almacenamiento de Azure |
| `HITSTOR_AZURE_TENANT` | | ID de inquilino (tenant) de Azure AD para la llamada a la API de bloqueo de retención |
| `HITSTOR_AZURE_CLIENT` | | ID de cliente de Azure AD para la llamada a la API de bloqueo de retención |
| `HITSTOR_AZURE_SUB` | | ID de suscripción de Azure para la llamada a la API de bloqueo de retención |

El bloqueo de retención solo se activa cuando Sentinel detecta una amenaza por
encima del umbral de confianza; no se ejecuta en cada respaldo. Consulte
[Sentinel contra ransomware](../admin-guide/sentinel.md).

## Notas { #notes }

- La contraseña del repositorio se toma de `--password`, luego de `HITSTOR_PASSWORD`, luego de `--password-file` y, por último, del `password_file` del repositorio seleccionado con `--repo-name`. No existe la variable `HITSTOR_PASSWORD_FILE`.
- `s3_access_key`, `s3_secret_key` y `serve_dashboard_password` no tienen variables de entorno que lea el binario `hitstor`: use el archivo de configuración o los flags de la CLI.

## Véase también { #see-also }

- [Referencia de configuración](config-reference.md) -- todas las claves de configuración
- [Guía de configuración](../user-guide/configuration.md) -- tutorial de uso
