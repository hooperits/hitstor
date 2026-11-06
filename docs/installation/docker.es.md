# Despliegue con Docker

HitStor ofrece un Dockerfile multietapa que produce una imagen de contenedor mínima basada en Alpine. La imagen ejecuta el servidor HitStor con soporte para gRPC y el dashboard web.

## Requisitos previos { #prerequisites }

- Docker 20.10+ o un runtime de contenedores compatible
- Docker Compose v2 (para el despliegue basado en compose)

## Construir la imagen { #build-the-image }

Desde la raíz del repositorio:

```bash
docker build -t hitstor:latest .
```

Para construirla con una etiqueta de versión específica:

```bash
docker build -t hitstor:0.2.0 \
  --build-arg VERSION=0.2.0 \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg DATE=$(date -u +%Y-%m-%d) .
```

O use el target del Makefile:

```bash
make docker-build VERSION=0.2.0
```

## Detalles de la imagen { #image-details }

La imagen Docker usa una construcción en dos etapas:

1. **Etapa de compilación** -- `golang:1.26-alpine` compila el binario estático con `CGO_ENABLED=0`.
2. **Etapa de ejecución** -- `alpine:3.22` con solo `ca-certificates`, `tzdata` y `wget` (para la verificación de estado) instalados. Todos los binarios de HitStor de `cmd/` se copian a `/usr/bin`.

La imagen se ejecuta con un usuario `hitstor` sin privilegios de root.

| Propiedad | Valor |
|----------|-------|
| Imagen base | `alpine:3.22` |
| Usuario | `hitstor` |
| Puertos expuestos | `8420` (gRPC), `8421` (dashboard) |
| Volúmenes | `/var/lib/hitstor` (datos), `/etc/hitstor` (configuración) |
| Punto de entrada | `/entrypoint.sh` |
| Comando predeterminado | `serve` |

## Inicio rápido { #quick-start }

Para la guía completa de principio a fin (servidor Docker + instalación del agente), consulte [Inicio rápido](quickstart.md).

Ejecute el servidor directamente con Docker:

```bash
docker run -d \
  --name hitstor \
  -p 8420:8420 \
  -p 8421:8421 \
  -v hitstor-data:/var/lib/hitstor \
  -e HITSTOR_PASSWORD=my-secret-password \
  -e HITSTOR_REPO=/var/lib/hitstor/repo \
  hitstor:latest
```

En el primer inicio, el entrypoint ejecuta `hitstor init` para `HITSTOR_REPO` si allí no existe un repositorio, y luego `hitstor serve`. Abra `https://<host>:8421` (certificado autofirmado) y cree el primer administrador como se describe en [Contraseña del dashboard y primer administrador](quickstart.md#dashboard-password). Sin `HITSTOR_DASHBOARD_PASSWORD`, el contenedor no tiene un archivo de configuración donde guardar la contraseña generada, así que léala con `docker logs hitstor`.

## Despliegue con Docker Compose { #docker-compose-deployment }

### Inicio rápido (repositorio local) { #quickstart-local-repository }

Para evaluación y uso en homelab:

```bash
cp .env.example .env
# edite HITSTOR_PASSWORD
docker compose -f docker-compose.quickstart.yml up -d
```

Consulte [Inicio rápido](quickstart.md) para la configuración del agente y el stack opcional de MinIO (`--profile s3`).

### Producción { #production }

El despliegue de producción usa `docker-compose.prod.yml`:

### 1. Crear el directorio de configuración { #1-create-the-configuration-directory }

```bash
mkdir -p config
```

Coloque su `config.yaml` en este directorio. Puede copiar y editar el ejemplo:

```bash
cp packaging/config/server-config.yaml.example config/config.yaml
editor config/config.yaml
```

### 2. Definir las variables de entorno { #2-set-environment-variables }

Cree un archivo `.env` en la raíz del repositorio:

```bash
HITSTOR_VERSION=latest
HITSTOR_PASSWORD=my-secret-password
```

O exporte las variables en su shell antes de ejecutar `docker compose`.

### 3. Iniciar los servicios { #3-start-the-services }

```bash
docker compose -f docker-compose.prod.yml up -d
```

### 4. Verificar { #4-verify }

Compruebe que el contenedor esté en ejecución y en buen estado:

```bash
docker compose -f docker-compose.prod.yml ps
```

La verificación de estado usa `wget` para consultar el dashboard en el puerto 8421 cada 30 segundos.

Vea los registros:

```bash
docker compose -f docker-compose.prod.yml logs -f
```

## Puertos { #ports }

| Puerto | Protocolo | Descripción |
|------|----------|-------------|
| 8420 | TCP | Servidor gRPC -- conexiones de agentes y operaciones de la CLI |
| 8421 | TCP | Dashboard web |

## Volúmenes { #volumes }

| Punto de montaje | Descripción |
|-------------|-------------|
| `/var/lib/hitstor` | Datos del repositorio. Aquí se almacenan los paquetes de respaldo cifrados, los índices y las instantáneas (snapshots). Use un volumen con nombre o un bind mount hacia almacenamiento persistente. |
| `/etc/hitstor` | Archivos de configuración. Se montan en modo de solo lectura desde el directorio `config/` del host. |

## Variables de entorno { #environment-variables }

| Variable | Descripción |
|----------|-------------|
| `HITSTOR_PASSWORD` | Contraseña del repositorio usada para cifrar y descifrar. |
| `HITSTOR_REPO` | Ruta del repositorio dentro del contenedor. Los archivos compose definen `/var/lib/hitstor/repo`; la imagen no tiene valor predeterminado. |
| `HITSTOR_DASHBOARD_PASSWORD` | Contraseña de configuración inicial del dashboard, para crear el primer administrador. El entrypoint la escribe en `/etc/hitstor/config.yaml` cuando ese archivo todavía no existe. |
| `HITSTOR_AUTO_INIT` | `true` (predeterminado) ejecuta `hitstor init` para `HITSTOR_REPO` al iniciar si no existe un repositorio; `false` lo omite. |
| `HITSTOR_VERSION` | Etiqueta de versión de la imagen que usan los archivos compose. |

## Verificación de estado { #health-check }

El archivo compose de producción incluye una verificación de estado integrada:

```yaml
healthcheck:
  test: ["CMD-SHELL", "wget -q -O /dev/null --no-check-certificate https://localhost:8421/login || wget -q -O /dev/null http://localhost:8421/login"]
  interval: 30s
  timeout: 5s
  retries: 3
```

Use `docker inspect` o `docker compose ps` para consultar el estado. La verificación solicita la página de inicio de sesión por HTTPS (con el certificado autofirmado) y, si falla, por HTTP sin cifrar, que el dashboard solo sirve mientras no existe un repositorio.

## Guardar la imagen { #saving-the-image }

Para exportar la imagen como tarball para un despliegue sin conexión:

```bash
make docker-save VERSION=0.2.0
```

Esto genera `dist/hitstor-0.2.0-docker.tar.gz`. Cárguela en el host de destino con:

```bash
docker load < hitstor-0.2.0-docker.tar.gz
```

## Próximos pasos { #next-steps }

- [Primeros pasos](../user-guide/getting-started.md) -- inicialice un repositorio y cree su primer respaldo.
- [Configuración](../user-guide/configuration.md) -- referencia completa del archivo de configuración.
- [Actualización](upgrading.md) -- cómo actualizar un despliegue con Docker.
