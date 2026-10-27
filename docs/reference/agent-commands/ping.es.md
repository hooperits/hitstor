# hitstor-agent ping

Prueba la conectividad con el servidor.

## Sinopsis { #synopsis }

```
hitstor-agent ping [flags]
```

## Descripción { #description }

Envía una solicitud gRPC Ping al servidor remoto para verificar la conectividad, la validación del certificado TLS y la autenticación con la clave de API. Si tiene éxito, imprime la versión del servidor.

## Ejemplos { #examples }

```bash
hitstor-agent ping --server backup.internal:8420 --api-key YOUR_KEY
```

Salida correcta:

```
pong: server v0.1.0
```
