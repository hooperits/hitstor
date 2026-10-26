> **Sample only** — captured on a developer WSL2 host with profile `tiny` (~0.4 MiB).
> Not for product claims. Re-run `./benchmarks/run.sh --profile medium` on target hardware
> and replace the “Latest lab run” table in `docs/dev-guide/benchmarks.md`.

# HitStor benchmark results

- **When (UTC):** 2026-07-23T17:30:14Z
- **Profile:** `tiny` (0.4 MiB logical source)
- **Host:** wisdom · Linux-6.6.87.2-microsoft-standard-WSL2-x86_64-with-glibc2.39 · 6 CPUs · 26675936 kB
- **Go:** go version go1.24.13 linux/amd64
- **HitStor:** hitstor vdev (commit unknown, built unknown, linux/amd64)

## Wall-clock times (seconds)

| Tool | Full backup | Incremental | Restore | Repo size (MiB) | Full MiB/s | Restore MiB/s |
|------|------------:|------------:|--------:|----------------:|-----------:|--------------:|
| HitStor | 0.32 | 0.18 | 0.12 | 0.4 | 1.2 | 3.4 |

## Notes

- Corpus is deterministic (gen_corpus.go, seed 42).
- Incremental mutates ~10% of files (seed 99) then re-runs backup.
- Timings are single-run wall clock on a quiet machine; publish averages of >=3 runs for marketing claims.
- Compare only numbers generated with the same profile, host class, and tool versions.

Raw JSON: `results.json`
