# ADR 0006: Nominal data types and pattern-matching rollout

- **Status:** Proposed
- **Date:** 2026-10-20
- **Relationship:** Builds on [ADR 0003: Array value semantics](0003-array-value-semantics.md) and [ADR 0005: Native resource guarantees and enforcement boundaries](0005-native-resource-guarantees.md). This proposal does not claim the features below are implemented.

## Context

Aster currently supports primitive types and `Array<T>`. Adding structs, enums, generics, `Option<T>`, and `Result<T, E>` at the same time would combine several independent changes: name resolution, recursive type validation, aggregate representation, bytecode verification, pattern exhaustiveness, and generic instantiation. The VM and Native backends share a typed IR, so new semantics should be resolved by the checker and carried through that IR rather than re-inferred by either backend.

Arrays already have explicit value semantics. Any aggregate that contains an array must preserve those semantics across bindings, function arguments and returns, field reads, and aggregate construction. A shallow Go-struct copy must not become an observable aliasing loophole.

## Proposed decision

Implement these features as separate, reviewable vertical slices, in this order:

1. **Non-generic structs** — nominal declarations, field type validation, construction, read-only field selection, value passing, and VM/Native parity.
2. **Enums with payloads and exhaustive `match` expressions** — named variants, payload construction/deconstruction, arm-local bindings, result-type agreement, unreachable/duplicate-arm checks, and exhaustiveness checks.
3. **User-defined generics** — generic type and function parameters, type-argument substitution, constraints/instantiation policy, recursion/resource limits, and bytecode metadata.
4. **Core `Option<T>` and `Result<T, E>`** — model these as ordinary generic enums using the enum and generic machinery, with documented standard variants and pattern matching.

Each slice must have its own syntax/semantics tests, negative type-checking tests, bytecode malformed-input/verifier tests, VM/Native differential tests, and release documentation. Do not mark a feature complete merely because the parser accepts its syntax.

### Struct source model (proposed)

Use nominal, module-local type names. A struct declaration lists fields in a stable order:

```aster
struct Point {
    x: i64,
    y: i64,
}

let p = Point(3, 4);
print(p.x);
print(p.y);
```

- `Point(3, 4)` is the struct constructor. Constructor arguments are positional and checked against field count and declared field types in declaration order.
- `p.x` selects a declared field. The checker resolves the field and its type; code generation must consume this resolution from the typed IR.
- Struct names and field names are case-sensitive. Duplicate type declarations and duplicate field names are rejected. Built-in type and reserved function/builtin names cannot be reused as struct types.
- Types may be referred to before their declaration in the same source file, matching the current function forward-reference model. Multiple source files and imports remain out of scope.
- Struct values are copied by value. Fields are read-only in the first slice; a whole mutable binding can be reassigned, but direct field assignment is deferred until aggregate mutation can preserve value semantics for nested arrays.
- Arrays stored in fields retain the value semantics of ADR 0003. Reading an array field into another independently mutable value must establish the required COW sharing boundary, and arrays nested inside structs must not become mutable aliases across copies.
- Initial struct support does not introduce default constructors, implicit field defaults, reflection APIs, serialization guarantees, or automatic structural equality. Unsupported operations must receive a checker diagnostic rather than falling through to a backend runtime error.
- Recursive layouts must be handled deliberately. Direct by-value recursive cycles cannot be emitted as ordinary Go structs; the implementation slice must either reject such cycles with a type diagnostic or define a bounded representation before enabling them. Recursion guarded by `Array<T>` may be allowed only after the type-layout and COW tests prove it safe.
- Printed struct representation must be specified and tested in both backends before structs are accepted as arguments to `print`; it must not leak Go's internal field names or COW metadata.

Using a call-shaped constructor avoids making `Type { ... }` ambiguous with the existing `if condition { ... }` block syntax. The syntax can be reconsidered if a future parser design adds an unambiguous record-literal form.

### Enum and match source model (proposed)

Enums are closed nominal sum types. Variants may carry zero or more typed positional payloads:

```aster
enum Message {
    Quit,
    Move(i64, i64),
    Write(string),
}
```

Use `Message::Quit`, `Message::Move(10, 20)`, and `Message::Write("hello")` for variant construction. Enums should be introduced together with `match`; exposing tags without a safe way to inspect payloads would leave the feature incomplete.

The proposed `match` is an expression. Every arm produces a value, every arm must resolve to the same type, and a non-exhaustive match is a type-checking error. Initial patterns should support a variant with positional binders and a wildcard. Duplicate variants, unreachable arms after a wildcard, payload-arity mismatches, and repeated pattern bindings are rejected. The exact punctuation and grammar of match arms must be fixed in the implementation PR before code generation starts.

No implicit fallthrough, exception-like control transfer, or unchecked tag cast is introduced. Generic enums and standard `Option`/`Result` declarations wait until generic substitution and recursive type validation exist.

## Bytecode and backend requirements

- Add explicit aggregate type/field metadata to the normalized typed IR and bytecode program. The bytecode verifier must validate declared types, field indexes, constructor arity and operand types, enum tags/payloads, and match control flow before execution.
- Because aggregate type metadata and operations change the bytecode contract, use an explicit format-version change rather than silently extending the current version 5 schema. Preserve the existing rejection of unsupported versions.
- Runtime values must not rely on source text or dynamic type guessing to find fields. The verifier and both backends must use the resolved type and field/variant identity.
- Native code generation must use collision-free generated Go identifiers; source identifiers must never be emitted as trusted raw Go fragments.
- Keep COW metadata out of the serialized representation. Add array-sharing boundary tests for struct construction, binding/rebinding, field extraction, function arguments/returns, and arrays nested within arrays/structs.
- Type nesting, aggregate nesting, verifier state, and generated source size need explicit limits consistent with ADR 0005. Aggregate values are not a process-wide memory bound or a sandbox.

## Rollout and acceptance criteria

1. Publish the implemented-subset language specification before merging each feature.
2. Add parser acceptance/rejection tests and checker tests for invalid declarations, unknown fields, duplicate declarations, type mismatches, and recursive layouts.
3. Add malformed bytecode tests before enabling VM execution of aggregate instructions.
4. Run the same semantic fixtures on VM and Native. Compare output, failure class, exit status, and source-positioned diagnostics.
5. Benchmark representative construction, field reads, nested arrays, and function passing after correctness gates pass. Benchmarks inform optimization; they do not relax value semantics.
6. Update the roadmap only for the exact slice that landed. Generics and `Option`/`Result` remain unchecked until their own implementation and test requirements are met.

## Open questions before acceptance

- Whether recursive struct layouts through `Array<T>` are allowed in the first release or deferred.
- Exact `match` arm grammar, pattern binding scope, and nested-pattern support.
- Whether aggregate equality will be explicit, derived structurally, or unavailable in the initial releases.
- The exact stable bytecode representation for nominal types and variants, including format versioning.
- Whether direct struct-field mutation should be a later feature or part of the initial struct slice once aggregate COW paths are available.
