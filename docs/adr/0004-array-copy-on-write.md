# ADR 0004: Copy-on-write array storage

- **Status:** Accepted
- **Date:** 2026-10-20
- **Supersedes:** No existing decision; this refines the implementation strategy allowed by ADR 0003.

## Context

ADR 0003 defines `Array<T>` as a value type: mutation through one value must not become observable through another copied value. The current VM and Native implementations enforce this by copying each array container on the indexed-assignment path before updating it.

That strategy is correct but expensive for repeated writes. On the 2026-10-20 AMD EPYC 7763 baseline, assigning every element in an array of length 1024 measured a median of 21,120 ns/assignment on the VM and 1,703 ns/assignment on Native. A full pass copies O(n²) elements for a flat array of length n. The benchmark also includes Native process startup and must be interpreted with the constraints recorded in `bench/results/array-assignment-2026-10-20.md`.

The next optimization should avoid copying a container when the value being mutated has no other independently mutable owner, while preserving value semantics across both backends.

## Decision

Use **copy-on-write (COW) array storage** for indexed assignment. Implement the VM prototype first, measure it against the existing baseline, then port the same observable semantics to Native. Do not change the language-level value semantics defined by ADR 0003.

### Representation and write behavior

- Represent runtime arrays as handles to private storage containing the element buffer and a shared marker. The Native backend should use a typed wrapper for `Array<T>` rather than exposing raw Go slices as Aster array values.
- When another independently mutable Aster value begins sharing an array's storage, mark that storage as shared. Reads used only as temporary expression operands are not, by themselves, a semantic copy.
- When updating a container whose storage is marked shared, copy that container before writing and make the new storage unshared. When its storage is unshared, update it in place. Apply this rule at each container along a nested indexed-assignment path.
- When a shared container is copied, its array-valued children become shared as well: the old and new containers now both refer to those child values. Nested updates must apply COW recursively so that a write detaches every shared container on the path.
- Mark storage shared at every relevant value-copy boundary, including binding initialization or rebinding from an existing array, function argument passing, array-valued element insertion/replacement, and array values loaded from bytecode constants that remain live in the constant pool. Function returns and other escape boundaries must preserve the same rule whenever another owner can remain live. Slice results must retain the existing independent-value behavior; any array-valued elements copied into a slice must remain protected from cross-value mutation.
- Treat newly constructed arrays as uniquely owned until a real copy boundary is encountered. Do not mark every local read as shared: doing so would force a container copy on each iteration of the benchmark workload and defeat the optimization.
- Do not add reference counting or finalizers in the first implementation. The shared marker is conservative and sticky on existing storage; it is not cleared when other values become unreachable. A subsequent write may therefore make an unnecessary copy after an alias has died, but it must never mutate storage that is still shared. A newly detached storage starts unshared.
- Keep the bytecode JSON representation compatible with the existing `Value`/array encoding. Runtime storage metadata must not leak into serialized bytecode or require a format-version change. Add explicit serialization coverage if the runtime representation requires custom encoding.

### Native prototype notes (2026-10-20)

The VM COW prototype was merged in PR #44. The separate Native prototype represents generated `Array<T>` values with a typed `asterArray[T]` wrapper containing an element slice and a pointer to conservative, sticky shared-state metadata. Indexed mutation detaches shared storage at each array container along the path and updates unique storage in place. Value-copy boundaries in generated code include local aliasing, function arguments and returns; array-valued indexing, nested-array construction, replacement and slices preserve nested sharing.

The Native comparison in [`bench/results/native-array-assignment-cow-2026-10-20.md`](../../bench/results/native-array-assignment-cow-2026-10-20.md) measured reductions at every tested size on the same GitHub Actions runner: 41.89% at length 16, 57.05% at 64, 81.24% at 256 and 91.17% at 1024. These are workload-specific microbenchmark results, not general performance guarantees.

A follow-up review found that the initial Native `asterIndexI64` helper marked every array-valued indexed read as shared, including reads used only as temporary expression operands. This contradicted the representation rule above and could detach a nested row on every write. The correction leaves indexed reads unmarked and has code generation mark array-valued `IndexExpr` results only when they cross a local binding or rebinding boundary. The same-runner before/after measurement is recorded in [`bench/results/native-nested-array-assignment-cow-2026-10-20.md`](../../bench/results/native-nested-array-assignment-cow-2026-10-20.md).

### Slices and language compatibility

The first COW implementation keeps the current slice strategy: `values[start:end]` eagerly creates a fresh outer array and copies the selected range. Do not introduce user-visible views, identity, or changed slice bounds. If a copied element is itself an array, its child storage must be marked shared. Optimizing slice creation with internal views is outside this decision and requires separate measurements and tests.

### Required copy boundaries

Before enabling in-place writes, audit and test all locations that can create another live owner: binding initialization and rebinding; passing arrays to functions and returning arrays; constructing arrays from array-valued elements; replacing nested array elements; slicing nested arrays; loading array constants; and copying array values between VM stack, locals, and call frames. The VM and generated Native code may use different helper functions, but they must implement the same boundary rules.

## Validation and rollout

1. Keep the cross-backend regression test in `tests/array_value_semantics_test.go` as the semantic gate.
2. Implement the VM storage wrapper and COW path first. Run `go test ./...`, `go test -race ./...`, `go vet ./...`, formatting checks, examples, and the existing 32-bit array-index test. Confirm serialization and malformed-bytecode tests still pass.
3. Re-run `scripts/bench-array-assignment.sh` on the same host, Go version, and source environment as the baseline. Record five repetitions, medians, min/max, MAD, range/median, and exact environment metadata. Do not subtract Native startup time mechanically.
4. Implement the Native typed wrapper and mutation helpers. Re-run the full cross-backend suite and all CI checks, then repeat the same benchmark protocol. Record both improvements and regressions by array size; do not introduce a CI performance threshold until repeated same-environment data establishes a defensible noise margin.

The optimization is accepted only if every value-semantics test remains green. Benchmark changes are evidence for tuning, never permission to weaken semantics. If the storage wrapper or sharing-boundary bookkeeping does not produce a measurable same-environment improvement, profile the implementation and revise or revert it before further rollout.

## Consequences

- Repeated writes to an unshared flat array can mutate its existing element buffer, avoiding a full container copy per write. The first write to a shared array pays for one detach; subsequent writes to that detached value can be in place until it is shared again.
- Each alias that is independently mutated may need its own detach. Nested updates may copy only the shared containers along the indexed path.
- The sticky shared marker avoids reference-count maintenance across Go value copies and garbage collection, at the cost of occasional conservative copies after aliases become unreachable.
- Correctness depends on marking every semantic sharing boundary. A missed boundary can silently violate ADR 0003, so backend parity tests are a release gate for this change.
- Runtime storage wrappers add implementation complexity. The VM and Native representations need not be structurally identical, but the observable language behavior and benchmark interpretation must remain aligned.
