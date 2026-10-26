#!/usr/bin/env bash
# Reproducible HitStor backup/restore benchmarks (optional restic comparison).
#
# Usage:
#   ./benchmarks/run.sh [--profile small|medium|large|tiny] [--workdir DIR] [--skip-restic]
#
# Outputs:
#   $WORKDIR/results.json
#   $WORKDIR/results.md
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PROFILE="small"
WORKDIR="${HITSTOR_BENCH_DIR:-}"
SKIP_RESTIC=0
HITSTOR_BIN="${HITSTOR_BIN:-}"
PARALLEL="${HITSTOR_BENCH_PARALLEL:-0}"
PASSWORD="hitstor-bench-password"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile) PROFILE="$2"; shift 2 ;;
    --workdir) WORKDIR="$2"; shift 2 ;;
    --skip-restic) SKIP_RESTIC=1; shift ;;
    --hitstor) HITSTOR_BIN="$2"; shift 2 ;;
    -h|--help)
      sed -n '2,12p' "$0"
      exit 0
      ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$WORKDIR" ]]; then
  WORKDIR="$(mktemp -d /tmp/hitstor-bench-XXXXXX)"
fi
mkdir -p "$WORKDIR"
CORPUS="$WORKDIR/corpus"
REPO_HS="$WORKDIR/repo-hitstor"
REPO_RS="$WORKDIR/repo-restic"
RESTORE_HS="$WORKDIR/restore-hitstor"
RESTORE_RS="$WORKDIR/restore-restic"
RESULTS_JSON="$WORKDIR/results.json"
RESULTS_MD="$WORKDIR/results.md"

log() { printf '[bench] %s\n' "$*"; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1" >&2; exit 1; }
}

need_cmd go
need_cmd python3
need_cmd du
need_cmd date

if [[ -z "$HITSTOR_BIN" ]]; then
  log "building hitstor…"
  (cd "$ROOT" && CGO_ENABLED=0 go build -o "$WORKDIR/hitstor" ./cmd/hitstor/)
  HITSTOR_BIN="$WORKDIR/hitstor"
fi
[[ -x "$HITSTOR_BIN" ]] || { echo "hitstor binary not executable: $HITSTOR_BIN" >&2; exit 1; }

log "workdir=$WORKDIR profile=$PROFILE"

log "generating corpus…"
(cd "$ROOT" && go run ./benchmarks/gen_corpus.go -out "$CORPUS" -profile "$PROFILE" -seed 42)

SOURCE_BYTES=$(python3 - <<PY
import os
total=0
for root, dirs, files in os.walk("$CORPUS"):
    for f in files:
        if f == "CORPUS.txt":
            continue
        total += os.path.getsize(os.path.join(root, f))
print(total)
PY
)

host_meta() {
  python3 - <<'PY'
import json, os, platform, socket, subprocess, sys
cpus = os.cpu_count() or 0
mem = ""
try:
    with open("/proc/meminfo") as f:
        for line in f:
            if line.startswith("MemTotal:"):
                mem = line.split()[1] + " kB"
                break
except FileNotFoundError:
    pass
print(json.dumps({
    "hostname": socket.gethostname(),
    "os": platform.platform(),
    "arch": platform.machine(),
    "cpus": cpus,
    "mem_total": mem,
    "go_version": subprocess.check_output(["go", "version"], text=True).strip(),
}))
PY
}

time_cmd() {
  # Prints seconds as float on stdout; command output goes to log file.
  local logf="$1"; shift
  python3 - "$logf" "$@" <<'PY'
import subprocess, sys, time
logf = sys.argv[1]
cmd = sys.argv[2:]
t0 = time.perf_counter()
with open(logf, "wb") as lf:
    p = subprocess.run(cmd, stdout=lf, stderr=subprocess.STDOUT)
dt = time.perf_counter() - t0
if p.returncode != 0:
    sys.stderr.write(open(logf, "r", errors="replace").read()[-4000:])
    sys.exit(p.returncode)
print(f"{dt:.6f}")
PY
}

dir_bytes() {
  # Apparent size of directory contents (portable).
  python3 - <<PY
import os
total=0
for root, dirs, files in os.walk("$1"):
    for f in files:
        try:
            total += os.path.getsize(os.path.join(root, f))
        except OSError:
            pass
print(total)
PY
}

# --- HitStor ---
export HITSTOR_PASSWORD="$PASSWORD"
log "hitstor init…"
rm -rf "$REPO_HS"
mkdir -p "$REPO_HS"
"$HITSTOR_BIN" --repo "$REPO_HS" --password "$PASSWORD" --quiet init >/dev/null

HS_FLAGS=(--repo "$REPO_HS" --password "$PASSWORD" --quiet)
if [[ "$PARALLEL" != "0" ]]; then
  HS_FLAGS+=(--parallel "$PARALLEL")
fi

log "hitstor full backup…"
HS_FULL_SEC=$(time_cmd "$WORKDIR/hitstor-full.log" \
  "$HITSTOR_BIN" "${HS_FLAGS[@]}" backup "$CORPUS")

