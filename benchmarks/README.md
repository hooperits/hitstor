**English** | [Español](README.es.md)

# HitStor performance benchmarks

Reproducible **backup / incremental / restore** timings for filesystem workloads, with an optional **restic** comparison.

The goal is published, reproducible numbers against comparable backup agents.

## Quick start

```bash
# From repo root (builds hitstor automatically)
./benchmarks/run.sh --profile small

# Larger corpus (~1 GiB logical, longer)
./benchmarks/run.sh --profile medium --workdir /var/tmp/hitstor-bench

# HitStor only
./benchmarks/run.sh --profile small --skip-restic

# Use an existing binary
./benchmarks/run.sh --hitstor ./bin/hitstor --profile tiny
```

Artifacts land in the work directory (temp dir by default):

| File | Description |
|------|-------------|
| `results.md` | Human-readable table (paste into PRs / docs) |
| `results.json` | Machine-readable schema v1 |
| `corpus/` | Generated dataset |
| `*.log` | Tool stdout/stderr for the timed runs |

## Profiles

| Profile | Approx. files | Target logical size | Use |
|---------|---------------|---------------------|-----|
| `tiny` | 50 | ~0.4 MiB | CI smoke / script debug |
| `small` | 1 000 | ~100 MiB | Laptop default |
| `medium` | 5 000 | ~1 GiB | Lab publish |
| `large` | 20 000 | ~5 GiB | Server-class publish |

All profiles use **seed 42** for the corpus and **seed 99** for the incremental mutation set so two machines generate the same tree.

```bash
go run ./benchmarks/gen_corpus.go -out /tmp/corpus -profile small -seed 42
```

## Methodology (what we measure)

1. **Full backup** — empty repo, entire corpus.
2. **Incremental** — append 5 bytes to ~10% of files, backup again.
3. **Restore** — restore `latest` into an empty directory.
4. **Repo size** — sum of file sizes under the repository directory (on-disk bytes, not logical).

Throughput is `source_bytes / wall_seconds` (MiB/s). Parallelism defaults to HitStor auto (`--parallel 0`); override with `HITSTOR_BENCH_PARALLEL`.

### Fair comparison notes

- Same host, same profile, same seed.
- Do not mix cold-cache and warm-cache runs when quoting numbers.
- For public claims, report **median of ≥3 runs** and include CPU/RAM/disk type.
- restic is optional; if missing, the harness still produces HitStor numbers.
- This harness measures **local CLI** backup/restore (not agent→server RPC). Agent path is a separate lab exercise.

## Publishing results

1. Run on representative hardware (document disk: NVMe / HDD / network).
2. Keep `results.json` + `results.md`.
3. Copy the markdown table into:
   - PR description, or
   - `docs/dev-guide/benchmarks.md` “Latest lab run” section (with date + host class).
4. Never claim “faster than X” without the matching restic row from the **same** run.

### JSON schema (v1)

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
