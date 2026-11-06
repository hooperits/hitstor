# Testing

This page covers how to run tests, check coverage, and documents known gotchas encountered during development.

## Unit Tests

```bash
make test
```

Runs `go test ./...` across all packages. No external dependencies or built binaries required.

## Integration Tests

```bash
make test-integration
```

Runs `go test -tags integration ./...`. The plugin integration tests build the plugins they need (for example `plugins/example`) with `go build`, so no pre-built binaries are required. Tests that need external services skip themselves when the service is not available: the S3 backend and pruner tests expect MinIO at `localhost:9000`, and the Sentinel cloud tests need cloud credentials and bucket variables such as `HITSTOR_GCS_BUCKET` or `HITSTOR_S3_BUCKET`.

`.github/workflows/release.yml` runs `go test ./...` (unit tests only) before building a release.

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

Target: **80%+ per package**.

## Known Gotchas

Issues encountered during development that are worth remembering:

| Issue | Solution |
|-------|----------|
| `go-plugin` v1.7.0 `GRPCClient` signature | First parameter must be `context.Context` |
| bufconn gRPC tests fail silently on large messages | Raise the limits on both sides: `grpc.MaxRecvMsgSize`/`grpc.MaxSendMsgSize` on the test server and `grpc.MaxCallRecvMsgSize`/`grpc.MaxCallSendMsgSize` in the client's default call options (the tests use `math.MaxInt32`) |
| Plugin proto cross-file imports fail | Use the `-I api/proto` include path (already in the `proto-plugin` Makefile target) |
| Streaming differs between plugin types | The scanner's `Scan` and the workload `Backup` stream from the plugin, and workload `Restore` streams to it; storage, notifier, and hook plugins use unary RPCs only |
| Test breakage after scanner type refactor | Use `type ScanResult = scanner.ScanResult` (type alias) instead of a new type |