# Mutate ~10% of files for incremental (deterministic).
log "mutating corpus for incremental…"
python3 - <<PY
import os, random
rng = random.Random(99)
files=[]
for root, dirs, fs in os.walk("$CORPUS"):
    for f in fs:
        if f == "CORPUS.txt":
            continue
        files.append(os.path.join(root, f))
files.sort()
n = max(1, len(files)//10)
for path in rng.sample(files, n):
    with open(path, "ab") as fh:
        fh.write(b"\nINCR\n")
print(f"mutated {n} files")
PY

log "hitstor incremental backup…"
HS_INCR_SEC=$(time_cmd "$WORKDIR/hitstor-incr.log" \
  "$HITSTOR_BIN" "${HS_FLAGS[@]}" backup "$CORPUS")

log "hitstor restore…"
rm -rf "$RESTORE_HS"
mkdir -p "$RESTORE_HS"
HS_RESTORE_SEC=$(time_cmd "$WORKDIR/hitstor-restore.log" \
  "$HITSTOR_BIN" "${HS_FLAGS[@]}" restore latest "$RESTORE_HS")

HS_REPO_BYTES=$(dir_bytes "$REPO_HS")
HS_VERSION=$("$HITSTOR_BIN" version 2>/dev/null | head -1 || echo unknown)

# --- restic (optional) ---
RS_FULL_SEC=""
RS_INCR_SEC=""
RS_RESTORE_SEC=""
RS_REPO_BYTES=""
RS_VERSION=""
if [[ "$SKIP_RESTIC" -eq 0 ]] && command -v restic >/dev/null 2>&1; then
  export RESTIC_PASSWORD="$PASSWORD"
  log "restic init…"
  rm -rf "$REPO_RS"
  mkdir -p "$REPO_RS"
  restic -r "$REPO_RS" init --quiet >/dev/null

  log "restic full backup…"
  # Recreate clean corpus for fair full-backup comparison (mutation was applied).
  # We already mutated — regenerating is cleaner for full; re-run gen for full restic
  # would differ. Use current corpus for both incremental path consistency:
  # For published numbers we re-generate a pristine corpus for restic full, then same mutation.
  PRISTINE="$WORKDIR/corpus-pristine"
  (cd "$ROOT" && go run ./benchmarks/gen_corpus.go -out "$PRISTINE" -profile "$PROFILE" -seed 42) >/dev/null

  RS_FULL_SEC=$(time_cmd "$WORKDIR/restic-full.log" \
    restic -r "$REPO_RS" backup --quiet "$PRISTINE")

  python3 - <<PY
import os, random
rng = random.Random(99)
files=[]
for root, dirs, fs in os.walk("$PRISTINE"):
    for f in fs:
        if f == "CORPUS.txt":
            continue
        files.append(os.path.join(root, f))
files.sort()
n = max(1, len(files)//10)
for path in rng.sample(files, n):
    with open(path, "ab") as fh:
        fh.write(b"\nINCR\n")
PY

  log "restic incremental backup…"
  RS_INCR_SEC=$(time_cmd "$WORKDIR/restic-incr.log" \
    restic -r "$REPO_RS" backup --quiet "$PRISTINE")

  log "restic restore…"
  rm -rf "$RESTORE_RS"
  mkdir -p "$RESTORE_RS"
  RS_RESTORE_SEC=$(time_cmd "$WORKDIR/restic-restore.log" \
    restic -r "$REPO_RS" restore latest --target "$RESTORE_RS" --quiet)

  RS_REPO_BYTES=$(dir_bytes "$REPO_RS")
  RS_VERSION=$(restic version 2>/dev/null | head -1 || echo restic)
else
  log "restic not found or --skip-restic; HitStor-only results"
fi

HOST_JSON=$(host_meta)
TS=$(date -u +%Y-%m-%dT%H:%M:%SZ)

export BENCH_SOURCE_BYTES="$SOURCE_BYTES"
export BENCH_HS_FULL_SEC="$HS_FULL_SEC"
export BENCH_HS_INCR_SEC="$HS_INCR_SEC"
export BENCH_HS_RESTORE_SEC="$HS_RESTORE_SEC"
export BENCH_HS_REPO_BYTES="$HS_REPO_BYTES"
export BENCH_HS_VERSION="$HS_VERSION"
export BENCH_RS_FULL_SEC="$RS_FULL_SEC"
export BENCH_RS_INCR_SEC="$RS_INCR_SEC"
export BENCH_RS_RESTORE_SEC="$RS_RESTORE_SEC"
export BENCH_RS_REPO_BYTES="$RS_REPO_BYTES"
export BENCH_RS_VERSION="$RS_VERSION"
export BENCH_PROFILE="$PROFILE"
export BENCH_PARALLEL="$PARALLEL"
export BENCH_TS="$TS"
export BENCH_HOST_JSON="$HOST_JSON"
export BENCH_RESULTS_JSON="$RESULTS_JSON"
export BENCH_RESULTS_MD="$RESULTS_MD"
export BENCH_WORKDIR="$WORKDIR"

python3 - <<'PY'
import json, os

def thr(bytes_, sec):
    if sec <= 0:
        return 0.0
    return (bytes_ / (1024 * 1024)) / sec

def mib(n):
    return n / (1024 * 1024)

source_bytes = int(os.environ["BENCH_SOURCE_BYTES"])
hs_full = float(os.environ["BENCH_HS_FULL_SEC"])
hs_incr = float(os.environ["BENCH_HS_INCR_SEC"])
hs_restore = float(os.environ["BENCH_HS_RESTORE_SEC"])
hs_repo = int(os.environ["BENCH_HS_REPO_BYTES"])
profile = os.environ["BENCH_PROFILE"]

result = {
    "schema_version": 1,
    "timestamp_utc": os.environ["BENCH_TS"],
    "profile": profile,
    "source_bytes": source_bytes,
    "source_mib": round(mib(source_bytes), 2),
    "parallel": os.environ["BENCH_PARALLEL"],
    "host": json.loads(os.environ["BENCH_HOST_JSON"]),
    "hitstor": {
        "version": os.environ.get("BENCH_HS_VERSION", "").strip(),
        "full_backup_sec": hs_full,
        "incremental_backup_sec": hs_incr,
        "restore_sec": hs_restore,
        "repo_bytes": hs_repo,
        "full_throughput_mib_s": round(thr(source_bytes, hs_full), 2),
        "restore_throughput_mib_s": round(thr(source_bytes, hs_restore), 2),
    },
    "restic": None,
}

rs_full = os.environ.get("BENCH_RS_FULL_SEC") or ""
if rs_full:
    rs_full_f = float(rs_full)
    rs_incr_f = float(os.environ["BENCH_RS_INCR_SEC"])
    rs_restore_f = float(os.environ["BENCH_RS_RESTORE_SEC"])
    rs_repo = int(os.environ["BENCH_RS_REPO_BYTES"])
    result["restic"] = {
        "version": os.environ.get("BENCH_RS_VERSION", "").strip(),
        "full_backup_sec": rs_full_f,
        "incremental_backup_sec": rs_incr_f,
        "restore_sec": rs_restore_f,
        "repo_bytes": rs_repo,
        "full_throughput_mib_s": round(thr(source_bytes, rs_full_f), 2),
        "restore_throughput_mib_s": round(thr(source_bytes, rs_restore_f), 2),
    }

results_json = os.environ["BENCH_RESULTS_JSON"]
results_md = os.environ["BENCH_RESULTS_MD"]
with open(results_json, "w", encoding="utf-8") as f:
    json.dump(result, f, indent=2)
    f.write("\n")

hs = result["hitstor"]
lines = [
    "# HitStor benchmark results",
    "",
    f"- **When (UTC):** {result['timestamp_utc']}",
    f"- **Profile:** `{result['profile']}` ({result['source_mib']} MiB logical source)",
    (
        f"- **Host:** {result['host'].get('hostname')} · {result['host'].get('os')} · "
        f"{result['host'].get('cpus')} CPUs · {result['host'].get('mem_total') or 'n/a mem'}"
    ),
    f"- **Go:** {result['host'].get('go_version')}",
    f"- **HitStor:** {hs['version']}",
]
if result["restic"]:
    lines.append(f"- **restic:** {result['restic']['version']}")
lines += [
    "",
    "## Wall-clock times (seconds)",
    "",
    "| Tool | Full backup | Incremental | Restore | Repo size (MiB) | Full MiB/s | Restore MiB/s |",
    "|------|------------:|------------:|--------:|----------------:|-----------:|--------------:|",
]

def row(name, d):
    return (
        f"| {name} | {d['full_backup_sec']:.2f} | {d['incremental_backup_sec']:.2f} | "
        f"{d['restore_sec']:.2f} | {mib(d['repo_bytes']):.1f} | "
        f"{d['full_throughput_mib_s']:.1f} | {d['restore_throughput_mib_s']:.1f} |"
    )

lines.append(row("HitStor", hs))
if result["restic"]:
    lines.append(row("restic", result["restic"]))
lines += [
    "",
    "## Notes",
    "",
    "- Corpus is deterministic (gen_corpus.go, seed 42).",
    "- Incremental mutates ~10% of files (seed 99) then re-runs backup.",
    "- Timings are single-run wall clock on a quiet machine; publish averages of >=3 runs for marketing claims.",
    "- Compare only numbers generated with the same profile, host class, and tool versions.",
    "",
    f"Raw JSON: `{os.path.basename(results_json)}`",
    "",
]

text = "\n".join(lines)
with open(results_md, "w", encoding="utf-8") as f:
    f.write(text)
print(text)
print(f"\n[bench] wrote {results_json}")
print(f"[bench] wrote {results_md}")
print(f"[bench] workdir {os.environ['BENCH_WORKDIR']}")
PY
