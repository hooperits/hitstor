# Pruebas

Esta página explica cómo ejecutar las pruebas y revisar la cobertura, y documenta los problemas conocidos encontrados durante el desarrollo.

## Pruebas unitarias { #unit-tests }

```bash
make test
```

Ejecuta `go test ./...` en todos los paquetes. No requiere dependencias externas ni binarios compilados.

## Pruebas de integración { #integration-tests }

```bash
make build agent
make test-integration
```

Ejecuta `go test -tags integration ./...`. Las pruebas de integración requieren los binarios del servidor y del agente compilados previamente.

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

Objetivo: **80 % o más por paquete** (alcanzado en la funcionalidad 008).

## Problemas conocidos { #known-gotchas }

Problemas encontrados durante el desarrollo que vale la pena recordar:

| Problema | Solución |
|----------|----------|
| Firma de `GRPCClient` en `go-plugin` v1.7.0 | El primer parámetro debe ser `context.Context` |
| Las pruebas gRPC con bufconn fallan silenciosamente con mensajes grandes | Agregue `grpc.MaxRecvMsgSize(math.MaxInt32)` a las opciones de conexión (dial) |
| Fallan las importaciones entre archivos proto | Use la ruta de inclusión `-I api/proto` (ya está en el Makefile) |
| El streaming del plugin escáner difiere del de los otros plugins | El escáner usa streaming del lado del servidor; los otros 3 tipos de plugin usan RPC unarias |
| Pruebas rotas después de refactorizar el tipo del escáner | Use `type ScanResult = scanner.ScanResult` (alias de tipo) en lugar de un tipo nuevo |
