# ADR 0006: Nominal data types and pattern-matching rollout

- **Status:** Accepted
- **Date:** 2026-10-20
- **Relationship:** Builds on [ADR 0003: Array value semantics](0003-array-value-semantics.md) and [ADR 0005: Native resource guarantees and enforcement boundaries](0005-native-resource-guarantees.md). [ADR 0007](0007-user-defined-generics-and-monomorphization.md) specifies the generic substitution and monomorphization strategy. The staged rollout described here has landed in the implemented subset; see [the roadmap](../roadmap.md) and [language specification](../language-spec.md) for current status and exact limitations. PRs #61 and #63 introduced the initial struct and enum/match slices; subsequent changes added generic functions, structs, and enums.

## Context

When this ADR was drafted, Aster supported primitive types, `Array<T>`, and the initial non-generic-struct subset. Adding enums, generics, `Option<T>`, and `Result<T, E>` together would have combined several independent changes: name resolution, recursive type validation, aggregate representation, bytecode verification, pattern exhaustiveness, and generic instantiation. The VM and Native backends share a typed IR, so these semantics are resolved by the checker and carried through that IR rather than re-inferred by either backend. The resulting implemented subset and its remaining limits are recorded in the language specification.

Arrays already have explicit value semantics. Any aggregate that contains an array must preserve those semantics across bindings, function arguments and returns, field reads, and aggregate construction. A shallow Go-struct copy must not become an observable aliasing loophole.

## Decision

The accepted implementation sequence was split into separate, reviewable vertical slices:

1. **Non-generic structs** — nominal declarations, field type validation, construction, read-only field selection, value passing, and VM/Native parity.
2. **Enums with payloads and exhaustive `match` expressions** — named variants, payload construction/deconstruction, arm-local bindings, result-type agreement, unreachable/duplicate-arm checks, and exhaustiveness checks.
3. **User-defined generics** — generic type and function parameters, type-argument substitution, constraints/instantiation policy, recursion/resource limits, and bytecode metadata.
4. **Generic `Option<T>` and `Result<T, E>` declarations** — model them as ordinary user-defined generic enums using the enum and generic machinery; they are not predeclared built-ins or a standard error-handling library.

Each slice must have its own syntax/semantics tests, negative type-checking tests, bytecode malformed-input/verifier tests, VM/Native differential tests, and release documentation. Do not mark a feature complete merely because the parser accepts its syntax.

### Initial struct source model (implemented subset)

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
- The initial slice rejects direct by-value recursive cycles with a type diagnostic and permits recursion guarded by `Array<T>`. The Bytecode verifier applies the same layout rule; this does not change the resource guarantees described by ADR 0005.
- Printed struct representation must be specified and tested in both backends before structs are accepted as arguments to `print`; it must not leak Go's internal field names or COW metadata.

Using a call-shaped constructor avoids making `Type { ... }` ambiguous with the existing `if condition { ... }` block syntax. The syntax can be reconsidered if a future parser design adds an unambiguous record-literal form.

### Initial non-generic enum and match source model

Enums are closed nominal sum types. The initial non-generic slice supported one or more variants, each with zero or more positional payload types. Generic enum declarations were added in the later rollout described in [ADR 0007](0007-user-defined-generics-and-monomorphization.md):

```aster
enum Message {
    Quit,
    Move(i64, i64),
    Write(string),
}

fn describe(message: Message) -> string {
    match message {
        Message::Quit => "quit",
        Message::Move(x, y) => "move",
        Message::Write(text) => text,
    }
}

let message = Message::Move(10, 20);
print(describe(message));
```

The initial grammar for this slice was:

```ebnf
enum_decl       = "enum" identifier "{" enum_variant
                  { "," enum_variant } [ "," ] "}" ;
enum_variant    = identifier [ "(" type { "," type } [ "," ] ")" ] ;
variant_value   = enum_name "::" variant_name
                | enum_name "::" variant_name
                  "(" expression { "," expression } [ "," ] ")" ;
match_expr      = "match" expression "{" match_arm
                  { "," match_arm } [ "," ] "}" ;
match_arm       = pattern "=>" expression ;
pattern         = "_" | enum_name "::" variant_name
                | enum_name "::" variant_name
                  "(" binding { "," binding } [ "," ] ")" ;
binding         = identifier ;
```

The lexer must recognize `::` as a single token. Match arms are comma-separated and a trailing comma is optional. A payload variant declares at least one payload type; a zero-payload variant is written without parentheses. Constructors and patterns for payload variants require parentheses with exactly the declared number of arguments or bindings.

Semantics and scope:

