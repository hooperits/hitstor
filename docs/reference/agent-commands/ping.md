# hitstor-agent ping

Test connectivity to the server.

## Synopsis

```
hitstor-agent ping [flags]
```

## Description

Sends a gRPC Ping request to the remote server to verify connectivity and TLS certificate validation. Prints the server address, version, and repository ID on success.

## Examples

```bash
hitstor-agent ping --server backup.internal:8420 --api-key YOUR_KEY
```

Successful output:

```
Server: backup.internal:8420
Version: 0.2.0
Repo ID: 3d3f89b7c1e24a6f0b9d2e8a4c7f1356e2a9d0b4f8c3e17a6b5d9f2c0e4a8b713
```
