# Compilación y empaquetado

Esta página documenta todos los targets del Makefile para compilar binarios y generar paquetes distribuibles. Toda la configuración relacionada con el empaquetado se encuentra en el directorio `packaging/`.

## Binarios { #binaries }

### Servidor { #server }

```bash
make build                  # bin/hitstor        (linux/amd64)
make build-arm64            # bin/hitstor-arm64   (linux/arm64)
make build-windows          # bin/hitstor.exe     (windows/amd64)
```

### Agente { #agent }

```bash
make agent                  # bin/hitstor-agent           (linux/amd64)
make agent-arm64            # bin/hitstor-agent-arm64      (linux/arm64)
make agent-windows          # bin/hitstor-agent.exe        (windows/amd64)
make agent-windows-arm64    # bin/hitstor-agent-arm64.exe  (windows/arm64)
```

### Todos los binarios { #all-binaries }

```bash
make build-all
```

Compila todos los binarios del servidor y del agente para todas las plataformas con un solo comando.

### Plugins { #plugins }

```bash
make plugins
```

Compila los cuatro binarios de plugins en `bin/`:

| Target | Salida |
|--------|--------|
| `make plugin-memory` | `bin/hitstor-backend-memory` |
| `make notifier-log` | `bin/hitstor-notifier-log` |
| `make hook-log` | `bin/hitstor-hook-log` |
| `make scanner-builtin` | `bin/hitstor-scanner-builtin` |

## Generación de código Protobuf { #protobuf-code-generation }

Regenere los archivos `.pb.go` y `_grpc.pb.go` después de modificar las definiciones `.proto`:

```bash
make proto            # Todos los protos (backup + agent + plugin)
make proto-backup     # Solo internal/proto/hitstor/v1/backup.proto
make proto-agent      # Solo internal/proto/hitstor/v1/agent.proto
make proto-plugin     # Solo api/proto/hitstor/plugin/v1/*.proto
```

!!! note "Nota"
    Los protos de plugins requieren la ruta de inclusión `-I api/proto` para las importaciones entre archivos. Esto ya está configurado en el Makefile.

## Paquetes de Linux (.deb / .rpm) { #linux-packages-deb-rpm }

