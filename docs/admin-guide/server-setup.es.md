# Configuración del servidor

Esta guía cubre la implementación del servidor HitStor, incluida la inicialización del repositorio, el arranque del servidor, la integración con systemd y la configuración del firewall.

## Crear un repositorio { #create-a-repository }

Los repositorios se pueden crear de dos maneras:

1. **Desde el dashboard** (recomendado): vaya a **Almacenamiento** y haga clic en **Nuevo repositorio**. El dashboard lo guía en la creación de un repositorio local o compatible con S3.
2. **Desde la CLI**:

```bash
hitstor init --repo /var/lib/hitstor/repo --password "REPO_PASSWORD"
```

En sistemas de producción, use un archivo de contraseña o una variable de entorno en lugar de pasar la contraseña en la línea de comandos:

```bash
export HITSTOR_PASSWORD="REPO_PASSWORD"
hitstor init --repo /var/lib/hitstor/repo
```

## Iniciar el servidor { #start-the-server }

Inicie el servidor HitStor con los puertos de gRPC y del dashboard:

```bash
hitstor serve --repo /var/lib/hitstor/repo --port 8420 --dashboard-port 8421
```

El servidor abre dos puertos de escucha:

| Puerto | Protocolo | Propósito |
|--------|-----------|-----------|
| 8420 | gRPC (TLS) | Comunicación con los agentes: respaldos, restauraciones, programación |
| 8421 | HTTP | Dashboard web y API JSON |

El servidor se ejecuta en primer plano. Use systemd (vea más abajo) para implementaciones en producción.

## Contraseña del repositorio { #repository-password }

El servidor necesita la contraseña del repositorio para descifrar y cifrar los datos. Hay tres formas de proporcionarla:

1. **Variable de entorno** (recomendada para producción):

    ```bash
    export HITSTOR_PASSWORD="REPO_PASSWORD"
    hitstor serve --repo /var/lib/hitstor/repo
    ```

2. **Archivo de contraseña**:

    ```bash
    hitstor serve --repo /var/lib/hitstor/repo --password-file /etc/hitstor/password.txt
    ```

    El archivo debe contener la contraseña en una sola línea, sin salto de línea al final. Establezca los permisos en `0600`.

3. **Flag de línea de comandos** (no recomendado para producción):

    ```bash
    hitstor serve --repo /var/lib/hitstor/repo --password "REPO_PASSWORD"
    ```

## Archivo de configuración { #configuration-file }

El servidor lee la configuración desde `/etc/hitstor/config.yaml` cuando se indica el flag `--config`. Una configuración típica del servidor:

```yaml
# /etc/hitstor/config.yaml
repo: /var/lib/hitstor/repo

# Certificados TLS (opcionales; se generan automáticamente si se omiten)
serve_tls_cert: /etc/hitstor/tls/server.crt
serve_tls_key: /etc/hitstor/tls/server.key

# Dashboard
serve_dashboard_password: "DASHBOARD_PASSWORD"

# Alertas
alert_webhook_url: "https://hooks.example.com/hitstor"
alert_stale_threshold: 25
```

Inicie el servidor con el archivo de configuración:

```bash
hitstor serve --config /etc/hitstor/config.yaml
```

Consulte [Configuración](../user-guide/configuration.md) para ver el esquema completo y las reglas de precedencia.

## Configuración de varios repositorios { #multi-repository-configuration }

HitStor permite administrar varios repositorios desde una sola instancia del servidor. Esto es útil para separar los respaldos por equipo, entorno o política de retención.

### Definir repositorios { #defining-repositories }

Agregue una sección `repositories` al archivo de configuración:

```yaml
# /etc/hitstor/config.yaml
repositories:
  - name: production
    type: local
    path: /var/lib/hitstor/prod-repo
    password_file: /etc/hitstor/prod-password

  - name: staging
    type: local
    path: /var/lib/hitstor/staging-repo
    password_file: /etc/hitstor/staging-password

  - name: offsite
    type: s3
    endpoint: s3.amazonaws.com
    bucket: company-backups
    region: us-east-1
    access_key_encrypted: <encrypted by server>
    secret_key_encrypted: <encrypted by server>
    password_file: /etc/hitstor/s3-repo-password
```

Las claves de acceso de S3 se almacenan cifradas (`access_key_encrypted`, `secret_key_encrypted`). Use la página Almacenamiento del dashboard para crear repositorios S3 sin tener que editar el texto cifrado a mano.

Cada repositorio debe tener un `name` único. El campo `type` debe ser `local` o `s3`.

### Compatibilidad con versiones anteriores { #backward-compatibility }

Si la lista `repositories` está vacía y existe un campo `repo`, el servidor funciona en el modo heredado de un solo repositorio, con la ruta de `repo` y la variable de entorno `HITSTOR_PASSWORD`.

### Asignar agentes a repositorios { #assigning-agents-to-repositories }

Cada agente apunta a un repositorio específico. Defina el repositorio de destino desde la página de detalle del agente en el dashboard; de lo contrario, el agente usará por defecto el primer repositorio configurado.

