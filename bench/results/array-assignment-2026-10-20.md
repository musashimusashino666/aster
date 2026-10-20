# Array element assignment benchmark baseline — 2026-10-20

## Purpose

Record the first measurement using the reproducible runner, four array sizes, and the no-op Native process-startup reference introduced in the benchmark-harness change. These values are local baselines for this host and toolchain, not cross-machine performance claims.

## Run metadata

- Measurement time: 2026-10-20 05:51:23 UTC (14:51:23 JST)
- Main source baseline: `599ccc8534a9ae698f9b7ebf31bc038e21e9a5e1`
- Exact runner checkout: `cf078d86057f4a0c3725f842fc1a7f4c3e91b2d1` (the same Aster benchmark/test source as the main baseline, plus a temporary, one-off measurement workflow)
- Workflow: [One-off array assignment benchmark run #39377104315](https://github.com/musashimusashino666/aster/actions/runs/39377104315)
- Result: PASS; `go test` completed in 75.770 seconds.
- Command: `go test ./tests -run '^$' -bench '^Benchmark(ArrayElementAssignment|NativeProcessStartup)' -benchmem -count=5`

Environment:

- Ubuntu 24.04.5 LTS; Linux kernel `6.17.0-1022-azure`; `linux/amd64`
- Go `go1.27.2 linux/amd64`; `GOAMD64=v1`; `CGO_ENABLED=1`; `GOTOOLCHAIN=local`; `GOFLAGS=`; `GOMAXPROCS=default`
- CPU exposed to the runner: 4 logical CPUs; AMD EPYC 7763 64-Core Processor (2 cores/socket, 2 threads/core)
- Working tree was clean at benchmark start.

## Results

Each reported median and dispersion is calculated from the five Go benchmark repetitions. `MAD` is the median absolute deviation; `range/median` is `(max - min) / median × 100%`. The range describes these five observations and is not a confidence interval.

| Backend | Array length | Rounds | Assignments per benchmark operation | Median (ns/assignment) | Min–max (ns/assignment) | MAD | Range/median |
|---|---:|---:|---:|---:|---:|---:|---:|
| VM | 16 | 2048 | 32,768 | 675.9 | 669.1–696.7 | 6.8 | 4.08% |
| VM | 64 | 512 | 32,768 | 1,383.0 | 1,373.0–1,455.0 | 8.0 | 5.93% |
| VM | 256 | 256 | 65,536 | 4,344.0 | 4,266.0–4,475.0 | 14.0 | 4.81% |
| VM | 1024 | 16 | 16,384 | 21,120.0 | 20,695.0–21,503.0 | 361.0 | 3.83% |
| Native | 16 | 2048 | 32,768 | 176.5 | 175.1–181.2 | 0.9 | 3.46% |
| Native | 64 | 512 | 32,768 | 234.5 | 232.8–237.2 | 0.7 | 1.88% |
| Native | 256 | 256 | 65,536 | 486.9 | 482.5–495.8 | 3.6 | 2.73% |
| Native | 1024 | 16 | 16,384 | 1,703.0 | 1,699.0–1,736.0 | 4.0 | 2.20% |

Median end-to-end benchmark time per operation:

| Backend | n=16 | n=64 | n=256 | n=1024 |
|---|---:|---:|---:|---:|
| VM | 22.148 ms | 45.314 ms | 284.673 ms | 346.027 ms |
| Native | 5.783 ms | 7.686 ms | 31.911 ms | 27.906 ms |

### Native process-startup reference

The no-op executable benchmark measured:

- Median: **1,169,457 ns/process** (1.169 ms)
- Min–max: 1,141,346–1,180,192 ns/process
- MAD: 10,735 ns
- Range/median: 3.32%

As a rough context check only, this reference equals about 20.2%, 15.2%, 3.7%, and 4.2% of the median Native operation time for n=16, 64, 256, and 1024 respectively. These ratios are not a calibrated estimate of the true startup share: the no-op executable differs from the generated benchmark programs in binary size and runtime initialization. **Do not subtract this reference mechanically** from the Native measurements. Startup and program execution remain combined in both Native `ns/op` and `ns/assignment`.

## Interpretation

- Time per assignment increases with array length for both backends under this copy-every-indexed-update implementation; n=1024 is materially more expensive per assignment.
- The n=1024 case completes 16 rounds rather than 256 rounds, keeping total work bounded. Its per-assignment value is still normalized by the exact number of assignments, but its total `ns/op` should not be compared directly with n=256 as if they executed the same amount of work.
- The five-run spread is approximately 1.9%–5.9% across array results. This is useful for tracking repeatability, but too few repetitions to infer a confidence interval or define a CI regression threshold.
- The earlier shared baseline used a different CPU model (AMD EPYC 9V74). Although Go was also 1.27.2 and the OS was Ubuntu 24.04, the CPU/runner allocation differs. Do not interpret differences from that earlier run as clean before/after performance changes.
- Native `-benchmem` values (around 19 KB/op and 41 allocs/op here) measure allocations in the parent Go test process, particularly the process-launch harness. They do **not** measure allocations inside the Native child executable. They are not a VM-vs-Native memory comparison.
- The reported VM allocation counts and bytes are for the in-process VM executing each whole workload; workload rounds differ by size, so compare them only with that normalization in mind.

## Reproduction

From a clean checkout with Go and Python 3 available:

```bash
bash scripts/bench-array-assignment.sh
```

The script saves the host/toolchain metadata, raw five-run output, and the summary in an ignored `bench/array-assignment-<UTC timestamp>.out` file. Use the exact same host, Go version, source commit, and command for future comparisons.
