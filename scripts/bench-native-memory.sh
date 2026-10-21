#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "unsupported platform: this initial harness requires Linux and GNU /usr/bin/time" >&2
  exit 2
fi
if [[ ! -x /usr/bin/time ]] || ! /usr/bin/time --version >/dev/null 2>&1; then
  echo "GNU /usr/bin/time is required" >&2
  exit 2
fi
for tool in go python3 git; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "required tool not found: $tool" >&2
    exit 2
  fi
done

REPEATS="$(printenv ASTER_BENCH_REPEATS 2>/dev/null || printf '5')"
if ! [[ "$REPEATS" =~ ^[1-9][0-9]*$ ]]; then
  echo "ASTER_BENCH_REPEATS must be a positive integer" >&2
  exit 2
fi

OUT=""
if [[ "$#" -gt 0 ]]; then
  OUT=$1
fi
if [[ -z "$OUT" ]]; then
  OUT="bench/native-memory-$(date -u +%Y%m%dT%H%M%SZ).out"
fi
if [[ "$OUT" != /* ]]; then
  OUT="$ROOT/$OUT"
fi
mkdir -p "$(dirname "$OUT")"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/src" "$TMP/bin"
RAW="$TMP/raw.tsv"
printf 'workload\trepetition\tpeak_rss_kib\telapsed_seconds\tbuild_ms\tbinary_bytes\n' > "$RAW"

python3 - "$TMP/src" <<'PY'
from pathlib import Path
import sys

target = Path(sys.argv[1])
(target / "noop.ast").write_text("print(0);\n", encoding="utf-8")

grow = """fn grow(value: string) -> string {
    let mut text = value;
    let mut count: i64 = 0;
    while count < 24 {
        text = text + text;
        count = count + 1;
    }
    return text;
}

"""
for count in (1, 4, 8):
    bindings = [f'let value{i} = grow("x");' for i in range(count)]
    total = " + ".join(f"len(value{i})" for i in range(count))
    source = grow + "\n".join(bindings) + f"\nprint({total});\n"
    (target / f"strings-{count}.ast").write_text(source, encoding="utf-8")
PY

{
  echo "# Native child-process memory benchmark"
  echo
  echo "- Measurement time (UTC): $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "- Source commit: $(git rev-parse HEAD)"
  echo "- OS/kernel: $(uname -srmo)"
  echo "- CPU: $(awk '/^model name[[:space:]]*:/ { sub(/^[^:]*:[[:space:]]*/, ""); print; exit }' /proc/cpuinfo)"
  echo "- Logical CPUs: $(getconf _NPROCESSORS_ONLN)"
  echo "- Go: $(go version)"
  echo "- Repetitions per workload: $REPEATS"
  echo "- RSS metric: GNU time %M, maximum resident set size in KiB for the child executable"
  echo "- Scope: build time is reported separately; peak RSS and elapsed time measure only the standalone generated Native executable, without an OS resource supervisor"
  echo
} > "$OUT"

ASTER="$TMP/aster"
go build -o "$ASTER" ./cmd/aster

for workload in noop strings-1 strings-4 strings-8; do
  source="$TMP/src/$workload.ast"
  binary="$TMP/bin/$workload"
  build_start="$(date +%s%N)"
  "$ASTER" build -o "$binary" "$source"
  build_end="$(date +%s%N)"
  build_ms="$(( (build_end - build_start) / 1000000 ))"
  binary_bytes="$(wc -c < "$binary" | tr -d ' ')"

  case "$workload" in
    noop) expected=0 ;;
    strings-1) expected=16777216 ;;
    strings-4) expected=67108864 ;;
    strings-8) expected=134217728 ;;
    *) echo "internal error: unknown workload $workload" >&2; exit 2 ;;
  esac

  for repetition in $(seq 1 "$REPEATS"); do
    metrics="$TMP/time.txt"
    stdout="$TMP/stdout.txt"
    stderr="$TMP/stderr.txt"
    if ! /usr/bin/time -f '%M %e' -o "$metrics" "$binary" > "$stdout" 2> "$stderr"; then
      echo "workload failed: $workload (iteration $repetition)" >&2
      cat "$stderr" >&2
      exit 1
    fi
    actual="$(tr -d '\r\n' < "$stdout")"
    if [[ "$actual" != "$expected" ]]; then
      echo "unexpected output for $workload: got '$actual', wanted '$expected'" >&2
      cat "$stderr" >&2
      exit 1
    fi
    read -r rss_kib elapsed_seconds < "$metrics"
    printf '%s\t%s\t%s\t%s\t%s\t%s\n' \
      "$workload" "$repetition" "$rss_kib" "$elapsed_seconds" "$build_ms" "$binary_bytes" >> "$RAW"
  done
done

{
  echo "## Raw samples"
  echo
  echo "Tab-separated values:"
  cat "$RAW"
  echo
} >> "$OUT"

python3 - "$RAW" "$OUT" <<'PY'
import csv
import statistics
import sys
from collections import defaultdict

raw_path, output_path = sys.argv[1:3]
groups = defaultdict(list)
with open(raw_path, newline="", encoding="utf-8") as handle:
    for row in csv.DictReader(handle, delimiter="\t"):
        row["peak_rss_kib"] = int(row["peak_rss_kib"])
        row["elapsed_seconds"] = float(row["elapsed_seconds"])
        row["build_ms"] = int(row["build_ms"])
        row["binary_bytes"] = int(row["binary_bytes"])
        groups[row["workload"]].append(row)

def summary(values):
    median = statistics.median(values)
    mad = statistics.median(abs(value - median) for value in values)
    spread = ((max(values) - min(values)) / median * 100) if median else 0.0
    return median, min(values), max(values), mad, spread

with open(output_path, "a", encoding="utf-8") as handle:
    handle.write("## Summary\n\n")
    handle.write("| Workload | Retained string payload | Peak RSS median (KiB) | Min–max (KiB) | MAD (KiB) | Range/median | Runtime median (s) | Build time (ms) | Binary size (bytes) |\n")
    handle.write("|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
    for workload in ("noop", "strings-1", "strings-4", "strings-8"):
        rows = groups[workload]
        rss = summary([row["peak_rss_kib"] for row in rows])
        runtime = statistics.median([row["elapsed_seconds"] for row in rows])
        build_ms = statistics.median([row["build_ms"] for row in rows])
        binary_bytes = int(statistics.median([row["binary_bytes"] for row in rows]))
        payload = {"noop": "0 MiB", "strings-1": "16 MiB", "strings-4": "64 MiB", "strings-8": "128 MiB"}[workload]
        handle.write(
            f"| {workload} | {payload} | {rss[0]:.1f} | {rss[1]}–{rss[2]} | "
            f"{rss[3]:.1f} | {rss[4]:.2f}% | {runtime:.6f} | {build_ms} | {binary_bytes} |\n"
        )
    handle.write(
        "\n## Interpretation notes\n\n"
        "- Peak RSS includes the Go runtime, mapped executable pages, and Aster runtime objects. It is not the language heap size, cumulative allocations, or a portable process-memory quota.\n"
        "- The string workloads retain independent ASCII strings of 16 MiB each. Temporary concatenation allocations and garbage-collector behavior may affect peak RSS.\n"
        "- Build time is elapsed time for aster build; its peak memory is not measured by this script. Runtime RSS is measured after building, in a separate fresh process.\n"
        "- There is intentionally no pass/fail resource threshold. Repeat on an otherwise idle host before comparing revisions; do not use a single runner measurement to choose a production limit.\n"
    )
PY

echo "Report written to $OUT"
