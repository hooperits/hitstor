# Compilar desde el código fuente

Se recomienda compilar desde el código fuente para desarrollo, pruebas o cuando no hay paquetes precompilados para su plataforma. HitStor se compila con `CGO_ENABLED=0`, lo que produce binarios totalmente estáticos sin dependencias en tiempo de ejecución.

## Requisitos previos { #prerequisites }

- **Go 1.26+** -- HitStor usa `log/slog` y otras funciones de versiones recientes de Go.
- **git** -- para clonar el repositorio.
- **make** -- para usar los targets de compilación incluidos (opcional, pero recomendado).

## Clonar el repositorio { #clone-the-repository }

```bash
git clone https://github.com/hooperits/hitstor.git
cd hitstor
```

## Compilar { #build }

### Servidor { #server }

```bash
make build
```

El binario se escribe en `bin/hitstor`.

### Agente { #agent }

```bash
make agent
```

El binario se escribe en `bin/hitstor-agent`.

### Compilar sin Make { #build-without-make }

Si prefiere compilar manualmente:

```bash
# Servidor
CGO_ENABLED=0 go build -o bin/hitstor ./cmd/hitstor/

# Agente
CGO_ENABLED=0 go build -o bin/hitstor-agent ./cmd/hitstor-agent/
```

## Compilación cruzada { #cross-compilation }

Como HitStor se compila con `CGO_ENABLED=0`, la compilación cruzada es sencilla. El Makefile incluye targets para las plataformas más comunes:

```bash
# Linux ARM64 (Raspberry Pi, AWS Graviton)
make build-arm64
make agent-arm64

# Windows amd64
make agent-windows

# Windows ARM64
make agent-windows-arm64
```

Para compilar de forma cruzada manualmente para cualquier plataforma:

```bash
# macOS AMD64
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o bin/hitstor-darwin-amd64 ./cmd/hitstor/

# macOS ARM64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o bin/hitstor-darwin-arm64 ./cmd/hitstor/
```

El binario resultante es un único archivo estático sin dependencias externas.

## Instalar en el PATH { #install-to-path }

Copie el binario a un directorio incluido en su `$PATH`:

```bash
sudo cp bin/hitstor /usr/local/bin/
sudo cp bin/hitstor-agent /usr/local/bin/
```

## Verificar la compilación { #verify-the-build }

```bash
hitstor version
```

Esto muestra la versión, el hash del commit y la fecha de compilación. En las compilaciones de desarrollo, la versión se informa como `dev`.

También puede verificarlo con:

```bash
hitstor --help
```

## Ejecutar las pruebas { #running-tests }

Para ejecutar el conjunto de pruebas después de compilar:

```bash
make test
```

Para las pruebas de integración:

```bash
make test-integration
```

## Compilar todos los targets { #build-all-targets }

Para compilar todos los binarios (servidor, agente y todas las variantes de compilación cruzada) con un solo comando:

```bash
make build-all
```

## Próximos pasos { #next-steps }

- [Primeros pasos](../user-guide/getting-started.md) -- inicialice un repositorio y cree su primer respaldo.
- [Configuración](../user-guide/configuration.md) -- cree un archivo de configuración para no repetir flags.
