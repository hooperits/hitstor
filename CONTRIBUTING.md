# Contributing to HitStor

Thanks for your interest in improving HitStor! Bug reports, documentation fixes and pull requests are welcome.

## Before you start

- For bugs and feature ideas, open an issue first so we can discuss the approach.
- For security problems, follow [SECURITY.md](SECURITY.md) instead of opening a public issue.

## Development

Requirements: Go 1.26+ (see `go.mod`). `protoc` is only needed if you change `.proto` files (`make proto`).

```bash
make build   # build binaries into bin/
make test    # run the test suite
```

See the [developer guide](docs/dev-guide/index.md) for the codebase overview, test lab and release process.

## Pull requests

- Keep changes focused; one feature or fix per pull request.
- Add or update tests. The project follows test-first, table-driven tests.
- Run `gofmt` and make sure `go vet ./...` and `go test ./...` pass.
- Follow the conventions in the README (error wrapping, `log/slog`, no `utils/` packages).
- Use [Conventional Commits](https://www.conventionalcommits.org/) for commit messages (`feat:`, `fix:`, `docs:` ...).

## License

By contributing, you agree that your contributions will be licensed under the
[GNU Affero General Public License v3.0](LICENSE), the same license as the project.
