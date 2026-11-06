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
| `.github/workflows/release.yml` | Runs `go test ./...`, then `make release-assets VERSION=1.2.0`, then builds the signed workload plugin packages (`make plugin-packages`), checks each one with `hitstor-pluginkit verify`, regenerates `SHA256SUMS`, and creates the GitHub release `v1.2.0` with every file in `dist/` attached. Release notes are generated from the commits since the previous tag. |
| `.github/workflows/docker-publish.yml` | Builds the Docker image for `linux/amd64` and `linux/arm64` and pushes it to GHCR (`ghcr.io/hooperits/hitstor`) tagged `1.2.0`, `1.2`, and `latest`. It can also be started by hand (`workflow_dispatch`). |

The version embedded in the binaries and in the artifact names is the tag without the leading `v`. Plugin packages keep their own version, taken from each `plugins/<id>/plugin.yaml`.

## Plugin Signing Key { #plugin-signing-key }

The release job runs in the GitHub environment `release`, which accepts deployments only from `v*` tags. Its secret `HITSTOR_PLUGIN_SIGNING_KEY` holds the base64 ed25519 seed of the official plugin key, and only the **Build signed plugin packages** step receives it. Each package is then verified against the official public keys embedded in the agent (`hitstor-pluginkit verify`), so a missing or wrong secret fails the release instead of publishing packages that agents would reject.

To rotate the key, generate a new pair with `hitstor-pluginkit keygen`, add the new public key to `OfficialKeys` in `internal/pluginpkg/trust.go` (keep the old one so existing packages stay valid), release an agent with it, and only then replace the secret.

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

The release workflow then adds the plugin packages to `dist/` (for example `hitstor-plugin-mssql-0.5.0-windows-amd64.hpk` and the `example` plugin for Linux and Windows) and regenerates `SHA256SUMS`.

`scripts/install-agent.sh` downloads `hitstor-agent-VERSION-linux-ARCH.run` from the latest release, so both `.run` architectures must be present.

## Build Locally

To reproduce the release assets on a workstation (requires Go, [nfpm](https://nfpm.goreleaser.com/), [NSIS](https://nsis.sourceforge.io/) and Python with Pillow for the Windows icon):

```bash
make release-assets VERSION=1.2.0
make plugin-packages PLUGIN_KEY_FILE=mykey.key   # or PLUGIN_UNSIGNED=1
make checksums
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
