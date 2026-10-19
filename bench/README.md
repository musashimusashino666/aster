# Benchmarking Aster

No performance claim is made from the current prototype. Benchmark numbers need to be measured on a documented host and compared with equivalent workloads and optimization settings.

## In-process VM baselines

Run:

```bash
go test ./internal/vm -run '^$' -bench '^BenchmarkVM' -benchmem -count=5
```

The current Go benchmarks cover a numeric loop and recursive Fibonacci after one-time parse, type-check, and bytecode compilation. Their measurements are execution-only for the VM, not end-to-end compile times.

## Native comparison

Build an executable first, then measure repeated runs separately:

```bash
mkdir -p ./bin
go build -o ./bin/aster ./cmd/aster
./bin/aster build -o ./bench/loop examples/data_processing.ast
```

Use a tool such as `hyperfine` or a carefully controlled shell harness. Record OS/kernel, CPU, Go version, commit, compiler flags, warm/cold cache conditions, number of repetitions, median and dispersion. Compare identical outputs and check error behavior as well as elapsed time. Do not compare Aster's VM execution with another language's optimized native executable and call the result a language-level performance comparison.

## Planned workloads

- Integer arithmetic and branch-heavy loops
- Function calls and recursion
- Array indexing and traversal
- String operations and Unicode length
- Parsing/type-checking time on a fixed source corpus
- Native build time and executable size
- Memory allocation and peak resident memory

AI/GPU, WebAssembly, game-loop latency, mobile and embedded benchmarks are deferred until those backends exist.