- Enum declarations are top-level and visible throughout the source file, including before their declaration, matching the existing function and struct declaration model. Type names share the nominal type namespace with structs. Enum names cannot collide with struct names, other enums, built-in types, or reserved built-in names. Variant names must be unique within one enum; the same variant name in two different enums is permitted because all construction and matching uses the qualified `Enum::Variant` form.
- The initial enum slice was non-generic. Generic enum declarations and generic payload substitution/inference were added later; see [ADR 0007](0007-user-defined-generics-and-monomorphization.md) and the language specification. Payloads must resolve to supported storable types after substitution; `unit` payloads and implicit conversions remain unsupported. Direct by-value recursive cycles across structs and enums are rejected; recursion guarded by `Array<T>` is permitted under the existing type-nesting and resource limits.
- Variant identity is the pair (enum type ID, declaration-order variant index). Constructor arguments are evaluated once, left to right, and must match payload types and arity exactly. Constructed values preserve aggregate value semantics, including array copy-on-write boundaries across construction, local bindings, function arguments/returns, and payload extraction.
- `match` evaluates its subject exactly once and requires the subject to have an enum type. Every named pattern must reference a variant of that exact enum. Pattern payload arity must match the variant declaration. A payload binding is immutable and visible only in that arm's expression; repeated binding names in one pattern are rejected. `_` matches any remaining variant and does not bind the subject.
- Every arm is an expression. All arm expressions must have the same type; there is no implicit coercion. Only the selected arm is evaluated, and there is no fallthrough. A match is exhaustive when every variant is covered explicitly or by a wildcard. A variant may be named in at most one arm. A second wildcard, any arm after a wildcard, and a wildcard after all variants have already been explicitly covered are unreachable and rejected. Missing variants without a wildcard are a type-checking error.
- The implemented flat-match subset still excludes guards, nested patterns, literal patterns, alternative/or-patterns, structural destructuring of structs, and exceptions. Generic enums are supported separately under [ADR 0007](0007-user-defined-generics-and-monomorphization.md). Unsupported forms must receive a parser or checker diagnostic rather than reaching a backend.
- Assign stable type-checker diagnostic codes for enum declaration/variant conflicts, invalid payload types, constructor type/arity mismatch, non-enum match subjects, unknown or mismatched variants, pattern binding errors, duplicate/unreachable arms, non-exhaustive matches, and arm result-type disagreement. Tests must assert diagnostic codes, not only error text.

Bytecode v7 and backend contract for enums/match:

- Increment the bytecode format from v6 to v7. Version 7 is the only accepted format after the change; continue rejecting unsupported versions. Add immutable `EnumTypes` metadata containing each enum's stable ID, name, declaration-order variants, and payload type lists.
- Add a tagged enum runtime value that stores the enum type ID, variant index, and payload values; do not infer a tag from source strings or generated Go type names.
- `BUILD_ENUM` identifies the enum type ID and variant index and consumes exactly the payload values declared for that variant. The verifier validates enum/variant indexes, arity, and each operand's abstract type before VM execution.
- Represent each source-level match as an explicit bytecode match table. A table names one enum type and contains exactly one valid target PC for every declaration-order variant; wildcard coverage is expanded to the corresponding variant entries during lowering. `MATCH_ENUM` dispatches from an initialized local holding a value of that enum type. The verifier checks enum/local type agreement, table size and type ID, every target PC, payload access indexes/types, and compatible abstract operand-stack states at branch merges. It must reject an incomplete or malformed dispatch table before execution.
- `GET_ENUM_PAYLOAD` reads a declared payload slot from the verified enum value/local. It cannot be used with a mismatched enum type, variant, or out-of-range payload index. All branch and payload metadata are resolved in the typed IR; neither backend re-resolves variant names from source text.
- The Native backend must emit collision-free Go identifiers and implement the same selected-arm, payload, evaluation-order, failure-class, and aggregate value semantics as the VM. Generated source must not embed source identifiers as trusted raw Go fragments.
- Keep COW metadata out of serialized bytecode. Add VM/Native differential tests for nested arrays in enum payloads, function passing and returning enums, and arm bindings; compare output, exit status, and source-positioned failures.
- Enum metadata and match tables must obey the existing limits for type depth, aggregate recursion, verifier work, and generated source size. These features are not a process sandbox or a process-wide memory bound.

## Bytecode and backend requirements

- Add explicit aggregate type/field metadata to the normalized typed IR and bytecode program. The bytecode verifier must validate declared types, field indexes, constructor arity and operand types, enum tags/payloads, and match control flow before execution.
- Because aggregate type metadata and operations change the bytecode contract, increment the bytecode format from version 6 to version 7 rather than silently extending the current schema. Preserve the existing rejection of unsupported versions.
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
6. Update the roadmap only for the exact slice that landed. The roadmap currently marks structs, enums/match, and user-defined generics (including ordinary `Option<T>` / `Result<T, E>` enum definitions) as implemented; future extensions remain planned until their own implementation and test requirements are met.

## Open questions for later slices and deferred enhancements

- Whether aggregate equality will be explicit, derived structurally, or unavailable in the initial releases.
- Whether direct struct-field mutation should be added later, once aggregate COW paths are available.
- Whether nested, literal, guarded, or alternative patterns should be introduced after the initial flat-pattern slice.
