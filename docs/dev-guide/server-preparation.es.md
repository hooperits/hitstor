# Preparación del servidor

Esta página describe qué hacer en un servidor Ubuntu recién instalado **antes** de instalar HitStor. Solo cubre los requisitos previos del sistema operativo. Para los pasos posteriores a la instalación (init, serve, systemd), consulte [Configuración del servidor](../admin-guide/server-setup.md).

!!! tip "¿Usa el instalador .run?"
    La [instalación rápida en Linux (.run)](../installation/linux-installer.md) se encarga automáticamente, mediante un asistente interactivo, de crear el usuario, preparar los directorios, configurar, inicializar el repositorio e iniciar el servicio. De esta página solo necesita las secciones **Actualizaciones del sistema**, **Firewall** y **Almacenamiento**.

## Actualizaciones del sistema { #system-updates }

Comience con un sistema completamente actualizado:

```bash
sudo apt update && sudo apt upgrade -y
```

## Paquetes requeridos { #required-packages }

Ubuntu incluye `systemd` de forma predeterminada. Instale los paquetes adicionales de los que depende HitStor:

```bash
sudo apt install -y ca-certificates openssl
```

| Paquete | Propósito |
|---------|-----------|
| `systemd` | Administración de servicios (preinstalado en Ubuntu) |
| `ca-certificates` | Validación de certificados TLS para las conexiones de los agentes |
| `openssl` | Generación de certificados TLS |

!!! note "Nota"
    El script posterior a la instalación del `.deb` crea automáticamente el usuario de sistema `hitstor`, su grupo y todos los directorios necesarios. No es necesario crear el usuario manualmente.

## Configuración del firewall { #firewall-configuration }

HitStor usa dos puertos TCP:

| Puerto | Protocolo | Servicio |
|--------|-----------|----------|
| 8420 | TCP | gRPC: comunicación con los agentes |
| 8421 | TCP | Dashboard web y métricas de Prometheus |

### Instalar y configurar ufw { #install-and-configure-ufw }

```bash
sudo apt install ufw
```

### Abrir ambos puertos { #open-both-ports }

!!! warning "Advertencia"
    Permita siempre SSH (puerto 22) **antes** de ejecutar `ufw enable`, o podría perder el acceso remoto al servidor.

```bash
sudo ufw allow 22/tcp comment "SSH"
sudo ufw allow 8420/tcp comment "HitStor gRPC"
sudo ufw allow 8421/tcp comment "HitStor Dashboard"
sudo ufw enable
```

Verifique las reglas:

```bash
sudo ufw status verbose
```

### Restringir el dashboard a la LAN { #restrict-dashboard-to-lan }

Si el dashboard solo debe ser accesible desde la red local:

```bash
sudo ufw allow from 10.0.0.0/24 to any port 8421 proto tcp comment "HitStor Dashboard (LAN)"
```

Reemplace `10.0.0.0/24` por el rango real de su red.

## Consideraciones de almacenamiento { #storage-considerations }

Los datos del repositorio se encuentran de forma predeterminada en `/var/lib/hitstor/repo`.

- **Estimar el almacenamiento**: tamaño de los datos sin procesar dividido entre la tasa de deduplicación esperada. Las tasas de deduplicación típicas van de 2x a 10x, según qué tan similares sean los datos respaldados entre instantáneas (snapshots).
- **Volumen dedicado**: monte una partición o un volumen independiente en `/var/lib/hitstor` para aislar el almacenamiento de los respaldos del disco del sistema operativo.
- **Sistema de archivos**: ext4 o XFS. Ambos funcionan bien; XFS rinde un poco mejor con archivos grandes.
- **Permisos**: el script posterior a la instalación establece `/var/lib/hitstor` con modo `750`, propiedad de `hitstor:hitstor`.

```bash
# Ejemplo: montar un volumen dedicado
sudo mkfs.xfs /dev/sdb1
sudo mkdir -p /var/lib/hitstor
echo '/dev/sdb1 /var/lib/hitstor xfs defaults 0 2' | sudo tee -a /etc/fstab
sudo mount /var/lib/hitstor
```

## Certificados TLS { #tls-certificates }

Cuando no hay ningún certificado configurado, HitStor genera al iniciar un certificado autofirmado en memoria. Esto sirve para pruebas, pero las implementaciones en producción deben usar certificados adecuados.

### Autofirmado con OpenSSL { #self-signed-with-openssl }

```bash
sudo mkdir -p /etc/hitstor/tls
sudo openssl req -x509 -newkey rsa:4096 -nodes \
    -keyout /etc/hitstor/tls/server.key \
    -out /etc/hitstor/tls/server.crt \
    -days 365 -subj "/CN=HOSTNAME"
sudo chmod 600 /etc/hitstor/tls/server.key
sudo chown hitstor:hitstor /etc/hitstor/tls/server.key /etc/hitstor/tls/server.crt
```

Reemplace `HOSTNAME` por el FQDN o la dirección IP del servidor.

### Let's Encrypt { #lets-encrypt }

```bash
sudo apt install -y certbot
sudo certbot certonly --standalone -d backup.example.com
```

Para una configuración detallada de TLS, consulte [Certificados TLS](../admin-guide/tls-certificates.md).

## Qué hacen los instaladores { #what-the-installers-do }

### Instalador `.run` { #run-installer }

El instalador `.run`, con su asistente interactivo, se encarga de todo:

1. Crea el grupo y el usuario de sistema `hitstor`
2. Crea los directorios: `/var/lib/hitstor`, `/var/log/hitstor`, `/etc/hitstor`
3. Instala el binario y el servicio de systemd
4. Escribe los archivos de configuración a partir de las respuestas del asistente (modo `600`)
5. Inicializa el repositorio de respaldos
6. Habilita e inicia el servicio
7. Verifica que el servicio esté en ejecución y que los puertos estén escuchando

### Paquete `.deb` { #deb-package }

Cuando instala el paquete `.deb` con `dpkg -i`, el script posterior a la instalación:

1. Crea el grupo de sistema `hitstor` (si no existe)
2. Crea el usuario de sistema `hitstor` con directorio personal `/var/lib/hitstor` y shell `/usr/sbin/nologin`
3. Crea los directorios: `/var/lib/hitstor`, `/var/log/hitstor`, `/etc/hitstor`
4. Asigna la propiedad a `hitstor:hitstor` y el modo `750`
5. Copia `server.env.example` a `server.env` (modo `600`) si no existe un `server.env`
6. Ejecuta `systemctl daemon-reload` para que se reconozca el nuevo archivo de servicio

Después de instalar el paquete, aún debe editar manualmente los archivos de configuración e inicializar el repositorio.

## Próximos pasos { #next-steps }

1. **Recomendado**: instale con el asistente interactivo: [Instalación rápida en Linux (.run)](../installation/linux-installer.md)
2. O instale el paquete: [Paquetes para Linux](../installation/linux-packages.md)
3. Inicialice el repositorio e inicie el servidor: [Configuración del servidor](../admin-guide/server-setup.md)
