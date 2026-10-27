# Proceso de publicación de versiones

Las versiones se compilan y publican mediante GitHub Actions cuando se envía (push) una etiqueta de versión.

## Publicar una versión { #cut-a-release }

```bash
git tag -a v1.2.0 -m "HitStor 1.2.0"
git push origin v1.2.0
```

La etiqueta inicia dos workflows:

| Workflow | Qué hace |
|----------|--------------|
| `.github/workflows/release.yml` | Ejecuta `go test ./...`, luego `make release-assets VERSION=1.2.0`, y crea la release de GitHub `v1.2.0` con todos los archivos de `dist/` adjuntos. Las notas de la versión se generan a partir de los commits realizados desde la etiqueta anterior. |
| `.github/workflows/docker-publish.yml` | Compila la imagen de Docker y la publica en GHCR (`ghcr.io/hooperits/hitstor`) con las etiquetas `1.2.0` y `1.2`. |

La versión incrustada en los binarios y en los nombres de los artefactos es la etiqueta sin la `v` inicial.

Cuando terminen los workflows, despliegue la documentación si cambió:

```bash
make docs-deploy
```

## Artefactos de la versión { #release-assets }

`make release-assets` ejecuta `clean`, `build-all`, `pkg-linux`, `pkg-windows`, los instaladores `.run` para amd64 y arm64, y `checksums`. Para `VERSION=1.2.0`, `dist/` contiene:

```
dist/
├── hitstor-server-1.2.0-linux-amd64.run
├── hitstor-server-1.2.0-linux-arm64.run
├── hitstor-agent-1.2.0-linux-amd64.run
├── hitstor-agent-1.2.0-linux-arm64.run
├── hitstor_1.2.0_amd64.deb
├── hitstor-1.2.0-1.x86_64.rpm
├── hitstor-agent_1.2.0_amd64.deb
├── hitstor-agent-1.2.0-1.x86_64.rpm
├── hitstor-1.2.0-windows-amd64-setup.exe
├── hitstor-agent-1.2.0-windows-amd64-setup.exe
└── SHA256SUMS
```

`scripts/install-agent.sh` descarga `hitstor-agent-VERSION-linux-ARCH.run` de la versión más reciente, por lo que deben estar presentes los `.run` de ambas arquitecturas.

## Compilar localmente { #build-locally }

Para reproducir los artefactos de la versión en una estación de trabajo (requiere Go, [nfpm](https://nfpm.goreleaser.com/), [NSIS](https://nsis.sourceforge.io/) y Python con Pillow para el ícono de Windows):

```bash
make release-assets VERSION=1.2.0
```

`make release VERSION=1.2.0` compila los mismos artefactos más el tarball de la imagen de Docker (`dist/hitstor-1.2.0-docker.tar.gz`, requiere Docker).

## Variable VERSION { #version-variable }

La variable `VERSION` se pasa a todos los pasos de compilación mediante los flags del enlazador de Go (`-ldflags`). Controla:

- La cadena de versión incrustada en el binario (`hitstor version`)
- La versión del paquete en los metadatos de `.deb`, `.rpm` y del instalador de Windows
- La etiqueta de la imagen de Docker (`hitstor:VERSION`)
- Los nombres de los archivos de salida en `dist/`

Dos variables adicionales se establecen automáticamente:

| Variable | Origen | Ejemplo |
|----------|--------|---------|
| `VERSION` | Argumento del Makefile (predeterminado: `dev`) | `1.2.0` |
| `COMMIT` | `git rev-parse --short HEAD` | `9341885` |
| `DATE` | `date -u +%Y-%m-%d` | `2026-02-24` |

## Compilaciones parciales { #partial-builds }

| Comando | Qué compila |
|---------|----------------|
| `make release-linux` | Solo los paquetes `.deb` + `.rpm` |
| `make installers` | Solo los instaladores `.run` (amd64) |
| `make pkg-windows` | Solo los instaladores `.exe` de Windows |
| `make release-docker` | Solo el tarball de la imagen de Docker |
| `make checksums` | `SHA256SUMS` de todo el contenido de `dist/` |

## Limpieza { #clean }

Elimine todos los artefactos de compilación:

```bash
make clean
```

Esto borra `bin/`, `dist/` y `site/`.
