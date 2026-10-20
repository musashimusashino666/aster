# ADR 0003: Array value semantics

- **Status:** Accepted
- **Date:** 2026-10-20

## Context

Aster currently represents arrays as dynamically sized arrays in both execution backends. The VM's `bytecode.Value` and generated Go code use slices, whose underlying storage may be shared when values are copied. Array-element assignment and slicing are not supported in Aster 0.1, so that sharing is not currently observable through element mutation.

Before mutable indexing or slicing is introduced, the language must define whether aliases observe each other's writes. Leaving this to backend implementation details would make the same Aster program behave differently on the VM and native backends, and would constrain later ownership and optimization work.

## Decision

`Array<T>` is a **value type**, not a reference type. Operations that copy an array value have the observable semantics of a recursive value copy:

- Initializing a binding from an existing array, rebinding a mutable variable, passing an array as a function argument, and returning an array preserve value semantics.
- Nested arrays are copied recursively in the semantic model. Mutating an element through one value must not change another value that was copied from it, including when the mutation targets a nested array.
- A future slice/range operation produces an independent array value, not a mutable view into the original array.
- Implementations may use deep copies, copy-on-write, persistent data structures, or another representation, provided all observable behavior matches these rules.
- The language does not expose array identity or pointer/reference operations that would allow programs to distinguish these implementation strategies.

This decision defines the contract for future mutable array operations. It does not add array-element assignment or slicing to Aster 0.1, and it does not require changing the current behavior of programs that cannot mutate array elements.

## Consequences

- VM and native backends must share tests for array copying, function argument and return boundaries, rebinding, nested arrays, and future slice behavior.
- A straightforward implementation may incur allocation or copying costs. Copy-on-write or persistent representations may be evaluated later, but only as semantics-preserving optimizations backed by benchmarks.
- Element mutation must update the value being indexed and must not leak through copied bindings or function boundaries.
- If a future feature requires reference semantics or mutable views, it needs a separate explicit language-design decision rather than an implementation-specific exception.
