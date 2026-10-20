# ADR 0003: Array value semantics

- **Status:** Accepted
- **Date:** 2026-10-20

## Context

Aster represents arrays as dynamically sized arrays in both execution backends. The VM's `bytecode.Value` and generated Go code use slices, whose underlying storage may be shared when values are copied. Mutable indexed assignment and half-open array slicing are implemented; both operations must prevent writes from becoming observable through another copied array value.

Before mutable indexing or slicing is introduced, the language must define whether aliases observe each other's writes. Leaving this to backend implementation details would make the same Aster program behave differently on the VM and native backends, and would constrain later ownership and optimization work.

## Decision

`Array<T>` is a **value type**, not a reference type. Operations that copy an array value have the observable semantics of a recursive value copy:

- Initializing a binding from an existing array, rebinding a mutable variable, passing an array as a function argument, and returning an array preserve value semantics.
- Nested arrays are copied recursively in the semantic model. Mutating an element through one value must not change another value that was copied from it, including when the mutation targets a nested array.
- A slice/range operation produces an independent array value, not a mutable view into the original array. The current syntax is `values[start:end]`, with inclusive start and exclusive end.
- Implementations may use deep copies, copy-on-write, persistent data structures, or another representation, provided all observable behavior matches these rules.
- The language does not expose array identity or pointer/reference operations that would allow programs to distinguish these implementation strategies.

This decision governs both implemented features. Indexed assignment copies each array container along the path before updating it. Slicing allocates and copies a fresh array for the selected range, including an independent empty array when the range length is zero. Nested elements may share internal representation, but later mutations still copy their own indexed path and remain unobservable through the original value.

## Consequences

- VM and native backends must share tests for array copying, function argument and return boundaries, rebinding, nested arrays, and future slice behavior.
- The current VM and native implementations copy every element of each array container along the indexed path before updating it. For a flat array of length n, performing n element assignments therefore copies O(n²) elements in total; a nested update copies the containers along its path. This is an implementation cost, not a relaxation of value semantics. The VM instruction budget does not account for the number of elements copied, so repeated assignments can consume substantial CPU time and memory bandwidth. `tests/array_assignment_benchmark_test.go` measures VM and native execution at multiple sizes using 256 rounds of indexed assignments per benchmark operation and reports time per assignment. Native compilation is excluded from the timed section; each measured operation still launches a process, but the repeated workload reduces the startup-time share. Benchmark allocation counts for the native executable are not captured by Go's parent-process `-benchmem` metrics. Copy-on-write or persistent representations may be evaluated later, but only as semantics-preserving optimizations backed by measured results.
- Element mutation must update the value being indexed and must not leak through copied bindings or function boundaries.
- If a future feature requires reference semantics or mutable views, it needs a separate explicit language-design decision rather than an implementation-specific exception.