Los paquetes de Linux se compilan con [nfpm](https://nfpm.goreleaser.com/) usando archivos de configuración YAML ubicados en `packaging/nfpm/`.

### Targets del Makefile { #makefile-targets }

| Target | Requisito previo | Salida |
|--------|-------------|--------|
| `make pkg-server-deb` | `bin/hitstor` | `dist/hitstor_VERSION_amd64.deb` |
| `make pkg-server-rpm` | `bin/hitstor` | `dist/hitstor-VERSION.x86_64.rpm` |
| `make pkg-agent-deb` | `bin/hitstor-agent` | `dist/hitstor-agent_VERSION_amd64.deb` |
| `make pkg-agent-rpm` | `bin/hitstor-agent` | `dist/hitstor-agent-VERSION.x86_64.rpm` |
| `make pkg-linux` | (todos los anteriores) | Los 4 paquetes de Linux |

Cada target compila primero, de forma automática, el binario que necesita.

### Definir la versión { #setting-the-version }

Pase la variable `VERSION` para controlar la versión del paquete:

```bash
make pkg-server-deb VERSION=1.2.0
```

Si no se especifica, `VERSION` toma el valor predeterminado `dev`.

### Cómo funciona { #how-it-works }

nfpm lee una configuración YAML (p. ej. `packaging/nfpm/hitstor-server.yaml`), toma el binario precompilado de `bin/` y lo empaqueta junto con las unidades de systemd, los ejemplos de configuración y los scripts de instalación en un `.deb` o `.rpm`.

### Contenido del paquete del servidor { #server-package-contents }

| Origen | Se instala en | Descripción |
|--------|-------------|-------------|
| `bin/hitstor` | `/usr/bin/hitstor` | Binario del servidor |
| `packaging/systemd/hitstor-server.service` | `/usr/lib/systemd/system/hitstor-server.service` | Unidad de systemd |
| `packaging/systemd/hitstor-server.env.example` | `/etc/hitstor/server.env.example` | Plantilla del archivo de entorno |
| `packaging/config/server-config.yaml.example` | `/etc/hitstor/config.yaml.example` | Plantilla de configuración |

Script posterior a la instalación (`packaging/scripts/server-postinstall.sh`):

- Crea el usuario y el grupo de sistema `hitstor`
- Crea `/var/lib/hitstor`, `/var/log/hitstor`, `/etc/hitstor`
- Copia `server.env.example` a `server.env` si este no existe
- Ejecuta `systemctl daemon-reload`

### Contenido del paquete del agente { #agent-package-contents }

| Origen | Se instala en | Descripción |
|--------|-------------|-------------|
| `bin/hitstor-agent` | `/usr/bin/hitstor-agent` | Binario del agente |
| `packaging/systemd/hitstor-agent.service` | `/usr/lib/systemd/system/hitstor-agent.service` | Unidad de systemd |
| `packaging/systemd/hitstor-agent.timer` | `/usr/lib/systemd/system/hitstor-agent.timer` | Temporizador de respaldo diario |
| `packaging/systemd/hitstor-agent.env.example` | `/etc/hitstor/agent.env.example` | Plantilla del archivo de entorno |
| `packaging/config/agent-config.yaml.example` | `/etc/hitstor/agent-config.yaml.example` | Plantilla de configuración |

Script posterior a la instalación (`packaging/scripts/agent-postinstall.sh`):

- Crea `/etc/hitstor`
- Copia `agent.env.example` a `agent.env` si este no existe
- Ejecuta `systemctl daemon-reload`

### Archivos de configuración de nfpm { #nfpm-configuration-files }

- `packaging/nfpm/hitstor-server.yaml` — definición del paquete del servidor
- `packaging/nfpm/hitstor-agent.yaml` — definición del paquete del agente

Ambos usan las variables de entorno `${VERSION}` y `${ARCH}`, que el Makefile define en el momento de la compilación.

## Instaladores autoextraíbles (.run) { #self-extracting-installers-run }

Los instaladores autoextraíbles `.run` reúnen el binario, las unidades de systemd, las plantillas de configuración y un asistente interactivo en un único archivo shell ejecutable. El usuario descarga un solo archivo, lo ejecuta con `sudo` y el asistente configura todo: contraseñas, puertos, inicialización del repositorio y arranque del servicio.

### Targets del Makefile { #makefile-targets_1 }

| Target | Requisito previo | Salida |
|--------|-------------|--------|
| `make installer-server` | `bin/hitstor` | `dist/hitstor-server-VERSION-linux-amd64.run` |
| `make installer-server-arm64` | `bin/hitstor-arm64` | `dist/hitstor-server-VERSION-linux-arm64.run` |
| `make installer-agent` | `bin/hitstor-agent` | `dist/hitstor-agent-VERSION-linux-amd64.run` |
| `make installer-agent-arm64` | `bin/hitstor-agent-arm64` | `dist/hitstor-agent-VERSION-linux-arm64.run` |
| `make installers` | (servidor + agente amd64) | Ambos instaladores amd64 |

```bash
make installer-server VERSION=1.2.0
make installer-agent VERSION=1.2.0
```

### Cómo funciona { #how-it-works_1 }

El script de compilación (`packaging/installer/build-installer.sh`) arma un archivo `.run` en cuatro pasos:

1. **Reunir el payload**: copia el binario, las unidades de systemd y los ejemplos de configuración en un directorio temporal
2. **Crear el archivo comprimido**: comprime el payload en un `tar.gz`
3. **Incrustar las bibliotecas**: reemplaza los marcadores `__INLINE_*__` de la plantilla `.sh.in` por el contenido de los archivos `lib/*.sh` correspondientes
4. **Ensamblar**: sustituye `__VERSION__` y luego concatena la cabecera shell ensamblada y el payload tar.gz en un único archivo

El archivo `.run` resultante tiene esta estructura:

```
┌─────────────────────────────────┐
│ Shell script header             │  ← Installer logic (v2 template
│ (with inlined library code)     │     + all libraries inlined)
├─────────────────────────────────┤
│ __PAYLOAD_BELOW__               │  ← Marker line
├─────────────────────────────────┤
│ tar.gz binary payload           │  ← Binary + configs + service files
└─────────────────────────────────┘
```

En tiempo de ejecución, la cabecera shell busca el marcador `__PAYLOAD_BELOW__`, extrae el tar.gz desde esa posición a un directorio temporal y ejecuta los pasos de instalación.

### Archivos fuente del instalador { #installer-source-files }

El instalador v2 usa una arquitectura modular con bibliotecas compartidas que se incrustan en el momento de la compilación:

| Archivo | Descripción |
|------|-------------|
| `packaging/installer/build-installer.sh` | Script de compilación que arma el archivo `.run` |
| `packaging/installer/server-installer-v2.sh.in` | Plantilla de la cabecera shell del instalador del servidor |
| `packaging/installer/agent-installer-v2.sh.in` | Plantilla de la cabecera shell del instalador del agente |

Bibliotecas compartidas en `packaging/installer/lib/`:

| Archivo | Marcador `__INLINE_*__` | Descripción |
|------|----------------------|-------------|
| `lib/common.sh` | `__INLINE_COMMON__` | Análisis de flags, salida en color, extracción del payload, banner, verificaciones de root/systemd |
| `lib/wizard-server.sh` | `__INLINE_WIZARD_SERVER__` | Asistente interactivo del servidor (ruta del repositorio, contraseña, puertos, TLS, contraseña del dashboard) |
| `lib/wizard-agent.sh` | `__INLINE_WIZARD_AGENT__` | Asistente interactivo del agente (dirección del servidor, clave de API, rutas de respaldo) |
| `lib/verify.sh` | `__INLINE_VERIFY__` | Verificación posterior a la instalación (puertos en escucha, estado del servicio) |
| `lib/uninstall.sh` | `__INLINE_UNINSTALL__` | Eliminación limpia de binarios, archivos de servicio, configuración y datos |
| `lib/upgrade.sh` | `__INLINE_UPGRADE__` | Actualización en el lugar con reversión automática en caso de fallo |

### Flags del instalador { #installer-flags }

Los instaladores del servidor y del agente aceptan los mismos flags:

| Flag | Descripción |
|------|-------------|
| `--no-wizard` | Omite el asistente interactivo (lee la configuración de variables de entorno) |
| `--uninstall` | Elimina todos los archivos instalados y detiene y deshabilita los servicios |
| `--version` | Muestra la versión del instalador y termina |
| `--help` | Muestra información de uso |

### Comportamiento del instalador { #installer-behavior }

El instalador detecta automáticamente el modo de instalación:

1. **Instalación nueva**: si no encuentra un binario existente, ejecuta el asistente interactivo (o `--no-wizard` para automatización)
2. **Actualización**: si encuentra un binario existente en `/usr/bin/hitstor` o `/usr/bin/hitstor-agent`, ejecuta automáticamente el flujo de actualización
3. **Desinstalación**: cuando se pasa `--uninstall`, elimina todo (con solicitud de confirmación)

#### Instalación nueva (asistente) { #fresh-install-wizard }

El asistente solicita toda la configuración necesaria, escribe los archivos de configuración, inicializa el repositorio (solo en el servidor), habilita e inicia el servicio y luego verifica que los puertos estén en escucha.

#### Instalación nueva (`--no-wizard`) { #fresh-install-no-wizard }

Para despliegues automatizados, pase la configuración mediante variables de entorno:

```bash
# Servidor
sudo HITSTOR_REPO=/var/lib/hitstor/repo \
     HITSTOR_PASSWORD=secret \
     ./hitstor-server-1.0.0-linux-amd64.run --no-wizard

# Agente
sudo HITSTOR_AGENT_SERVER=backup.example.com:8420 \
     HITSTOR_AGENT_API_KEY=<64-char-hex> \
     ./hitstor-agent-1.0.0-linux-amd64.run --no-wizard
```

#### Actualización con reversión { #upgrade-with-rollback }

Cuando el instalador detecta una instalación existente:

1. Respalda el binario actual en una ubicación temporal
2. Detiene el servicio
3. Reemplaza el binario por la nueva versión
4. Inicia el servicio
5. Verifica que el servicio esté en ejecución
6. En caso de fallo: restaura automáticamente el binario respaldado y reinicia la versión anterior

Los archivos de configuración **nunca** se sobrescriben: las modificaciones del usuario siempre se conservan. Los archivos de ejemplo se actualizan a la versión más reciente.

#### Desinstalación { #uninstall }

```bash
sudo ./hitstor-server-1.0.0-linux-amd64.run --uninstall
```

Detiene y deshabilita el servicio, elimina el binario y las unidades de systemd y, opcionalmente, elimina los directorios de configuración y de datos (con confirmación).

### Payload del instalador del servidor { #server-installer-payload }

| Archivo en el paquete | Se instala en |
|----------------|-------------|
| `hitstor` | `/usr/bin/hitstor` |
| `hitstor-server.service` | `/usr/lib/systemd/system/hitstor-server.service` |
| `config.yaml.example` | `/etc/hitstor/config.yaml.example` + `/etc/hitstor/config.yaml` |
| `server.env.example` | `/etc/hitstor/server.env.example` + `/etc/hitstor/server.env` |

El instalador del servidor también crea el usuario/grupo de sistema `hitstor` y los directorios `/var/lib/hitstor` y `/var/log/hitstor`.

### Payload del instalador del agente { #agent-installer-payload }

| Archivo en el paquete | Se instala en |
|----------------|-------------|
| `hitstor-agent` | `/usr/bin/hitstor-agent` |
| `hitstor-agent.service` | `/usr/lib/systemd/system/hitstor-agent.service` |
| `hitstor-agent.timer` | `/usr/lib/systemd/system/hitstor-agent.timer` |
| `agent-config.yaml.example` | `/etc/hitstor/agent-config.yaml.example` + `/etc/hitstor/agent-config.yaml` |
| `agent.env.example` | `/etc/hitstor/agent.env.example` + `/etc/hitstor/agent.env` |

## Paquetes de Windows (instaladores .exe) { #windows-packages-exe-installers }

Los instaladores de Windows se compilan con [NSIS](https://nsis.sourceforge.io/) usando los scripts de `packaging/nsis/`.

| Target | Requisito previo | Salida |
|--------|-------------|--------|
| `make pkg-server-windows` | `bin/hitstor.exe` | `dist/hitstor-VERSION-windows-amd64-setup.exe` |
| `make pkg-agent-windows` | `bin/hitstor-agent.exe` | `dist/hitstor-agent-VERSION-windows-amd64-setup.exe` |
| `make pkg-windows` | (ambos anteriores) | Ambos instaladores de Windows |

### Scripts de NSIS { #nsis-scripts }

- `packaging/nsis/hitstor-server.nsi` — instalador del servidor (se instala en `%PROGRAMFILES%\HitStor`, se agrega al PATH, reglas de firewall y registro como servicio de Windows opcionales; admite `/S`)
- `packaging/nsis/hitstor-agent.nsi` — instalador del agente (se instala en `%PROGRAMFILES%\HitStor Agent`, registro opcional como servicio de Windows)

## Imágenes de Docker { #docker-images }

### Compilación { #build }

```bash
make docker-build VERSION=1.2.0
```

Usa el `Dockerfile` multietapa ubicado en la raíz del proyecto:

1. **Etapa de compilación**: `golang:1.26-alpine`, compila el binario del servidor
2. **Etapa de ejecución**: `alpine:3.22`, copia el binario y expone los puertos 8420 (gRPC) y 8421 (dashboard)

### Guardar para distribución { #save-for-distribution }

```bash
make docker-save VERSION=1.2.0
```

Genera `dist/hitstor-VERSION-docker.tar.gz`, un tarball comprimido con gzip que se puede cargar con `docker load`.

## Versiones publicadas { #releases }

Al hacer push de una etiqueta `vX.Y.Z`, la CI compila todos los recursos de la versión y publica el release en GitHub. Consulte [Proceso de publicación](release-process.md).

## Documentación { #documentation }

```bash
make docs-serve     # Vista previa local en http://127.0.0.1:8000/hitstor/
make docs-build     # Compila en site/ (con --strict)
make docs-deploy    # Despliega en GitHub Pages
```

## Directorio de salida { #output-directory }

Todos los artefactos distribuibles se escriben en `dist/`. Los targets de empaquetado crean este directorio automáticamente y se limpia con:

```bash
make clean          # Elimina bin/, dist/ y site/
```
