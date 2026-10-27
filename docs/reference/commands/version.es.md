# hitstor version

Muestra la información de la versión y busca actualizaciones.

## Sinopsis { #synopsis }

```
hitstor version [--check] [--json]
```

## Descripción { #description }

Imprime la versión de HitStor, el hash del commit, la fecha de compilación y la plataforma.

Con `--check`, consulta la API de GitHub Releases para determinar si hay una versión más reciente disponible.

## Flags { #flags }

| Flag | Descripción |
|------|-------------|
| `--check` | Busca actualizaciones disponibles |
| `--json` | Genera la salida en formato JSON |

## Ejemplos { #examples }

### Imprimir la versión { #print-version }

```bash
hitstor version
```

Salida:

```
hitstor v1.0.0 (commit a1b2c3d, built 2026-02-27, linux/amd64)
```

### Buscar actualizaciones { #check-for-updates }

```bash
hitstor version --check
```

Salida cuando hay una actualización disponible:

```
Update available: v1.1.0 (current: v1.0.0)
Release: https://github.com/hooperits/hitstor/releases/tag/v1.1.0
```

Salida cuando la versión está al día:

```
You are running the latest version (v1.0.0).
```

Salida cuando la verificación falla (error de red):

```
Could not check for updates.
```

## Verificación automática de versión { #automatic-version-check }

Cuando `update_check: true` está definido en la configuración del servidor (el valor predeterminado), el servidor busca actualizaciones cada 24 horas. Si hay una versión más reciente disponible, aparece un aviso en la parte superior del dashboard web.

Consulte la [Referencia de configuración](../config-reference.md) para conocer los parámetros `update_check` y `update_url`.
