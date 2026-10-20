# Native array copy-on-write benchmark comparison

Date: 2026-10-20 (JST)

## Scope and provenance

- Workflow run: [39383027771](https://github.com/musashimusashino666/aster/actions/runs/39383027771)
- Pre-Native-COW baseline: `fde20109e77314a31d54a9b4fdd47bb3f3710c9e`
- Native COW candidate tested: `1e1b52bd6ea6c02b5ad2cc59052c733df9227ae0`
- Runner: GitHub-hosted `ubuntu-24.04`, image release `ubuntu24/20261012.334`
- Kernel: `6.17.0-1022-azure`
- CPU: AMD EPYC 7763 64-Core Processor (same runner instance for both revisions)
- Go: `go1.27.2 linux/amd64`
- Command for both revisions: `go test ./tests -run '^$' -bench '^Benchmark(ArrayElementAssignment|NativeProcessStartup)' -benchmem -count=5`
- Method: baseline then candidate, sequentially in one job. Each statistic summarizes five Go benchmark repetitions. `MAD` is median absolute deviation; `range/median` is (max - min) / median.
- Native per-assignment timing includes child-process startup. The startup benchmark is a reference, not a value to subtract mechanically.

## Native array element assignment

Unit: ns/assignment. Percentage change is `(COW median / baseline median - 1) × 100%`; negative means faster.

| Array length | Baseline median (min–max) | Baseline MAD | Baseline range/median | Native COW median (min–max) | COW MAD | COW range/median | Change | Baseline / COW |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 16 | 183.1 (181.6–185.5) | 0.2 | 2.13% | 106.4 (105.9–108.8) | 0.4 | 2.73% | -41.89% | 1.72× |
| 64 | 246.1 (243.6–249.0) | 1.4 | 2.19% | 105.7 (105.2–106.7) | 0.3 | 1.42% | -57.05% | 2.33× |
| 256 | 508.0 (499.6–510.5) | 2.5 | 2.15% | 95.31 (94.95–96.79) | 0.36 | 1.93% | -81.24% | 5.33× |
| 1024 | 1,761.0 (1,758.0–1,784.0) | 2.0 | 1.48% | 155.5 (155.1–155.9) | 0.3 | 0.51% | -91.17% | 11.32× |

## Control measurements

### Native process startup

Unit: ns/process.

| Run | Median (min–max) | MAD | Range/median |
|---|---:|---:|---:|
| Baseline | 1,205,388 (1,198,338–1,221,602) | 6,099 | 1.93% |
| Native COW candidate | 1,210,304 (1,194,308–1,215,496) | 5,192 | 1.75% |

The startup-control median increased by approximately 0.41%, much smaller than the observed Native assignment changes.

### VM control

The Native-only code-generation change should not affect the VM. VM medians were similar between the sequential runs:

| Array length | Baseline VM median (ns/assignment) | Candidate-run VM median (ns/assignment) | Change |
|---:|---:|---:|---:|
| 16 | 476.7 | 477.7 | +0.21% |
| 64 | 454.1 | 455.5 | +0.31% |
| 256 | 445.8 | 453.5 | +1.73% |
| 1024 | 498.4 | 500.2 | +0.36% |

The baseline VM length-256 spread was relatively high (7.72% range/median); its small median change should not be attributed to Native COW.

## Interpretation

- Native COW was faster at all four measured array lengths. The observed median reductions ranged from 41.89% to 91.17%.
- The relative improvement grows with array length, consistent with avoiding repeated full-container copies during indexed assignment.
- The process-startup control changed by only +0.41%, and VM control medians changed by +0.21% to +1.73%. These controls do not explain the Native assignment improvements.
- Spread statistics describe five repetitions only; they are not confidence intervals. The benchmark includes one measured workload family and is not a guarantee about every Aster program.
- This report uses the typed-dispatch implementation in candidate commit `1e1b52bd6ea6c02b5ad2cc59052c733df9227ae0`. An earlier intermediate implementation used reflective method dispatch and showed regressions for small arrays; it was not retained.

## Decision

The typed Native COW implementation is supported by this same-runner measurement: all tested array sizes improved, and no material change appeared in the VM control. Proceed with the Native implementation as a separate PR, gated on the current-head CI and the existing cross-backend value-semantics suite. Keep Native and VM representations separate; this change does not alter bytecode JSON or its version.
