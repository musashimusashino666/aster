# ADR 0007: User-defined generics and monomorphization

- **Status:** Accepted
- **Date:** 2026-10-20
- **Relationship:** Refines the third stage in [ADR 0006: Nominal data types and pattern-matching rollout](0006-nominal-data-types-and-pattern-matching.md). This record defines implementation requirements; it does not claim generics are already implemented.

## Context

Aster supports concrete nominal structs and enums plus the built-in `Array<T>`. User-defined generics are needed for reusable nominal types and ordinary `Option<T>` / `Result<T, E>` enums. They affect parsing, substitution, recursive-layout checks, compile-time resource use, typed IR, bytecode verification, and Native Go generation.

The VM and Native backends already consume shared, type-resolved IR. Generic semantics should therefore be resolved once, before either backend emits code, rather than re-inferred independently by the VM compiler and Native code generator.

## Decision

### 1. Use monomorphization

Generic declarations are compile-time templates. After type argument resolution, the compiler structurally substitutes arguments and creates concrete instances. The typed IR handed to Bytecode compilation and Native code generation contains only concrete types, fields, payloads, and function targets. Backends do not perform generic inference or substitution.

An instance is keyed by the stable generic declaration identity and a canonical ordered list of concrete type arguments. Equivalent requests reuse the same instance. Distinct arguments to the same nominal template produce distinct nominal types. This keeps runtime values free of type-argument objects and preserves the existing concrete-type verifier model.

### 2. Syntax and inference contract

The intended source model is:

```aster
fn identity<T>(value: T) -> T {
    return value;
}

struct Box<T> {
    value: T,
}

enum Option<T> {
    None,
    Some(T),
}

enum Result<T, E> {
    Ok(T),
    Err(E),
}

let boxed: Box<i64> = Box(42);
let value: Option<i64> = Option::Some(7);
let empty: Option<i64> = Option::<i64>::None;
let n = identity::<i64>(7);
```

- Functions, structs, and enums may declare zero or more unique type parameters between angle brackets. Declarations without parameters retain existing behavior.
- Type annotations support structural applications such as `Box<i64>`, `Option<string>`, and `Result<i64, string>`, including nested `Array<T>` and nominal applications.
- Generic functions accept explicit arguments with `::<...>`, for example `identity::<i64>(7)`. This avoids ambiguity with the existing less-than comparison operator.
- When explicit arguments are omitted, function inference unifies parameter type patterns with ordinary argument types. It recursively handles patterns such as `Array<T>` and `Box<T>`. It does not infer solely from the expected result type in the first slice. Unresolved parameters produce a diagnostic that recommends explicit arguments.
- Generic struct constructors infer arguments from constructor values when all parameters can be resolved. Explicit arguments may use `Box::<i64>(42)`. Field arity and substituted field types are checked before lowering.
- Generic enum constructors infer arguments from payloads and an available expected type when unambiguous. A payload-free variant requires an expected concrete enum type or explicit arguments, e.g. `Option::<i64>::None`. Match subjects and patterns must resolve to the exact same concrete enum instantiation.
- Type argument counts are exact. Partial application, defaulted type arguments, overloading, and implicit conversions are not supported.

### 3. Initial capabilities and exclusions

- Type parameters may appear in function parameters and returns, local annotations, struct fields, enum payloads, and nested type applications. Substitution recursively visits every occurrence.
- The first slice has no traits or bounds, `where` clauses, associated types, higher-kinded parameters, const generics, generic methods, or specialization/overload system. A type parameter cannot be used with an operator or built-in that requires a capability not established by the checker. Such code receives a type diagnostic.
- Generic functions may bind, pass, return, and forward values of parameter types. Operations requiring a particular primitive or aggregate capability must still be supported by the existing concrete type rules.
- Generic structs and enums preserve nominal identity, arity checks, exhaustive matching, and array copy-on-write (COW) behavior. `Option<T>` and `Result<T, E>` are ordinary generic enums, not magic checker cases or dedicated VM opcodes; they land only after generic enum parity is proven.

### 4. Substitution, recursion, and caching

Substitution operates on parsed type structure and resolved declaration identities. It must not replace identifier substrings in source or resolve names heuristically in a backend.

Each substitution environment maps a declaration's type-parameter identity to a concrete type. Before accepting an instantiation, validate substituted fields, payloads, function signatures, type nesting, and nominal by-value layout cycles. The existing `Array<T>` indirection boundary remains legal; direct by-value recursion remains rejected after substitution.

The cache key is the generic declaration ID plus normalized ordered type-argument IDs. Instantiation order must be deterministic. Recursive calls to an identical in-progress instantiation reuse that instance so ordinary generic recursion works. Expanding recursion such as `f<T>` calling `f<Array<T>>` must stop with a compile-time resource diagnostic rather than producing unbounded instances.

### 5. Compile-time resource limits

Generic instantiation creates compiler work and generated output; it must fail closed before exhausting process resources. Centralize limits in `internal/limits` and enforce them in the checker/monomorphizer and Native code generator, with defensive checks for untrusted bytecode metadata.

At minimum, limits cover type parameters per declaration, recursive type-application depth, distinct function/type instances, active expansion depth, total monomorphized declarations, generated Native source bytes, aggregate metadata, verifier work, and match/payload tables after substitution. Cache hits do not count as new distinct instances. Numeric thresholds are centralized, documented, and tested at both sides of each boundary; they must not drift between checker and backends.

Limit failures must be source-positioned where a source declaration or instantiation triggered them. These compile-time limits do not claim a process-wide memory bound and do not replace [ADR 0005](0005-native-resource-guarantees.md).

### 6. Typed IR and Bytecode contract

- All generic templates are resolved before backend lowering. The shared IR contains concrete type identities, substituted metadata, and calls to concrete function IDs.
- Bytecode serializes concrete instances only. If these can be represented using the current concrete function/type records, keep Bytecode v7. If a required wire-format change is introduced, bump the version explicitly and continue rejecting unsupported versions; do not silently reinterpret v7.
- The verifier validates each concrete instance's type metadata, recursive layout, call signature, payload types, match tables, and shared limits before execution.
- Native generation consumes the same concrete IR as the VM compiler. Specialized functions and nominal types use collision-free compiler-generated identifiers; user-provided generic names are never trusted as raw Go source.
- COW boundaries must remain correct through generic parameter passing, returns, fields, enum payloads, and extraction.

### 7. Required implementation sequence and acceptance criteria

Implement small vertical slices in this order:

1. Type-parameter syntax and structural substitution for generic functions.
2. Generic function inference and canonical monomorphization.
3. Generic structs and enum payloads, including post-substitution layout validation.
4. Concrete Bytecode serialization/verifier and Native codegen parity.
5. Generic `Option<T>` and `Result<T, E>` enums.

Each slice needs parser acceptance/rejection tests, negative type-checking tests with stable diagnostic codes, recursive-expansion boundary tests, malformed-bytecode tests, and VM/Native differential fixtures. Fixtures must cover one and multiple type parameters, nested `Array<T>`, recursive generic functions, enum payload bindings, and COW boundaries. Run the full CI matrix on Go stable and Go 1.23.x, including race detection, static analysis, examples, and 32-bit safety checks.

The language specification must describe only the exact features landed; the roadmap stays unchecked until both backends and all applicable negative tests are complete.

## Consequences

Monomorphization reuses concrete runtime and verifier contracts, but compilation and generated code grow with the number of unique instances. Instance caching and explicit caps are correctness requirements, not optional optimizations. Generic operator support remains intentionally limited until a separate constraints design defines capabilities precisely.
