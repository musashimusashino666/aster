# Benchmarking Aster

No performance claim is made from the current prototype. Benchmark numbers depend on the host and Go toolchain; compare repeated measurements on the same machine before considering representation changes.

## In-process VM baselines

Run the fixed-workload benchmarks from the repository root:

```bash
go version
git rev-parse HEAD
go test ./internal/vm -run '^$' -bench '^BenchmarkVM' -benchmem -count=5
```

The benchmarks parse, type-check, and compile their Aster source once before timing the VM execution loop. They report allocations as well as elapsed time. Current workloads are:

- `BenchmarkVMNumericLoop`: integer arithmetic and branch-heavy loop.
- `BenchmarkVMRecursiveFibonacci`: recursive function calls and integer arithmetic.
- `BenchmarkVMFunctionCallLoop`: repeated calls with one `i64` argument and result.
- `BenchmarkVMArrayTraversal`: reads and sums 256 array elements using indexed access.
- `BenchmarkVMStringConcatAndUnicodeLen`: string concatenation and Unicode code-point counting.
- `BenchmarkVMFormattedArrayPrint`: construction and formatting of a 128-element array, with the writer set to `io.Discard` so terminal throughput does not dominate.

The runtime currently writes each `print` directly with `fmt.Fprintln`; it does not aggregate output through a shared buffered writer. `BenchmarkVMFormattedArrayPrint` intentionally uses `io.Discard`, so it measures formatting and execution rather than stdout system-call throughput. Do not extrapolate this benchmark to print-heavy workloads; output-heavy comparisons need an explicit, equivalent output destination and should report I/O costs separately.

These are execution-focused microbenchmarks, not end-to-end compile measurements. Use the same Go version, commit, benchmark flags, and host when comparing revisions. Record OS/kernel, CPU, Go version, commit, compiler flags, warm/cold cache conditions, repetitions, median, and dispersion. Do not compare single-run results or interpret a microbenchmark as a general language-performance claim.

## Native comparison

Build an executable before timing repeated runs:

```bash
mkdir -p ./bin
go build -o ./bin/aster ./cmd/aster
./bin/aster build -o ./bench/data_processing examples/data_processing.ast
```

Use `hyperfine` or another controlled harness to compare repeated executions of that native binary with the VM workload. Make sure both executions perform equivalent work and produce equivalent output. Measure native build time separately from executable runtime. Record the same host/compiler metadata and report median and dispersion. Do not compare Aster's VM execution with another language's optimized native executable and call the result a language-performance comparison.

## Array element assignment

Run from a Git checkout with Go and Python 3 available. The repository script captures the array benchmark together with its environment metadata and repeated-run summary:

```bash
bash scripts/bench-array-assignment.sh
# Or choose an output path:
bash scripts/bench-array-assignment.sh bench/array-assignment-baseline.out
```

By default, the script writes an ignored `bench/array-assignment-<UTC timestamp>.out` report. It records the commit, working-tree cleanliness, OS/kernel, CPU details when available, Go version, relevant Go environment, the exact command, raw benchmark output, and summary statistics. It runs `go test ./tests -run '^$' -bench '^Benchmark(ArrayElementAssignment|NativeProcessStartup)' -benchmem -count=5`.

The array workload covers lengths 16, 64, 256, and 1024. The VM and Native benchmark variants use the same generated Aster source for each size and perform the same number of element assignments per workload. Workload rounds vary by size to keep the benchmark bounded while reducing the share of launch overhead for larger arrays. The custom `ns/assignment` metric divides the elapsed benchmark time by the total number of assignments.

`BenchmarkNativeProcessStartup` measures repeated launches of a no-op executable built with the host Go toolchain. Treat it as an approximate process-launch reference, not as an exact startup cost to subtract from the array results: generated binary size and runtime initialization differ. The Native array benchmark still starts a new process for every benchmark operation, so its `ns/op` and `ns/assignment` include process startup and execution. The separate reference helps assess how material startup overhead may be, but it does not eliminate that overhead.

The summary reports the median, minimum, maximum, median absolute deviation (MAD), and range relative to the median across the five Go benchmark repetitions. These describe observed spread, not a confidence interval. Repeat runs on the same otherwise-idle machine and compare summaries; do not turn noisy microbenchmark timings into a CI pass/fail threshold.

The `-benchmem` allocation figures for Native are allocations made by the Go test process (including process-launch harness work). They do **not** measure allocations inside the child Native executable and must not be used as a VM-versus-Native child-memory comparison. If child-process allocation or peak-memory data is needed, add a separate process-aware measurement rather than inferring it from `-benchmem`.

## Native child-process peak memory

To measure the generated Native executable itself, run the Linux-only process-aware harness:

```bash
bash scripts/bench-native-memory.sh
# Reduce to one repetition for a functional smoke test:
ASTER_BENCH_REPEATS=1 bash scripts/bench-native-memory.sh bench/native-memory-smoke.out
# Or choose an output path:
bash scripts/bench-native-memory.sh bench/native-memory-baseline.out
```

