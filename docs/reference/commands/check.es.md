# hitstor check

Verifica la integridad del repositorio.

## Sinopsis { #synopsis }

```
hitstor check [flags]
```

## Descripción { #description }

Comprueba la integridad estructural del repositorio. De forma predeterminada, verifica que todas las instantáneas (snapshots), los índices y los metadatos de los paquetes sean coherentes y legibles.

Con `--read-data`, además lee y descifra cada blob de cada archivo de paquete para verificar que ningún dato esté dañado. Es más lento, pero ofrece una verificación completa de extremo a extremo.

Los errores se muestran en stderr. El código de salida es distinto de cero si se encuentra algún error.

## Flags { #flags }

| Flag | Tipo | Valor predeterminado | Descripción |
|------|------|---------|-------------|
| `--read-data` | bool | false | Lee y verifica todos los datos de los paquetes (más lento, pero exhaustivo) |

Además de todos los [flags globales](index.md#global-flags), incluido `--repo-name` para configuraciones con varios repositorios.

## Ejemplos { #examples }

Comprobación estructural rápida:

```bash
hitstor check --repo /backups/hitstor --password "secret"
```

Verificación completa, incluidos todos los datos de los blobs:

```bash
hitstor check --repo /backups/hitstor --password "secret" --read-data
```

## Notas { #notes }

- Sin `--read-data`, la comprobación verifica la coherencia de los metadatos (las instantáneas hacen referencia a árboles válidos, los índices están bien formados, etc.), pero no lee el contenido de los archivos de paquete.
- Con `--read-data`, se descifra cada blob y se vuelve a verificar su ID. Esto detecta la degradación de bits (bit-rot), los archivos truncados y otros daños a nivel de almacenamiento.
