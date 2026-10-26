# hitstor-agent ping

Test connectivity to the server.

## Synopsis

```
hitstor-agent ping [flags]
```

## Description

Sends a gRPC Ping request to the remote server to verify connectivity, TLS certificate validation, and API key authentication. Prints the server version on success.

## Examples

```bash
hitstor-agent ping --server backup.internal:8420 --api-key YOUR_KEY
```

Successful output:

```
pong: server v0.1.0
```