### Administrar repositorios desde el dashboard { #managing-repositories-from-the-dashboard }

Los repositorios también se pueden crear y eliminar desde la página Almacenamiento del dashboard. Consulte [Dashboard](dashboard.md) para más detalles.

## Certificados TLS { #tls-certificates }

Toda la comunicación gRPC entre el servidor y los agentes está cifrada con TLS.

### Generados automáticamente (predeterminado) { #auto-generated-default }

Si no se proporcionan los flags `--tls-cert` / `--tls-key`, el servidor genera automáticamente un certificado autofirmado en el primer arranque y lo guarda en:

- `<repo>/server.crt`: certificado (distribúyalo a los agentes)
- `<repo>/server.key`: clave privada (consérvela solo en el servidor)

El certificado incluye todas las direcciones IP y el nombre de host del servidor en sus nombres alternativos del sujeto (SAN), de modo que los agentes pueden conectarse por IP o por nombre de host. El certificado se reutiliza entre reinicios del servidor.

Para distribuir el certificado a los agentes:

```bash
scp /var/lib/hitstor/repo/server.crt agent-host:/etc/hitstor/server.crt
```

Luego configure cada agente para que confíe en él:

```yaml
# config.yaml del agente
agent_ca_cert: /etc/hitstor/server.crt
```

O mediante un flag de la CLI:

```bash
hitstor-agent --ca-cert /etc/hitstor/server.crt ping
```

### Certificados personalizados (producción) { #custom-certificates-production }

En producción, proporcione sus propios certificados:

```bash
hitstor serve --repo /var/lib/hitstor/repo \
  --tls-cert /etc/hitstor/tls/server.crt \
  --tls-key /etc/hitstor/tls/server.key
```

O mediante el archivo de configuración:

```yaml
serve_tls_cert: /etc/hitstor/tls/server.crt
serve_tls_key: /etc/hitstor/tls/server.key
```

Si usa un certificado firmado por una CA pública, los agentes no necesitan el flag `--ca-cert`.

## Servicio de systemd { #systemd-service }

En sistemas Linux instalados mediante paquetes deb o rpm, se incluye una unidad de systemd.

### Habilitar e iniciar { #enable-and-start }

```bash
sudo systemctl enable hitstor-server
sudo systemctl start hitstor-server
```

### Archivo de entorno { #environment-file }

La unidad de systemd lee variables de entorno desde `/etc/hitstor/server.env`. Configure aquí la contraseña del repositorio y cualquier otro ajuste:

```bash
# /etc/hitstor/server.env
# Las contraseñas de los repositorios se administran desde el dashboard.
# Para el modo heredado de un solo repositorio:
# HITSTOR_PASSWORD=REPO_PASSWORD
# HITSTOR_REPO=/var/lib/hitstor/repo
```

Establezca los permisos para que solo root pueda leer el archivo:

```bash
sudo chmod 0600 /etc/hitstor/server.env
sudo chown root:root /etc/hitstor/server.env
```

### Verificar el estado { #check-status }

```bash
sudo systemctl status hitstor-server
sudo journalctl -u hitstor-server -f
```

## Reglas de firewall { #firewall-rules }

Abra los puertos de gRPC y del dashboard en el servidor. Para `firewalld`:

```bash
sudo firewall-cmd --permanent --add-port=8420/tcp
sudo firewall-cmd --permanent --add-port=8421/tcp
sudo firewall-cmd --reload
```

Para `ufw` (Ubuntu/Debian):

```bash
sudo apt install ufw
sudo ufw allow 22/tcp comment "SSH"
sudo ufw allow 8420/tcp comment "HitStor gRPC"
sudo ufw allow 8421/tcp comment "HitStor Dashboard"
sudo ufw enable
```

!!! warning "Advertencia"
    Permita siempre SSH (puerto 22) **antes** de ejecutar `ufw enable`, o podría perder el acceso remoto al servidor.

Verifique las reglas:

```bash
sudo ufw status verbose
```

Si el dashboard solo debe ser accesible desde una red de administración, restrinja el puerto 8421 según corresponda:

```bash
sudo ufw allow from 10.0.0.0/24 to any port 8421 proto tcp
```

## Salida de registros { #log-output }

HitStor usa el paquete `slog` de Go para el registro estructurado. Cuando se ejecuta como servicio de systemd, la salida de registros es JSON de forma predeterminada, lo que facilita procesarla con herramientas como `jq`:

```bash
sudo journalctl -u hitstor-server -o cat | jq .
```

Aumente el nivel de detalle con el flag `--verbose`:

| Flag | Nivel | Descripción |
|------|-------|-------------|
| (ninguno) | warn | Solo advertencias y errores |
| `-v` | info | Mensajes operativos (respaldo iniciado, agente conectado) |
| `-vv` | debug | Estado interno detallado (procesamiento de fragmentos, escritura de paquetes) |

## Próximos pasos { #next-steps }

- [Certificados TLS](tls-certificates.md): configure certificados personalizados para producción
- [Administración de agentes](agent-management.md): registre agentes para que se conecten a este servidor
- [Dashboard](dashboard.md): acceda al dashboard web
