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

These are execution-focused microbenchmarks, not end-to-end compile measurements. Use the same Go version, commit, benchmark flags, and host when comparing revisions. Record OS/kernel, CPU, Go version, commit, compiler flags, warm/cold cache conditions, repetitions, median, and dispersion. Do not compare single-run results or interpret a microbenchmark as a general language-performance claim.

## Native comparison

Build an executable before timing repeated runs:

```bash
mkdir -p ./bin
go build -o ./bin/aster ./cmd/aster
./bin/aster build -o ./bench/data_processing examples/data_processing.ast
```

Use `hyperfine` or another controlled harness to compare repeated executions of that native binary with the VM workload. Make sure both executions perform equivalent work and produce equivalent output. Measure native build time separately from executable runtime. Record the same host/compiler metadata and report median and dispersion. Do not compare Aster's VM execution with another language's optimized native executable and call the result a language-level performance comparison.

## Further workloads

Parsing/type-checking time over a fixed source corpus, native build time and executable size, and process peak resident memory still need dedicated harnesses. AI/GPU, WebAssembly, game-loop latency, mobile and embedded benchmarks are deferred until those backends exist.
