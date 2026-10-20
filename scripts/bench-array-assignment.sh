#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
output="${1:-bench/array-assignment-${timestamp}.out}"
case "$output" in
  /*) ;;
  *) output="$repo_root/$output" ;;
esac
mkdir -p "$(dirname "$output")"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required to summarize repeated benchmark results." >&2
  exit 1
fi

{
  printf '# Aster array-assignment benchmark report\n'
  printf 'timestamp_utc=%s\n' "$timestamp"
  printf 'commit=%s\n' "$(git rev-parse HEAD)"
  if [[ -z "$(git status --porcelain)" ]]; then
    printf 'working_tree_clean=yes\n'
  else
    printf 'working_tree_clean=no\n'
    printf 'warning=working tree has uncommitted changes; this report may not describe the commit alone\n'
  fi
  printf 'go_version=%s\n' "$(go version)"
  printf 'go_environment:\n'
  for variable in GOOS GOARCH GOAMD64 CGO_ENABLED GOTOOLCHAIN GOFLAGS; do
    printf '%s=%s\n' "$variable" "$(go env "$variable")"
  done
  printf 'GOMAXPROCS=%s\n' "${GOMAXPROCS:-default}"
  printf 'uname=%s\n' "$(uname -a)"

  if command -v lscpu >/dev/null 2>&1; then
    printf 'cpu_details:\n'
    lscpu | grep -E '^(Architecture|CPU\(s\)|Model name|Thread\(s\) per core|Core\(s\) per socket|Socket\(s\)):' || true
  elif command -v sysctl >/dev/null 2>&1; then
    printf 'cpu_model=%s\n' "$(sysctl -n machdep.cpu.brand_string 2>/dev/null || printf unknown)"
  fi

  printf '%s\n' "benchmark_command=go test ./tests -run '^
  printf '\n# Raw benchmark output\n'
  go test ./tests -run '^$' -bench '^Benchmark(ArrayElementAssignment|NativeProcessStartup)' -benchmem -count=5
} 2>&1 | tee "$output"

python3 - "$output" <<'PY' | tee -a "$output"
from collections import defaultdict
import re
import statistics
import sys

report_path = sys.argv[1]
benchmark_line = re.compile(
    r"^(Benchmark\S+)\s+\d+\s+([0-9]+(?:\.[0-9]+)?)\s+ns/op(?:\s+(.*))?$"
)
custom_metric = re.compile(r"([0-9]+(?:\.[0-9]+)?)\s+(ns/assignment|ns/process)\b")
samples = defaultdict(list)

with open(report_path, encoding="utf-8") as report:
    for line in report:
        match = benchmark_line.match(line.strip())
        if not match:
            continue
        benchmark, ns_per_op, trailing = match.groups()
        samples[(benchmark, "ns/op")].append(float(ns_per_op))
        for value, unit in custom_metric.findall(trailing or ""):
            samples[(benchmark, unit)].append(float(value))

print("\n# Repeated-run summary")
if not samples:
    print("No benchmark result lines were found; inspect the raw output above.")
    raise SystemExit(1)

for (benchmark, unit), values in sorted(samples.items()):
    median = statistics.median(values)
    mad = statistics.median(abs(value - median) for value in values)
    spread = (max(values) - min(values)) / median * 100 if median else 0.0
    print(
        f"{benchmark} [{unit}]: samples={len(values)}, "
        f"median={median:.3f}, min={min(values):.3f}, max={max(values):.3f}, "
        f"MAD={mad:.3f}, range/median={spread:.2f}%"
    )

print(
    "Spread statistics describe these repetitions only; they are not a confidence interval. "
    "Native ns/op and ns/assignment include child-process startup."
)
PY

printf 'Report saved to: %s\n' "$output"
 -bench '^Benchmark(ArrayElementAssignment|NativeProcessStartup)' -benchmem -count=5"
  printf '\n# Raw benchmark output\n'
  go test ./tests -run '^$' -bench '^Benchmark(ArrayElementAssignment|NativeProcessStartup)' -benchmem -count=5
} 2>&1 | tee "$output"

python3 - "$output" <<'PY' | tee -a "$output"
from collections import defaultdict
import re
import statistics
import sys

report_path = sys.argv[1]
benchmark_line = re.compile(
    r"^(Benchmark\S+)\s+\d+\s+([0-9]+(?:\.[0-9]+)?)\s+ns/op(?:\s+(.*))?$"
)
custom_metric = re.compile(r"([0-9]+(?:\.[0-9]+)?)\s+(ns/assignment|ns/process)\b")
samples = defaultdict(list)

with open(report_path, encoding="utf-8") as report:
    for line in report:
        match = benchmark_line.match(line.strip())
        if not match:
            continue
        benchmark, ns_per_op, trailing = match.groups()
        samples[(benchmark, "ns/op")].append(float(ns_per_op))
        for value, unit in custom_metric.findall(trailing or ""):
            samples[(benchmark, unit)].append(float(value))

print("\n# Repeated-run summary")
if not samples:
    print("No benchmark result lines were found; inspect the raw output above.")
    raise SystemExit(1)

for (benchmark, unit), values in sorted(samples.items()):
    median = statistics.median(values)
    mad = statistics.median(abs(value - median) for value in values)
    spread = (max(values) - min(values)) / median * 100 if median else 0.0
    print(
        f"{benchmark} [{unit}]: samples={len(values)}, "
        f"median={median:.3f}, min={min(values):.3f}, max={max(values):.3f}, "
        f"MAD={mad:.3f}, range/median={spread:.2f}%"
    )

print(
    "Spread statistics describe these repetitions only; they are not a confidence interval. "
    "Native ns/op and ns/assignment include child-process startup."
)
PY

printf 'Report saved to: %s\n' "$output"
