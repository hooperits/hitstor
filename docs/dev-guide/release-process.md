# Release Process

This page documents how to produce a full set of release artifacts and publish them.

## Full Release

```bash
make release VERSION=1.2.0
```

This runs the following steps in order:

1. `clean` — removes `bin/`, `dist/`, `site/`
2. `build-all` — compiles all server and agent binaries (linux, windows, arm64)
3. `pkg-linux` — builds `.deb` and `.rpm` packages for server and agent
4. `docker-save` — builds and exports the Docker image as a tarball
5. `checksums` — generates `SHA256SUMS` for all files in `dist/`

## Output

After a successful release, `dist/` contains:

```
dist/
├── hitstor_1.2.0_amd64.deb
├── hitstor-1.2.0.x86_64.rpm
├── hitstor-agent_1.2.0_amd64.deb
├── hitstor-agent-1.2.0.x86_64.rpm
├── hitstor-1.2.0-docker.tar.gz
└── SHA256SUMS
```

## VERSION Variable

The `VERSION` variable is passed to all build steps via Go linker flags (`-ldflags`). It controls:

- The version string embedded in the binary (`hitstor version`)
- Package version in `.deb` and `.rpm` metadata
- Docker image tag (`hitstor:VERSION`)
- Output file names in `dist/`

Two additional variables are set automatically:

| Variable | Source | Example |
|----------|--------|---------|
| `VERSION` | Makefile argument (default: `dev`) | `1.2.0` |
| `COMMIT` | `git rev-parse --short HEAD` | `9341885` |
| `DATE` | `date -u +%Y-%m-%d` | `2026-02-24` |

## Partial Releases

Build only a subset of artifacts:

| Command | What It Builds |
|---------|----------------|
| `make release-linux` | `.deb` + `.rpm` packages only |
| `make release-docker` | Docker image tarball only |
| `make pkg-windows` | Windows `.exe` installers only |
| `make checksums` | `SHA256SUMS` for everything in `dist/` |

## Post-Release Checklist

1. **Tag the commit**:

    ```bash
    $ git tag v1.2.0
    ```

2. **Push the tag**:

    ```bash
    $ git push origin v1.2.0
    ```

3. **Upload artifacts** to [GitHub Releases](https://github.com/hooperits/hitstor/releases):

    Upload all files from `dist/` to the new release.

4. **Deploy documentation**:

    ```bash
    $ make docs-deploy
    ```

## Clean

Remove all build artifacts:

```bash
make clean
```

This deletes `bin/`, `dist/`, and `site/`.
