# Testing

This page covers how to run tests, check coverage, and documents known gotchas encountered during development.

## Unit Tests

```bash
make test
```

Runs `go test ./...` across all packages. No external dependencies or built binaries required.

## Integration Tests

```bash
make build agent
make test-integration
```

Runs `go test -tags integration ./...`. Integration tests require pre-built server and agent binaries.

## Linting

```bash
make lint
```

Runs `go vet ./...` to catch common issues.

## Code Coverage

Generate a coverage report and view it in the browser:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

Target: **80%+ per package** (achieved in feature 008).

## Known Gotchas

Issues encountered during development that are worth remembering:

| Issue | Solution |
|-------|----------|
| `go-plugin` v1.7.0 `GRPCClient` signature | First parameter must be `context.Context` |
| bufconn gRPC tests fail silently on large messages | Add `grpc.MaxRecvMsgSize(math.MaxInt32)` to dial options |
| Proto cross-file imports fail | Use `-I api/proto` include path (already in Makefile) |
| Scanner plugin streaming differs from other plugins | Scanner uses server-side streaming; the other 3 plugin types use unary RPCs |
| Test breakage after scanner type refactor | Use `type ScanResult = scanner.ScanResult` (type alias) instead of a new type |
