# Native nested-array assignment COW benchmark

Date: 2026-10-20 (JST)

## Scope and provenance

- Workflow run: [39385676216](https://github.com/musashimusashino666/aster/actions/runs/39385676216)
- Runner: GitHub-hosted `ubuntu-24.04`
- CPU: AMD EPYC 9V45 96-Core Processor
- Go: `go1.27.2 linux/amd64`
- Command: `go test ./tests -run '^$' -bench '^BenchmarkNestedArrayElementAssignmentNative$' -benchmem -count=5`
- Baseline revision: `e5b99db56e64f07e61ef4c3d583d4db23ee2cf27` (value-copy-boundary handling added; temporary index reads still marked array children shared)
- Corrected revision: `d813271ffb9b2f495d3e32db7c2a23743f188ee6` (temporary reads no longer mark child arrays shared)
- The workflow checked out both revisions and measured them sequentially on the same runner instance. Each benchmark result is the median of five repetitions.
- Workload initializes a matrix containing one row, then repeatedly updates every element using `matrix[0][i] = matrix[0][i] + 1`; the loop condition also reads `len(matrix[0])`. It therefore exercises temporary indexed reads during repeated nested writes.
- Unit: ns/element assignment. Each timed Native benchmark operation launches a child process, so the per-assignment metric includes process startup. Do not interpret the smaller-array numbers as pure runtime operation cost.

## Results

Percentage change is `(corrected median / baseline median - 1) × 100%`; negative means faster.

| Row length | Baseline median (min–max) | Baseline MAD | Baseline range/median | Corrected median (min–max) | Corrected MAD | Corrected range/median | Change | Baseline / corrected |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 16 | 14,055 (13,818–14,395) | 215 | 4.11% | 13,649 (13,218–14,070) | 233 | 6.24% | -2.89% | 1.03× |
| 64 | 3,831 (3,747–4,021) | 82 | 7.15% | 3,449 (3,400–3,464) | 15 | 1.86% | -9.97% | 1.11× |
| 256 | 1,652 (1,605–1,657) | 5 | 3.15% | 930.1 (919.2–945.7) | 10.1 | 2.85% | -43.70% | 1.78× |
| 1024 | 3,362 (3,304–3,498) | 58 | 5.77% | 296.9 (293.8–304.1) | 3.1 | 3.47% | -91.17% | 11.32× |

MAD is the median absolute deviation from the sample median. Range/median is `(max - min) / median`. With five repetitions, these spread statistics are descriptive and are not confidence intervals.

## Interpretation

- Removing the shared mark from `asterIndexI64` reduces repeated Native nested-write cost as row length grows. The candidate revision differs from the baseline for this comparison by removing the mark from the indexed-read helper; the value-copy-boundary changes are present in both revisions.
- The 16-element result is close to the process-startup floor and should not be treated as evidence of a 2.89% runtime-only gain.
- At lengths 256 and 1024, the improvements are large enough to be consistent with eliminating repeated O(row length) detach/copy operations.
- The correctness counterpart, `TestIndexedArrayReadsShareOnlyAtValueCopyBoundaries`, covers array-valued indexed reads stored into a local, assigned back into a local, and subsequent nested writes across VM and Native.
- This is a targeted microbenchmark for the identified access pattern, not a general program-level performance guarantee.
