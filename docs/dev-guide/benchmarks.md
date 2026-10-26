# Performance benchmarks

HitStor ships a **reproducible filesystem backup/restore harness** under `benchmarks/`.

Use it to:

- Compare releases (same machine, same profile)
- Produce optional **restic** side-by-side numbers
- Support published claims with raw JSON + methodology

Full detail: `benchmarks/README.md` in the repository.

## Run

```bash
# From repository root
./benchmarks/run.sh --profile small

# Or via Make
make bench
make bench PROFILE=medium
```

Outputs: `results.md` and `results.json` in the work directory printed at the end of the run.

## Profiles

| Profile | Intent |
|---------|--------|
| `tiny` | Script smoke (~seconds) |
| `small` | Default laptop lab (~100 MiB logical) |
| `medium` | Publishable lab run (~1 GiB logical) |
| `large` | Server-class stress (~5 GiB logical) |

## What “good” looks like for product messaging

HitStor is a filesystem backup product ([Roadmap](../admin-guide/roadmap.md)), so quote numbers that matter to filesystem buyers:

1. **Full backup MiB/s** on local disk
2. **Incremental** wall time after small change set
3. **Restore MiB/s** (trust + RTO)
4. **On-disk repo size** vs source (dedup effectiveness)

Always pair throughput with hardware class (CPU, RAM, NVMe vs HDD).

## Latest lab run

A format example (profile `tiny`, **not** for marketing) lives at
`benchmarks/results/sample-tiny.md`.

Replace this section with a **medium**/**large** run on representative hardware:

```bash
./benchmarks/run.sh --profile medium --workdir /var/tmp/hitstor-bench
# paste results.md table below; archive results.json with the release
```

| Tool | Full backup | Incremental | Restore | Repo size (MiB) | Full MiB/s | Restore MiB/s |
|------|------------:|------------:|--------:|----------------:|-----------:|--------------:|
| _pending (publish medium/large run)_ | — | — | — | — | — | — |

See also: restore verification (`hitstor verify-restore`) for correctness after restore.
