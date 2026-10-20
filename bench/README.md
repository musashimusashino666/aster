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

The runtime currently writes each `print` directly with `fmt.Fprintln`; it does not aggregate output through a shared buffered writer. `BenchmarkVMFormattedArrayPrint` intentionally uses `io.Discard`, so it measures formatting and execution rather than stdout system-call throughput. Do not extrapolate this benchmark to print-heavy workloads; output-heavy comparisons need an explicit, equivalent output destination and should report I/O costs separately.\n\nThese are execution-focused microbenchmarks, not end-to-end compile measurements. Use the same Go version, commit, benchmark flags, and host when comparing revisions. Record OS/kernel, CPU, Go version, commit, compiler flags, warm/cold cache conditions, repetitions, median, and dispersion. Do not compare single-run results or interpret a microbenchmark as a general language-performance claim.

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

## Further workloads

Parsing/type-checking time over a fixed source corpus, native build time and executable size, and process peak resident memory still need dedicated harnesses. AI/GPU, WebAssembly, game-loop latency, mobile and embedded benchmarks are deferred until those backends exist.
