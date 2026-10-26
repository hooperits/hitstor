# Build from Source

Building from source is recommended for development, testing, or when pre-built packages are not available for your platform. HitStor compiles with `CGO_ENABLED=0`, producing fully static binaries with no runtime dependencies.

## Prerequisites

- **Go 1.26+** -- HitStor uses `log/slog` and other features from recent Go releases.
- **git** -- to clone the repository.
- **make** -- to use the provided build targets (optional but recommended).

## Clone the Repository

```bash
git clone https://github.com/hooperits/hitstor.git
cd hitstor
```

## Build

### Server

```bash
make build
```

The binary is written to `bin/hitstor`.

### Agent

```bash
make agent
```

The binary is written to `bin/hitstor-agent`.

### Build Without Make

If you prefer to build manually:

```bash
# Server
CGO_ENABLED=0 go build -o bin/hitstor ./cmd/hitstor/

# Agent
CGO_ENABLED=0 go build -o bin/hitstor-agent ./cmd/hitstor-agent/
```

## Cross-Compilation

Because HitStor compiles with `CGO_ENABLED=0`, cross-compilation is straightforward. The Makefile includes targets for common platforms:

```bash
# Linux ARM64 (Raspberry Pi, AWS Graviton)
make build-arm64
make agent-arm64

# Windows amd64
make agent-windows

# Windows ARM64
make agent-windows-arm64
```

To cross-compile manually for any platform:

```bash
# macOS AMD64
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o bin/hitstor-darwin-amd64 ./cmd/hitstor/

# macOS ARM64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o bin/hitstor-darwin-arm64 ./cmd/hitstor/
```

The resulting binary is a single static file with no external dependencies.

## Install to PATH

Copy the binary to a directory in your `$PATH`:

```bash
sudo cp bin/hitstor /usr/local/bin/
sudo cp bin/hitstor-agent /usr/local/bin/
```

## Verify the Build

```bash
hitstor version
```

This prints the version, commit hash, and build date. During development builds, the version is reported as `dev`.

You can also verify with:

```bash
hitstor --help
```

## Running Tests

To run the test suite after building:

```bash
make test
```

For integration tests:

```bash
make test-integration
```

## Build All Targets

To build all binaries (server, agent, and all cross-compiled variants) in a single command:

```bash
make build-all
```

## Next Steps

- [Getting Started](../user-guide/getting-started.md) -- initialize a repository and create your first backup.
- [Configuration](../user-guide/configuration.md) -- set up a config file to avoid repeating flags.
