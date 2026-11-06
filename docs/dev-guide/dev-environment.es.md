# Configuración del entorno de desarrollo

Esta página enumera todas las herramientas necesarias para compilar, empaquetar, probar y documentar HitStor desde el código fuente.

## Toolchain de Go { #go-toolchain }

HitStor requiere **Go 1.26.10** o posterior.

```bash
go version
go version go1.26.10 linux/amd64
```

Configure su `PATH` y `GOPATH` para que los binarios instalados con `go install` estén disponibles:

```bash
export GOPATH="$HOME/gopath"
export PATH="$GOPATH/bin:$(go env GOROOT)/bin:$PATH"
```

!!! tip "Consejo"
    Agregue estos `export` a su `~/.bashrc` o `~/.zshrc` para que se conserven entre sesiones.

## Compilador de Protobuf { #protobuf-compiler }

Solo es necesario al modificar archivos `.proto` (protocolo gRPC o interfaces de plugins).

### protoc { #protoc }

Instale el compilador de Protocol Buffers:

```bash
sudo apt install -y protobuf-compiler
protoc --version
libprotoc 3.21.12
```

### Plugins de Go { #go-plugins }

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

Los archivos `.pb.go` del repositorio se generaron con `protoc` 3.21.12, `protoc-gen-go` v1.36.11 y `protoc-gen-go-grpc` v1.6.2 (consulte el encabezado de cada archivo generado); use las mismas versiones para que las diferencias al regenerar sean mínimas.

Verifique que ambos estén en su `PATH`:

```bash
which protoc-gen-go
/home/user/gopath/bin/protoc-gen-go

which protoc-gen-go-grpc
/home/user/gopath/bin/protoc-gen-go-grpc
```

## Herramientas de empaquetado { #packaging-tools }

### nfpm (paquetes de Linux) { #nfpm-linux-packages }

[nfpm](https://nfpm.goreleaser.com/) genera paquetes `.deb` y `.rpm` a partir de archivos de configuración YAML.

```bash
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.44.0
nfpm --version
```

El flujo de publicación fija nfpm v2.44.0.

!!! warning "Advertencia"
    `go install` coloca el binario en `$GOPATH/bin`, **no** en `$(go env GOROOT)/bin`. Asegúrese de que `$GOPATH/bin` esté en su `PATH`.

### NSIS / makensis (paquetes de Windows) { #nsis-makensis-windows-packages }

Solo es necesario al compilar instaladores `.exe` para Windows.

```bash
sudo apt install -y nsis python3-pil
makensis -VERSION
```

Los targets de Windows ejecutan primero `make render-windows-icon` (`scripts/render-windows-icon.sh`), que usa Python con Pillow (`python3-pil`) para generar `packaging/nsis/hitstor.ico` a partir de `assets/icon.png`.

## Docker { #docker }

Solo es necesario al compilar imágenes de contenedor.

Instale Docker Engine siguiendo la [documentación oficial](https://docs.docker.com/engine/install/ubuntu/).

```bash
docker --version
```

## Herramientas de documentación { #documentation-tools }

[MkDocs](https://www.mkdocs.org/) con el tema [Material](https://squidfunk.github.io/mkdocs-material/) genera el sitio de documentación.

```bash
pip install -r requirements-docs.txt
mkdocs --version
```

`requirements-docs.txt` fija las versiones de MkDocs, Material y [mkdocs-static-i18n](https://ultrabug.github.io/mkdocs-static-i18n/) (el sitio en inglés y español). Material incluye las extensiones de Markdown que usa el sitio (admonitions, tabbed, superfences, etc.).

## Resumen { #summary }

| Herramienta | Propósito | Verificación |
|-------------|-----------|--------------|
| `go` 1.26+ | Compilación | `go version` |
| `protoc` | Generación de código a partir de protos | `protoc --version` |
| `protoc-gen-go` | Plugin de protobuf para Go | `which protoc-gen-go` |
| `protoc-gen-go-grpc` | Plugin de gRPC para Go | `which protoc-gen-go-grpc` |
| `nfpm` | Empaquetado `.deb` / `.rpm` | `nfpm --version` |
| `makensis` | Instaladores de Windows | `makensis -VERSION` |
| `python3` + Pillow | Ícono del instalador de Windows | `python3 -c 'import PIL'` |
| `docker` | Compilación de contenedores | `docker --version` |
| `mkdocs` | Sitio de documentación | `mkdocs --version` |
| `git` | Control de versiones | `git --version` |
| `make` | Automatización de la compilación | `make --version` |
