# Benchmarks de rendimiento

HitStor incluye un **arnés reproducible de respaldo/restauración de sistemas de archivos** en `benchmarks/`.

Úselo para:

- Comparar versiones (misma máquina, mismo perfil)
- Obtener cifras opcionales comparativas con **restic**
- Respaldar afirmaciones publicadas con JSON sin procesar y la metodología

Detalle completo: `benchmarks/README.md` en el repositorio.

## Ejecutar { #run }

```bash
# Desde la raíz del repositorio
./benchmarks/run.sh --profile small

# O mediante Make
make bench
make bench PROFILE=medium
```

Salidas: `results.md` y `results.json` en el directorio de trabajo que se imprime al final de la ejecución.

## Perfiles { #profiles }

| Perfil | Propósito |
|---------|--------|
| `tiny` | Prueba rápida del script (~segundos) |
| `small` | Laboratorio predeterminado en laptop (~100 MiB lógicos) |
| `medium` | Ejecución de laboratorio publicable (~1 GiB lógico) |
| `large` | Prueba de estrés de clase servidor (~5 GiB lógicos) |

## Qué significa un “buen” resultado para la comunicación del producto { #what-good-looks-like-for-product-messaging }

HitStor es un producto de respaldo de sistemas de archivos ([Hoja de ruta](../admin-guide/roadmap.md)), así que cite cifras que importen a quienes compran respaldo de sistemas de archivos:

1. **MiB/s de respaldo completo** en disco local
2. Tiempo real del respaldo **incremental** tras un conjunto pequeño de cambios
3. **MiB/s de restauración** (confianza + RTO)
4. **Tamaño del repositorio en disco** frente al origen (efectividad de la deduplicación)

Acompañe siempre el rendimiento con la clase de hardware (CPU, RAM, NVMe frente a HDD).

## Última ejecución de laboratorio { #latest-lab-run }

Un ejemplo del formato (perfil `tiny`, **no** apto para marketing) se encuentra en
`benchmarks/results/sample-tiny.md`.

Reemplace esta sección con una ejecución **medium**/**large** en hardware representativo:

```bash
./benchmarks/run.sh --profile medium --workdir /var/tmp/hitstor-bench
# pegue abajo la tabla de results.md; archive results.json junto con la versión
```

| Herramienta | Respaldo completo | Incremental | Restauración | Tamaño del repositorio (MiB) | MiB/s completo | MiB/s restauración |
|------|------------:|------------:|--------:|----------------:|-----------:|--------------:|
| _pendiente (publicar ejecución medium/large)_ | — | — | — | — | — | — |

Véase también: verificación de restauración (`hitstor verify-restore`) para comprobar la exactitud después de restaurar.
