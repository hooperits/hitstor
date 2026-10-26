# Dev Environment Setup

This page lists every tool required to build, package, test, and document HitStor from source.

## Go Toolchain

HitStor requires **Go 1.26.10** or later.

```bash
go version
go version go1.26.10 linux/amd64
```

Set up your `PATH` and `GOPATH` so that binaries installed with `go install` are available:

```bash
export GOPATH="$HOME/gopath"
export PATH="$GOPATH/bin:$(go env GOROOT)/bin:$PATH"
```

!!! tip
    Add these exports to your `~/.bashrc` or `~/.zshrc` so they persist across sessions.

## Protobuf Compiler

Required only when modifying `.proto` files (gRPC protocol or plugin interfaces).

### protoc

Install the Protocol Buffers compiler:

```bash
sudo apt install -y protobuf-compiler
protoc --version
libprotoc 27.3
```

### Go Plugins

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

Verify both are on your `PATH`:

```bash
which protoc-gen-go
/home/user/gopath/bin/protoc-gen-go

which protoc-gen-go-grpc
/home/user/gopath/bin/protoc-gen-go-grpc
```

## Packaging Tools

### nfpm (Linux Packages)

[nfpm](https://nfpm.goreleaser.com/) builds `.deb` and `.rpm` packages from YAML configuration files.

```bash
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
nfpm --version
```

!!! warning
    `go install` places the binary in `$GOPATH/bin`, **not** `$(go env GOROOT)/bin`. Make sure `$GOPATH/bin` is on your `PATH`.

### NSIS / makensis (Windows Packages)

Required only when building Windows `.exe` installers.

```bash
sudo apt install -y nsis
makensis -VERSION
```

## Docker

Required only when building container images.

Install Docker Engine following the [official documentation](https://docs.docker.com/engine/install/ubuntu/).

```bash
docker --version
```

## Documentation Tools

[MkDocs](https://www.mkdocs.org/) with the [Material](https://squidfunk.github.io/mkdocs-material/) theme generates the documentation site.

```bash
pip install mkdocs-material
mkdocs --version
```

This also installs all required Markdown extensions (admonitions, tabbed, superfences, etc.).

## Summary

| Tool | Purpose | Verify |
|------|---------|--------|
| `go` 1.26+ | Compilation | `go version` |
| `protoc` | Proto code generation | `protoc --version` |
| `protoc-gen-go` | Go protobuf plugin | `which protoc-gen-go` |
| `protoc-gen-go-grpc` | Go gRPC plugin | `which protoc-gen-go-grpc` |
| `nfpm` | `.deb` / `.rpm` packaging | `nfpm --version` |
| `makensis` | Windows installers | `makensis -VERSION` |
| `docker` | Container builds | `docker --version` |
| `mkdocs` | Documentation site | `mkdocs --version` |
| `git` | Source control | `git --version` |
| `make` | Build automation | `make --version` |
