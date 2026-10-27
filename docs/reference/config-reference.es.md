# Referencia de configuración

Referencia completa de todas las claves de configuración de HitStor. La configuración se almacena en formato YAML en `~/.config/hitstor/config.yaml` (Linux) o `%AppData%\hitstor\config.yaml` (Windows).

## Precedencia { #precedence }

Los valores se resuelven en este orden (de mayor a menor prioridad):

1. **Flag de la CLI** -- `--repo /path`
2. **Variable de entorno** -- `HITSTOR_REPO=/path`
3. **Archivo de configuración** -- `repo: /path`
4. **Valor predeterminado**

## Claves de configuración { #configuration-keys }

### Configuración principal { #core-settings }

| Clave | Tipo | Predeterminado | Flag de la CLI | Variable de entorno | Descripción |
|-----|------|---------|----------|---------|-------------|
| `repo` | string | | `--repo` | `HITSTOR_REPO` | Ubicación del repositorio (ruta local, `s3:endpoint/bucket` o `plugin:name/args`) |
| `password_file` | string | | `--password-file` | | Ruta al archivo que contiene la contraseña del repositorio |
| `parallel` | int | `0` | `--parallel` | | Número de workers en paralelo (`0` = detección automática según la cantidad de CPU) |
| `exclude` | list | | | | Patrones de exclusión globales (se aplican a todos los respaldos) |

### Backend S3 { #s3-backend }

| Clave | Tipo | Predeterminado | Flag de la CLI | Descripción |
|-----|------|---------|----------|-------------|
| `s3_access_key` | string | | `--s3-access-key` | Clave de acceso S3 (sensible -- se enmascara en `config show`) |
| `s3_secret_key` | string | | `--s3-secret-key` | Clave secreta S3 (sensible -- se enmascara en `config show`) |
| `s3_region` | string | `us-east-1` | `--s3-region` | Región S3 |

### Configuración del servidor { #server-settings }

La usa `hitstor serve` para configurar el servidor gRPC y el dashboard web.

