# Release Process

Releases are built and published by GitHub Actions when a version tag is pushed.

## Cut a Release

```bash
git tag -a v1.2.0 -m "HitStor 1.2.0"
git push origin v1.2.0
```

The tag starts two workflows:

| Workflow | What it does |
|----------|--------------|
| `.github/workflows/release.yml` | Runs `go test ./...`, then `make release-assets VERSION=1.2.0`, and creates the GitHub release `v1.2.0` with every file in `dist/` attached. Release notes are generated from the commits since the previous tag. |
| `.github/workflows/docker-publish.yml` | Builds the Docker image and pushes it to GHCR (`ghcr.io/hooperits/hitstor`) tagged `1.2.0` and `1.2`. |

The version embedded in the binaries and in the artifact names is the tag without the leading `v`.

After the workflows finish, deploy the documentation if it changed:

```bash
make docs-deploy
```

## Release Assets

`make release-assets` runs `clean`, `build-all`, `pkg-linux`, `pkg-windows`, the `.run` installers for amd64 and arm64, and `checksums`. For `VERSION=1.2.0`, `dist/` contains:

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

`scripts/install-agent.sh` downloads `hitstor-agent-VERSION-linux-ARCH.run` from the latest release, so both `.run` architectures must be present.

## Build Locally

To reproduce the release assets on a workstation (requires Go, [nfpm](https://nfpm.goreleaser.com/), [NSIS](https://nsis.sourceforge.io/) and Python with Pillow for the Windows icon):

```bash
make release-assets VERSION=1.2.0
```

`make release VERSION=1.2.0` builds the same assets plus the Docker image tarball (`dist/hitstor-1.2.0-docker.tar.gz`, requires Docker).

## VERSION Variable

The `VERSION` variable is passed to all build steps via Go linker flags (`-ldflags`). It controls:

- The version string embedded in the binary (`hitstor version`)
- Package version in `.deb`, `.rpm` and Windows installer metadata
- Docker image tag (`hitstor:VERSION`)
- Output file names in `dist/`

Two additional variables are set automatically:

| Variable | Source | Example |
|----------|--------|---------|
| `VERSION` | Makefile argument (default: `dev`) | `1.2.0` |
| `COMMIT` | `git rev-parse --short HEAD` | `9341885` |
| `DATE` | `date -u +%Y-%m-%d` | `2026-02-24` |

## Partial Builds

| Command | What It Builds |
|---------|----------------|
| `make release-linux` | `.deb` + `.rpm` packages only |
| `make installers` | `.run` installers (amd64) only |
| `make pkg-windows` | Windows `.exe` installers only |
| `make release-docker` | Docker image tarball only |
| `make checksums` | `SHA256SUMS` for everything in `dist/` |

## Clean

Remove all build artifacts:

```bash
make clean
```

This deletes `bin/`, `dist/`, and `site/`.