The script requires Linux, GNU `/usr/bin/time`, Go, Python 3, and Git. It records OS/kernel, CPU, logical CPU count, Go version, source commit, each raw sample, and a summary (median, min/max, MAD, and range relative to median). It builds a no-op executable and workloads retaining one, four, or eight independent 16 MiB ASCII strings, corresponding to 0, 16, 64, and 128 MiB of retained string payload. Native build elapsed time and binary size are reported separately from runtime measurements.

Peak RSS is reported in KiB from GNU `time`'s `%M` metric. It includes Go runtime costs and mapped executable pages; it is not Aster heap size, total allocated bytes, or a process-wide memory quota. The workloads are bounded and do not attempt to trigger OOM. Runtime measurements are collected in fresh standalone child processes without a resource supervisor, so they do not measure isolation overhead. No CI performance threshold is inferred from these results. Repeat on an otherwise idle host, retain the raw report, and compare only measurements with compatible host/toolchain metadata.

This harness is the measurement prerequisite for [Issue #82](https://github.com/musashimusashino666/aster/issues/82) and the proposed [ADR 0010](../docs/adr/0010-process-resource-isolation.md). It must not be used to pick default enforcement thresholds from one runner or to claim a hard memory guarantee.

## Further workloads

Parsing/type-checking time over a fixed source corpus, native build time and executable size, and process peak resident memory still need dedicated harnesses. AI/GPU, WebAssembly, game-loop latency, mobile and embedded benchmarks are deferred until those backends exist.


## Native resource-isolation overhead and CPU-time overshoot

The Linux-only harness for [Issue #93](https://github.com/musashimusashino666/aster/issues/93) compares the same Native Aster workloads under the ordinary CLI path and the opt-in constrained profile. It requires Go, Python 3, Git, Linux, and a genuinely delegated writable cgroup v2 hierarchy with both the cpu and memory controllers available and enabled for children. The script does not modify the host cgroup hierarchy or enable controllers automatically.

Set the delegated root and explicitly classify the host before running:

```bash
ASTER_CGROUP_ROOT=/absolute/path/to/delegated/root \
ASTER_BENCH_HOST_KIND=bare-metal \
python3 scripts/bench-resource-isolation.py
```

Use `bare-metal` only on a directly booted non-WSL bare-metal Linux host. Accepted labels also include `virtual-machine`, `wsl2`, `container`, and `unknown`; each result records that label so WSL2 or VM evidence cannot be mistaken for bare-metal evidence. The default is 30 repeated comparisons for each workload/profile and 30 CPU-time termination samples, plus five independent build-time samples per workload. Output is a timestamped Markdown report and a companion TSV of raw samples under `bench/results/`. An optional positional argument selects the Markdown report path; the TSV uses the same basename.

Workloads cover a launch-heavy no-op, an integer CPU loop, two retained 16 MiB strings below the default 256 MiB limit, a CPU-bound infinite loop terminated by the default 50 ms cumulative CPU-time budget, and a bounded-memory-limit OOM check that retains five 16 MiB strings under a default 64 MiB limit. The OOM case runs three times by default, must be classified as `RUN018`, and verifies that each newly created Aster cgroup is removed after the command exits. Baseline and constrained executions use identical source and alternating measurement order. The report records end-to-end CLI wall time, independently measured Native build time and executable size, sampled process-tree RSS, cgroup `memory.peak` when observed, exit classification, cgroup cleanup status, and post-termination CPU budget/usage/overshoot. Override defaults with `ASTER_BENCH_REPEATS`, `ASTER_BENCH_BUILD_REPEATS`, `ASTER_BENCH_OOM_REPEATS`, `ASTER_BENCH_MEMORY_LIMIT`, `ASTER_BENCH_OOM_MEMORY_LIMIT`, `ASTER_BENCH_CPU_RATE`, and `ASTER_BENCH_CPU_TIME_LIMIT`.

The end-to-end CLI time includes source analysis, Native code generation, the Go build, and execution. The independent build samples are not subtracted from that time: the current CLI does not expose a runtime-only constrained invocation or supervisor setup/cleanup duration, so those quantities are marked unavailable instead of inferred from the full latency delta. Process-tree RSS is sampled from `/proc`, can miss short-lived peaks, and can double-count shared pages; it is not the same accounting boundary as cgroup `memory.peak`.

Run on an otherwise idle machine and preserve both output files. Report median, nearest-rank p95, and maximum observed values, alongside the raw rows and exact environment metadata. The largest observed CPU-time overshoot is only the maximum of this finite sample set; it is not a kernel-independent or workload-independent upper bound. A successful WSL2 run does not satisfy the bare-metal requirement in [Issue #92](https://github.com/musashimusashino666/aster/issues/92). This harness does not establish a sandbox or a fixed performance target.