| Clave | Tipo | Predeterminado | Flag de la CLI | Descripción |
|-----|------|---------|----------|-------------|
| `serve_port` | int | `8420` | `--port` | Puerto de escucha del servidor gRPC |
| `serve_tls_cert` | string | | `--tls-cert` | Ruta al archivo del certificado TLS |
| `serve_tls_key` | string | | `--tls-key` | Ruta al archivo de la clave privada TLS |
| `serve_dashboard_port` | int | `8421` | `--dashboard-port` | Puerto de escucha HTTP del dashboard |
| `serve_dashboard_password` | string | | | Contraseña de inicio de sesión del dashboard (sensible -- si está vacía, se genera una aleatoria al iniciar y se escribe una vez en el registro; consulte [Contraseña del dashboard](../installation/quickstart.md#dashboard-password)) |
| `serve_dashboard_language` | string | | | Código del idioma por defecto del dashboard (`en`, `es`). Vacío usa el idioma del navegador; cada usuario puede cambiarlo con el selector. Consulte [Traducciones](../dev-guide/translations.md#choosing-the-language) |
| `serve_advertise_addr` | string | | `--advertise-addr` | Dirección que se anuncia a los agentes para el registro (úsela cuando el servidor está detrás de NAT) |
| `update_check` | bool | `true` | | Habilita la comprobación automática de versiones. Si está habilitada, el servidor busca actualizaciones cada 24 horas y muestra un banner en el dashboard. |
| `update_url` | string | | | URL personalizada para la comprobación de actualizaciones (vacía = API de GitHub Releases) |

### Alertas { #alerting }

| Clave | Tipo | Predeterminado | Descripción |
|-----|------|---------|-------------|
| `alert_webhook_url` | string | | URL de webhook para las notificaciones de alertas (recibe un POST con carga JSON) |
| `alert_stale_threshold` | int | `25` | Horas tras las cuales un agente se considera inactivo (mínimo: 1) |
| `alert_notifier_plugins` | list | | Nombres de los plugins notificadores que se cargan para entregar alertas |

### Configuración del agente { #agent-settings }

La usa `hitstor-agent` para configurar el agente de respaldo remoto.

| Clave | Tipo | Predeterminado | Flag de la CLI | Variable de entorno | Descripción |
|-----|------|---------|----------|---------|-------------|
| `agent_server` | string | | `--server` | `HITSTOR_AGENT_SERVER` | Dirección del servidor remoto (`host:port`) |
| `agent_api_key` | string | | `--api-key` | `HITSTOR_AGENT_API_KEY` | Clave de API del agente para la autenticación (sensible -- hexadecimal de 64 caracteres) |
| `agent_ca_cert` | string | | `--ca-cert` | | Ruta al certificado de la CA para la verificación TLS |
| `agent_hostname` | string | | `--hostname` | | Reemplaza el nombre de host enviado al servidor (por defecto, el nombre de host del sistema) |
| `agent_paths` | list | | | | Rutas de respaldo predeterminadas (se usan cuando no se indican rutas en la línea de comandos) |
| `agent_excludes` | list | | | | Patrones de exclusión predeterminados para los respaldos del agente |

### Configuración de plugins { #plugin-settings }

| Clave | Tipo | Predeterminado | Descripción |
|-----|------|---------|-------------|
| `plugin_dir` | string | | Directorio donde se buscan los binarios de plugins (se busca antes que en `$PATH`) |
| `hook_plugins` | list | | Nombres de los plugins de hook que se cargan para los eventos del ciclo de vida previos y posteriores al respaldo |
| `fs_scanner_plugin` | string | | Nombre del plugin escáner del sistema de archivos (`builtin` o un plugin `Scanner` personalizado; solo uno activo a la vez) |

### Integraciones (`integrations`) { #integrations-integrations }

Las usa Sentinel (Gemini). Los valores sensibles se almacenan cifrados (AES-256-GCM,
base64) con la clave de cifrado del repositorio. Se pueden editar desde la pestaña
**Configuración → Integraciones** del dashboard.

| Clave | Tipo | Descripción |
|-----|------|-------------|
| `gemini_api_key_encrypted` | string | Clave de API de Gemini cifrada para Sentinel (y para exportar la variable de entorno `GEMINI_API_KEY`) |
| `gemini_model` | string | Modelo de Gemini (predeterminado: `gemini-1.5-pro`) |

`GEMINI_API_KEY` sigue funcionando y tiene prioridad cuando está definida. Use **Importar desde
el entorno** en Configuración para migrarla a YAML cifrado.

#### Claves heredadas ignoradas { #ignored-legacy-keys }

Las claves de Microsoft 365 (`m365_*`), de proveedores de nube (`gcp_project`, `aws_role_arn`,
`aws_region`, `azure_tenant`, `azure_client`, `azure_sub`) y
`collector_plugins` se eliminaron del esquema de configuración en v2.0.0. Los archivos
`config.yaml` existentes que las contengan siguen cargándose (YAML ignora en silencio las claves
desconocidas), pero sus valores nunca se leen. Elimine estas claves cuando le resulte conveniente.

### Sentinel (`sentinel`) { #sentinel-sentinel }

Configuración opcional de telemetría de Sentinel (edición desde el dashboard en el futuro). Las claves reflejan las variables de entorno `HITSTOR_SENTINEL_*`.

## Múltiples repositorios (`repositories`) { #multi-repository-repositories }

Al ejecutar `hitstor serve` con varios repositorios, defina una lista en lugar de un único `repo`:

```yaml
repositories:
  - name: production
    type: local
    path: /var/lib/hitstor/prod-repo
    password_file: /etc/hitstor/prod-password

  - name: offsite
    type: s3
    endpoint: s3.amazonaws.com
    bucket: company-backups
    region: us-east-1
    access_key_encrypted: <base64 AES-256-GCM ciphertext>
    secret_key_encrypted: <base64 AES-256-GCM ciphertext>
    password_file: /etc/hitstor/s3-repo-password
```

Las credenciales S3 se almacenan cifradas en el archivo de configuración. La página Almacenamiento del dashboard puede crear repositorios sin necesidad de editar a mano el texto cifrado.

Si `repositories` está vacío y `repo` está definido, el servidor se ejecuta en el modo heredado de repositorio único.

## Tipos de clave { #key-types }

| Tipo | Descripción | Comandos de administración |
|------|-------------|-------------------|
| **string** | Valor único | `config set`, `config unset` |
| **int** | Valor entero | `config set`, `config unset` |
| **list** | Lista ordenada de valores | `config add`, `config remove` |

## Configuración de ejemplo { #example-configuration }

### Servidor local { #local-server }

```yaml
repo: /var/lib/hitstor/repo
parallel: 4
exclude:
  - "*.tmp"
  - ".cache"
serve_port: 8420
serve_tls_cert: /etc/hitstor/tls/cert.pem
serve_tls_key: /etc/hitstor/tls/key.pem
serve_dashboard_port: 8421
serve_dashboard_password: changeme
alert_webhook_url: https://hooks.slack.com/services/T.../B.../xxx
alert_stale_threshold: 25
```

### Agente remoto { #remote-agent }

```yaml
agent_server: backup.internal:8420
agent_api_key: a1b2c3d4e5f6...  # hexadecimal de 64 caracteres
agent_ca_cert: /etc/hitstor/ca.pem
agent_paths:
  - /etc
  - /home
  - /var/lib
agent_excludes:
  - "*.log"
  - "/home/*/.cache"
```

### Backend S3 { #s3-backend_1 }

```yaml
repo: s3:s3.amazonaws.com/my-backup-bucket
s3_access_key: AKIAIOSFODNN7EXAMPLE
s3_secret_key: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
s3_region: us-west-2
```

## Véase también { #see-also }

- [Guía de configuración](../user-guide/configuration.md) -- tutorial de uso
- [Variables de entorno](environment-variables.md) -- todas las variables de entorno
