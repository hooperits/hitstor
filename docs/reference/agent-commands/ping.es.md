# hitstor-agent ping

Prueba la conectividad con el servidor.

## Sinopsis { #synopsis }

```
hitstor-agent ping [flags]
```

## Descripción { #description }

Envía una solicitud gRPC Ping al servidor remoto para verificar la conectividad y la validación del certificado TLS. Si tiene éxito, imprime la dirección del servidor, su versión y el ID del repositorio.

## Ejemplos { #examples }

```bash
hitstor-agent ping --server backup.internal:8420 --api-key YOUR_KEY
```

Salida correcta:

```
Server: backup.internal:8420
Version: 0.2.0
Repo ID: 3d3f89b7c1e24a6f0b9d2e8a4c7f1356e2a9d0b4f8c3e17a6b5d9f2c0e4a8b713
```
