# Pruebas

Esta página explica cómo ejecutar las pruebas y revisar la cobertura, y documenta los problemas conocidos encontrados durante el desarrollo.

## Pruebas unitarias { #unit-tests }

```bash
make test
```

Ejecuta `go test ./...` en todos los paquetes. No requiere dependencias externas ni binarios compilados.

## Pruebas de integración { #integration-tests }

```bash
make test-integration
```

Ejecuta `go test -tags integration ./...`. Las pruebas de integración de plugins compilan con `go build` los plugins que necesitan (por ejemplo, `plugins/example`), así que no requieren binarios compilados previamente. Las pruebas que dependen de servicios externos se omiten solas cuando el servicio no está disponible: las del backend S3 y del depurador (pruner) esperan MinIO en `localhost:9000`, y las de Sentinel en la nube necesitan credenciales y variables de bucket como `HITSTOR_GCS_BUCKET` o `HITSTOR_S3_BUCKET`.

`.github/workflows/release.yml` ejecuta `go test ./...` (solo pruebas unitarias) antes de compilar una versión.

## Análisis estático (linting) { #linting }

```bash
make lint
```

Ejecuta `go vet ./...` para detectar problemas comunes.

## Cobertura de código { #code-coverage }

Genere un informe de cobertura y visualícelo en el navegador:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

Objetivo: **80 % o más por paquete**.

## Problemas conocidos { #known-gotchas }

Problemas encontrados durante el desarrollo que vale la pena recordar:

| Problema | Solución |
|----------|----------|
| Firma de `GRPCClient` en `go-plugin` v1.7.0 | El primer parámetro debe ser `context.Context` |
| Las pruebas gRPC con bufconn fallan silenciosamente con mensajes grandes | Suba los límites en ambos lados: `grpc.MaxRecvMsgSize`/`grpc.MaxSendMsgSize` en el servidor de prueba y `grpc.MaxCallRecvMsgSize`/`grpc.MaxCallSendMsgSize` en las opciones de llamada por defecto del cliente (las pruebas usan `math.MaxInt32`) |
| Fallan las importaciones entre archivos proto de plugins | Use la ruta de inclusión `-I api/proto` (ya está en el target `proto-plugin` del Makefile) |
| El streaming difiere entre tipos de plugin | `Scan` del escáner y `Backup` de carga de trabajo envían un stream desde el plugin, y `Restore` de carga de trabajo recibe un stream; los plugins de almacenamiento, notificador y hook solo usan RPC unarias |
| Pruebas rotas después de refactorizar el tipo del escáner | Use `type ScanResult = scanner.ScanResult` (alias de tipo) en lugar de un tipo nuevo |
