# hitstor init

Inicializa un nuevo repositorio de respaldo.

## Sinopsis { #synopsis }

```
hitstor init [flags]
```

## Descripción { #description }

Crea un nuevo repositorio de respaldo cifrado en la ubicación indicada por `--repo`. Esto crea la estructura de directorios (`data/`, `index/`, `snapshots/`, `keys/`, `locks/`, `config`) y genera una clave maestra cifrada con la contraseña proporcionada mediante derivación de clave scrypt.

El repositorio no debe existir previamente. Si la operación tiene éxito, se imprime el ID del repositorio.

## Flags { #flags }

Este comando usa solo los [flags globales](index.md#global-flags).

## Ejemplos { #examples }

Inicializar un repositorio local:

```bash
hitstor init --repo /backups/hitstor --password "my-secret-password"
```

Inicializar un repositorio S3:

```bash
hitstor init \
  --repo s3:http://localhost:9000/hitstor-backups \
  --password "my-secret-password" \
  --s3-access-key minioadmin \
  --s3-secret-key minioadmin
```

Usando variables de entorno:

```bash
export HITSTOR_REPO=/backups/hitstor
export HITSTOR_PASSWORD="my-secret-password"
hitstor init
```
