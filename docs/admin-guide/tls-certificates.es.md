# Certificados TLS

Toda la comunicación gRPC entre el servidor de HitStor y los agentes se cifra con TLS, y el dashboard se sirve por HTTPS con el mismo certificado. Esta guía describe el comportamiento predeterminado con certificado autofirmado, la configuración de certificados personalizados y la gestión de certificados en los agentes.

## Comportamiento predeterminado: certificados autofirmados { #default-behavior-self-signed-certificates }

Cuando el servidor se inicia sin una configuración explícita de certificado, genera automáticamente un certificado TLS autofirmado y su clave privada. Esto proporciona cifrado desde el primer momento sin configuración adicional.

El certificado generado automáticamente:

- Se crea en el primer inicio y se guarda como `server.crt` y `server.key` en el directorio del primer repositorio del servidor (por ejemplo `/var/lib/hitstor/repo/server.crt`), y luego se reutiliza en cada reinicio
- Es válido para `localhost`, el nombre de host del servidor, `127.0.0.1` y las direcciones IP locales que tenía el servidor al generarlo
- Es un certificado ECDSA P-256 válido por 10 años
- Es adecuado para pruebas y redes internas

Si el primer repositorio no está en una ruta local (por ejemplo, S3), el certificado se mantiene en memoria y se genera uno nuevo en cada reinicio. Para regenerar el certificado guardado (por ejemplo, cuando el servidor obtiene una nueva dirección IP), elimine ambos archivos y reinicie el servidor; los agentes que fijaron el certificado anterior deben olvidarlo (consulte [más abajo](#without-a-ca-certificate-certificate-pinning)).

## Certificados personalizados { #custom-certificates }

Para despliegues en producción, proporcione su propio certificado TLS y su clave privada.

### Con flags de la CLI { #using-cli-flags }

```bash
hitstor serve \
  --repo /var/lib/hitstor/repo \
  --tls-cert /etc/hitstor/tls/server.crt \
  --tls-key /etc/hitstor/tls/server.key
```

### Con el archivo de configuración { #using-the-config-file }

```yaml
# /etc/hitstor/config.yaml
serve_tls_cert: /etc/hitstor/tls/server.crt
serve_tls_key: /etc/hitstor/tls/server.key
```

El archivo de certificado debe contener la cadena completa (el certificado del servidor seguido de los certificados intermedios). El archivo de clave debe ser una clave privada codificada en PEM y sin cifrar.

También puede cargar el certificado y la clave desde **Configuración → Seguridad y TLS** en el dashboard; se guardan como `/etc/hitstor/tls/server.crt` y `server.key` y se escriben en `serve_tls_cert` y `serve_tls_key`. Reinicie el servidor para usarlos.

### Permisos de archivos { #file-permissions }

Restrinja el acceso a la clave privada, pero manténgala legible para el usuario con el que se ejecuta el servidor (`hitstor` con la unidad de systemd empaquetada):

```bash
sudo chown root:hitstor /etc/hitstor/tls/server.key
sudo chmod 0640 /etc/hitstor/tls/server.key
```

## Uso de Let's Encrypt { #using-lets-encrypt }

HitStor no se integra directamente con Let's Encrypt ni con ACME. Genere los certificados externamente con una herramienta como `certbot` y luego indique a HitStor los archivos resultantes.

### Ejemplo con Certbot { #example-with-certbot }

```bash
sudo certbot certonly --standalone -d backup.example.com
```

Esto crea los archivos de certificado en:

- `/etc/letsencrypt/live/backup.example.com/fullchain.pem`
- `/etc/letsencrypt/live/backup.example.com/privkey.pem`

La clave privada en `/etc/letsencrypt` solo es legible por root, y el servidor empaquetado se ejecuta con el usuario `hitstor`, así que copie ambos archivos a `/etc/hitstor/tls/` (el hook de despliegue de más abajo lo hace en cada renovación) e indique a HitStor las copias:

```yaml
# /etc/hitstor/config.yaml
serve_tls_cert: /etc/hitstor/tls/server.crt
serve_tls_key: /etc/hitstor/tls/server.key
```

### Renovación { #renewal }

Los certificados de Let's Encrypt son válidos por 90 días. Después de que certbot renueve el certificado, reinicie el servidor de HitStor para cargar los archivos nuevos:

```bash
sudo systemctl restart hitstor-server
```

Para automatizarlo, agregue un hook posterior a la renovación:

```bash
# /etc/letsencrypt/renewal-hooks/deploy/hitstor-restart.sh
#!/bin/bash
set -e
live=/etc/letsencrypt/live/backup.example.com
install -o root -g hitstor -m 0644 "$live/fullchain.pem" /etc/hitstor/tls/server.crt
install -o root -g hitstor -m 0640 "$live/privkey.pem" /etc/hitstor/tls/server.key
systemctl restart hitstor-server
```

```bash
sudo mkdir -p /etc/hitstor/tls
sudo chmod +x /etc/letsencrypt/renewal-hooks/deploy/hitstor-restart.sh
sudo /etc/letsencrypt/renewal-hooks/deploy/hitstor-restart.sh   # primera copia
```

## Renovación de certificados { #certificate-renewal }

Al reemplazar los archivos de certificado (de cualquier CA, no solo de Let's Encrypt), reinicie el servidor para cargar el certificado nuevo:

```bash
sudo systemctl restart hitstor-server
```

El servidor lee los archivos de certificado al iniciar. Los cambios en los archivos de certificado en disco no se detectan en tiempo de ejecución.

## Certificado de CA del agente { #agent-ca-certificate }

Cuando el servidor usa un certificado autofirmado o un certificado de una CA privada, los agentes deben configurarse para confiar en esa CA. Use la clave de configuración `agent_ca_cert` en cada agente para indicar el archivo del certificado de la CA.

### Configuración del agente { #agent-configuration }

En el archivo de configuración del agente:

```yaml
# /etc/hitstor/agent-config.yaml
agent_ca_cert: /etc/hitstor/tls/ca.crt
```

O mediante una variable de entorno:

```bash
# /etc/hitstor/agent.env
HITSTOR_AGENT_CA_CERT=/etc/hitstor/tls/ca.crt
```

El archivo del certificado de la CA debe ser el certificado, codificado en PEM, de la autoridad que firmó el certificado del servidor. Para certificados autofirmados, use el propio certificado del servidor (`server.crt` del directorio del repositorio).

### Sin certificado de CA (anclaje de certificado) { #without-a-ca-certificate-certificate-pinning }

Si `agent_ca_cert` no está definido, el agente confía en el certificado del servidor que ve en su primera conexión y ancla (pin) su huella SHA-256 en `/etc/hitstor/server-fingerprint` (`C:\ProgramData\HitStor\server-fingerprint` en Windows). Las conexiones posteriores se rechazan si el servidor presenta un certificado distinto, salvo que ese certificado sea válido para el nombre de host del servidor según las CA de confianza del sistema; en ese caso, el anclaje se actualiza automáticamente.

Realice la primera conexión (por ejemplo, `hitstor-agent ping`) en una red de confianza. Si reemplaza intencionalmente el certificado autofirmado del servidor, elimine el archivo de huella en cada agente para que ancle el nuevo.

### Distribución del certificado de la CA { #distributing-the-ca-certificate }

Copie el certificado de la CA a cada equipo con agente:

```bash
scp /etc/hitstor/tls/ca.crt agent-host:/etc/hitstor/tls/ca.crt
```

Verifique que el agente pueda conectarse:

```bash
hitstor-agent ping
```

## Solución de problemas de TLS { #troubleshooting-tls }

| Síntoma | Causa | Solución |
|---------|-------|----------|
| `certificate signed by unknown authority` | El agente no confía en la CA del servidor | Defina `agent_ca_cert` en el agente |
| `peer certificate fingerprint ... does not match pinned` | El certificado del servidor cambió desde que el agente lo ancló | Si el cambio fue intencional, elimine el archivo `server-fingerprint` del agente; de lo contrario, investigue un posible ataque de intermediario (man-in-the-middle) |
| `tls: bad certificate` | El certificado no coincide con el nombre de host | Asegúrese de que el SAN del certificado incluya el nombre de host o la IP del servidor |
| `tls: private key does not match public key` | Los archivos de certificado y clave no corresponden | Regenere el certificado y la clave o vuelva a emparejarlos |
| `permission denied` al leer el archivo de clave | Permisos de archivo demasiado restrictivos | Asegúrese de que el usuario del proceso de HitStor pueda leer el archivo de clave |

## Próximos pasos { #next-steps }

- [Gestión de agentes](agent-management.md) -- registre los agentes que se conectarán por TLS
- [Seguridad](security.md) -- modelo de seguridad completo, incluidos el cifrado y la derivación de claves
