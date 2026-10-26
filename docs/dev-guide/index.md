# Dev Guide

The HitStor Dev Guide is an internal reference for building, packaging, testing, and releasing HitStor. It also covers preparing a fresh Ubuntu server to receive a HitStor installation.

## Topics

| Guide | Description |
|-------|-------------|
| [Codebase Overview](codebase-overview.md) | Architecture map, binaries, packages, and feature maturity. |
| [Dev Environment](dev-environment.md) | Install all prerequisite tools needed to build and package HitStor. |
| [Build & Packaging](build-and-packaging.md) | Compile binaries and produce `.deb`, `.rpm`, Windows, and Docker packages. |
| [Server Preparation](server-preparation.md) | Prepare an Ubuntu server before installing the HitStor `.deb` package. |
| [Testing](testing.md) | Run unit tests, integration tests, linting, and code coverage. |
| [Benchmarks](benchmarks.md) | Reproducible backup/restore timings (optional restic comparison). |
| [Release Process](release-process.md) | Cut a release, generate all artifacts, and publish to GitHub. |
