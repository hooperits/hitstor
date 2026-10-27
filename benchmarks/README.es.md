[English](README.md) | **Español**

# Pruebas de rendimiento de HitStor

Tiempos reproducibles de **respaldo / incremental / restauración** para cargas de trabajo de sistemas de archivos, con una comparación opcional con **restic**.

El objetivo es publicar cifras reproducibles frente a agentes de respaldo comparables.

## Inicio rápido

```bash
# Desde la raíz del repositorio (compila hitstor automáticamente)
./benchmarks/run.sh --profile small

# Corpus más grande (~1 GiB lógico, tarda más)
./benchmarks/run.sh --profile medium --workdir /var/tmp/hitstor-bench

# Solo HitStor
./benchmarks/run.sh --profile small --skip-restic

# Usar un binario existente
./benchmarks/run.sh --hitstor ./bin/hitstor --profile tiny
```

Los resultados se guardan en el directorio de trabajo (un directorio temporal de forma predeterminada):

| Archivo | Descripción |
|------|-------------|
| `results.md` | Tabla legible por personas (para pegar en PR / documentación) |
| `results.json` | Esquema v1 legible por máquinas |
| `corpus/` | Conjunto de datos generado |
| `*.log` | stdout/stderr de la herramienta en las ejecuciones cronometradas |

## Perfiles

| Perfil | Archivos aprox. | Tamaño lógico objetivo | Uso |
|---------|---------------|---------------------|-----|
| `tiny` | 50 | ~0.4 MiB | Prueba rápida en CI / depuración del script |
| `small` | 1 000 | ~100 MiB | Predeterminado en laptop |
| `medium` | 5 000 | ~1 GiB | Publicación desde el laboratorio |
| `large` | 20 000 | ~5 GiB | Publicación en hardware de servidor |

Todos los perfiles usan la **semilla 42** para el corpus y la **semilla 99** para el conjunto de modificaciones incrementales, de modo que dos máquinas generen el mismo árbol.

```bash
go run ./benchmarks/gen_corpus.go -out /tmp/corpus -profile small -seed 42
```

## Metodología (qué medimos)

1. **Respaldo completo**: repositorio vacío, corpus completo.
2. **Incremental**: se agregan 5 bytes a ~10 % de los archivos y se vuelve a respaldar.
3. **Restauración**: se restaura `latest` en un directorio vacío.
4. **Tamaño del repositorio**: suma de los tamaños de archivo dentro del directorio del repositorio (bytes en disco, no lógicos).

El rendimiento es `source_bytes / wall_seconds` (MiB/s). El paralelismo usa por defecto el modo automático de HitStor (`--parallel 0`); puede cambiarlo con `HITSTOR_BENCH_PARALLEL`.

### Notas para una comparación justa

- Mismo host, mismo perfil, misma semilla.
- No mezcle ejecuciones con caché fría y con caché caliente al citar cifras.
- Para afirmaciones públicas, informe la **mediana de ≥3 ejecuciones** e incluya el tipo de CPU/RAM/disco.
- restic es opcional; si no está instalado, el arnés de pruebas igual produce las cifras de HitStor.
- Este arnés mide el respaldo/restauración con la **CLI local** (no el RPC agente→servidor). La ruta del agente es un ejercicio de laboratorio aparte.

## Publicar resultados

1. Ejecute las pruebas en hardware representativo (documente el disco: NVMe / HDD / red).
2. Conserve `results.json` + `results.md`.
3. Copie la tabla en Markdown en:
   - la descripción del PR, o
   - la sección “Latest lab run” de `docs/dev-guide/benchmarks.md` (con la fecha y el tipo de host).
4. Nunca afirme que algo es “más rápido que X” sin la fila de restic correspondiente de la **misma** ejecución.

### Esquema JSON (v1)

```json
{
  "schema_version": 1,
  "timestamp_utc": "RFC3339",
  "profile": "small",
  "source_bytes": 0,
  "host": { "os": "", "cpus": 0, "go_version": "" },
  "hitstor": {
    "version": "",
    "full_backup_sec": 0,
    "incremental_backup_sec": 0,
    "restore_sec": 0,
    "repo_bytes": 0,
    "full_throughput_mib_s": 0,
    "restore_throughput_mib_s": 0
  },
  "restic": null
}
```

## Makefile

```bash
make bench              # profile=small
make bench PROFILE=medium
make bench-skip-restic
```
