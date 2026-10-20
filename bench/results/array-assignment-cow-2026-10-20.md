# VM array copy-on-write benchmark comparison

Date: 2026-10-20 (JST)

## Scope and provenance

- Workflow run: [39380381853](https://github.com/musashimusashino666/aster/actions/runs/39380381853)
- Baseline (pre-COW `main`): `990ee8b6b1780afeeddcf9780aff9a82943a7750`
- COW candidate: `c35bfe7950451c8d8ee43a6dcac1dd84b0f144bc`
- Runner: GitHub-hosted `ubuntu-24.04`, Ubuntu 24.04.5, kernel `6.17.0-1022-azure`
- CPU: AMD EPYC 7763 64-Core Processor (4 logical CPUs exposed to the job)
- Go: `go1.27.2 linux/amd64`
- Command: `go test ./tests -run '^$' -bench '^Benchmark(ArrayElementAssignment|NativeProcessStartup)' -benchmem -count=5`
- Method: baseline and candidate were run sequentially in one workflow job on the same runner. Each listed statistic summarizes five samples. `MAD` is median absolute deviation; `range/median` is (max - min) / median.
- Native benchmark timings include child-process startup, as noted by the benchmark harness.

## VM array element assignment

Unit: ns/assignment. Change is (COW median / baseline median - 1).

| Array length | Baseline median (min–max) | Baseline MAD | Baseline range/median | COW median (min–max) | COW MAD | COW range/median | Change | Baseline / COW |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 16 | 682.1 (680.9–684.5) | 1.2 | 0.53% | 475.1 (474.7–480.6) | 0.4 | 1.24% | -30.35% | 1.44× |
| 64 | 1,362.0 (1,344.0–1,416.0) | 9.0 | 5.29% | 454.1 (453.3–454.4) | 0.3 | 0.24% | -66.66% | 3.00× |
| 256 | 4,377.0 (4,279.0–4,429.0) | 18.0 | 3.43% | 451.1 (448.9–463.2) | 0.0 | 3.17% | -89.69% | 9.70× |
| 1024 | 22,325.0 (21,703.0–34,583.0) | 622.0 | 57.69% | 498.3 (496.9–504.2) | 1.4 | 1.46% | -97.77% | 44.80× |

## Native control results

Native is included as a control; the COW change is VM-only and does not intentionally modify Native behavior.

Unit: ns/assignment.

| Array length | Baseline median (min–max) | Baseline MAD | Baseline range/median | COW-run median (min–max) | COW-run MAD | COW-run range/median | Change |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 16 | 180.5 (180.1–182.1) | 0.4 | 1.11% | 176.9 (176.2–179.7) | 0.4 | 1.98% | -1.99% |
| 64 | 243.7 (239.3–248.5) | 1.5 | 3.78% | 236.1 (233.9–239.8) | 1.2 | 2.50% | -3.12% |
| 256 | 492.6 (488.7–494.3) | 1.7 | 1.14% | 490.9 (489.9–499.5) | 1.0 | 1.96% | -0.35% |
| 1024 | 1,721.0 (1,701.0–1,759.0) | 20.0 | 3.37% | 1,735.0 (1,725.0–1,751.0) | 10.0 | 1.50% | +0.81% |

### Native process startup control

Unit: ns/process.

| Run | Median (min–max) | MAD | Range/median |
|---|---:|---:|---:|
| Baseline | 1,196,206 (1,183,116–1,292,604) | 13,090 | 9.15% |
| COW run | 1,212,910 (1,204,296–1,236,377) | 6,937 | 2.64% |

## Interpretation

- On this same-runner comparison, the VM median improved at every tested array length: 30.35% at length 16, 66.66% at 64, 89.69% at 256, and 97.77% at 1024.
- The baseline VM result at length 1024 is noisy: its range/median is 57.69%, driven by a high sample of 34,583 ns/assignment. The COW samples are tightly grouped (1.46% range/median). The direction and size of the observed difference are strong, but the exact percentage should be treated as indicative rather than as a precise general performance guarantee.
- Native control medians varied by -3.12% to +0.81% across the sequential runs, and Native process-startup medians differed by +1.40%. These small control shifts show some run-to-run variation and must not be attributed to the VM COW implementation.
- The spread statistics describe these five repetitions only; they are not confidence intervals. This benchmark covers the measured array-element-assignment workload and does not establish performance for every Aster program.

## Decision

This is positive performance evidence for the VM-only prototype and supports continuing the review. It does not replace the semantic review of sharing boundaries (function calls/returns, nested arrays, element replacement, constants, slices, and bytecode JSON compatibility). Do not merge PR #44 until that review and the current-head CI checks are complete.
