# Aster 0.1 language specification (implemented subset)

This document describes the syntax and semantics implemented in the current prototype. It is not a promise that planned features already exist.

## Source model

A source file is UTF-8 text; an optional leading UTF-8 BOM is ignored. The current lexer supports ASCII identifiers, decimal integer and floating-point literals, quoted strings with `\n`, `\r`, `\t`, `\0`, `\\`, and `\"` escapes, `//` line comments, and nested `/* ... */` comments. Lexer tokens retain a start position and an exclusive end position. Offsets and columns count UTF-8 bytes, and lines/columns are 1-based. End positions are not yet propagated through all AST/typed-IR nodes, bytecode instructions, or runtime diagnostics. Unexpected non-ASCII characters are reported by Unicode code point.

Identifiers begin with an ASCII letter or `_`, followed by ASCII letters, digits, or `_`. A file contains struct and enum declarations, function declarations, and top-level statements. Struct/enum type names and functions are visible throughout the file, including forward references and recursion. Imports and multiple compilation units are not supported yet.

## EBNF sketch

```ebnf
program       = { struct | enum_decl | function | statement } ;
struct        = "struct" identifier [ type_params ] "{" [ fields ] "}" ;
fields        = field { "," field } [ "," ] ;
field         = identifier ":" type ;
enum_decl     = "enum" identifier [ type_params ] "{" enum_variant { "," enum_variant } [ "," ] "}" ;
enum_variant  = identifier [ "(" type { "," type } [ "," ] ")" ] ;
function      = "fn" identifier [ type_params ] "(" [ parameters ] ")"
                [ "->" type ] block ;
type_params   = "<" identifier { "," identifier } ">" ;
parameters    = parameter { "," parameter } ;
parameter     = identifier ":" type ;
type          = "i64" | "f64" | "bool" | "string" | "unit"
              | "Array" "<" type ">" | nominal_type ;
nominal_type   = identifier [ "<" type { "," type } ">" ] ;
block         = "{" { statement } "}" ;
statement     = block
              | "let" [ "mut" ] identifier [ ":" type ] "=" expression ";"
              | identifier "=" expression ";"
              | "if" expression block [ "else" ( block | "if" expression block ) ]
              | "while" expression block
              | "return" [ expression ] ";"
              | expression ";" ;
expression    = ... | "[" [ expression { "," expression } ] "]"
                | identifier "::" "<" type { "," type } ">" "(" [ arguments ] ")"
                | enum_name "::" "<" type { "," type } ">" "::" variant_name [ "(" arguments ")" ]
                | enum_name "::" variant_name [ "(" arguments ")" ]
                | "match" expression "{" match_arm { "," match_arm } [ "," ] "}"
                | expression "[" expression "]"
                | expression "." identifier ;
match_arm     = pattern "=>" expression ;
pattern       = "_" | enum_name "::" variant_name
                [ "(" identifier { "," identifier } [ "," ] ")" ] ;
```

Calls and operators are parsed with the following precedence, from low to high:

1. `||`
2. `&&`
3. `==`, `!=`
4. `<`, `<=`, `>`, `>=`
5. `+`, `-`
6. `*`, `/`, `%`
7. prefix `-`, `!`
8. function call

## Semantics

The JSON bytecode format is versioned independently of source syntax. Version 2 introduced function parameter type metadata so the loader can verify call arguments, stack value types, return values, array element types, and local initialization before VM execution. Version 3 added `SET_INDEX_PATH` for checked nested array-element assignment. Version 4 added `SLICE` for checked half-open array slicing. Version 5 adds `TO_I64` and `TO_F64` for explicit numeric casts. Version 6 added nominal struct metadata plus `BUILD_STRUCT` and `GET_FIELD`. Version 7 adds enum metadata, tagged enum values, enum construction, payload extraction, and verified match dispatch. The current loader accepts version 7 and rejects older artifacts because they do not declare the instruction set and verification semantics required by the current VM.

- Bindings are immutable by default. Assignment is permitted only to `let mut` variables.
- A variable without a type annotation uses its initializer's type.
- Nested type constructors such as `Array<Array<i64>>` are limited to 256 levels. The parser and type checker reject deeper types, and the bytecode verifier uses the same shared limit.
- To bound recursive parsing and recursive AST traversal, syntax nesting is limited to 256 levels. The parser rejects excessively nested blocks, parenthesized/unary expressions, and expression ASTs made deep by operator, call, or index chains with a diagnostic error rather than passing an excessively deep tree to later stages.
- Implicit numeric conversions are not performed. Explicit casts use `i64(value)` and `f64(value)`: both accept an `i64` or `f64` argument, and a cast to the same type leaves the value unchanged. `f64(i64_value)` uses IEEE-754 binary64 conversion and may lose integer precision. `i64(f64_value)` truncates the fractional part toward zero and rejects NaN, infinities, and values outside the half-open range `[-2^63, 2^63)` with a runtime error.
- `if` and `while` conditions must have type `bool`.
- A function without an explicit return type returns `unit`.
- A non-unit function must not reach the end on any statically recognized path without returning a value of its declared type. Return-flow analysis recognizes direct returns, blocks, both branches of an unknown-condition `if/else`, literal-boolean `if` conditions (only the selected branch affects fallthrough), and literal-true `while` loops as non-fallthrough because `break` is not supported. A nonconstant `while` may execute zero times; a literal-false `while` executes no body and falls through.
- `&&` and `||` short-circuit.
- A struct may declare type parameters, for example `struct Box<T> { value: T }`. Type annotations use `Box<i64>`; construction supports explicit arguments (`Box::<i64>(42)`) or inference from field values (`Box(42)`). Multi-parameter structs such as `Pair<K, V>` are supported. Each distinct concrete type is monomorphized to a nominal struct before ordinary checking and backend lowering; fields, constructor arguments, function signatures, field selection, and local annotations use the substituted concrete types. Fields remain read-only, and structs still cannot be printed or compared for equality. Direct recursive by-value layouts are rejected after specialization; recursion guarded by `Array<T>` is permitted. Arrays stored in struct fields preserve copy-on-write value semantics through construction, field extraction, and function argument/return boundaries. Type argument inference must determine every type parameter from constructor field values. The compiler limits generic type parameters to 32, generated struct instances to 4,096, and expanding instantiation depth to 256.
- An `enum Name { Variant, Payload(Type, ...) }` declares a closed nominal sum type. Generic enum declarations such as `enum Option<T> { None, Some(T) }` and `enum Result<T, E> { Ok(T), Err(E) }` are monomorphized to concrete nominal enum types before ordinary type checking and backend lowering. Type annotations accept applications such as `Option<i64>`, `Result<i64, string>`, including nesting in arrays and other nominal types. Enum and struct type names share a namespace and are visible throughout the file. Variant names are unique within an enum and are always qualified as `Name::Variant`. Payload types must be supported storable types after substitution; `unit` payloads and implicit conversions are not supported. Empty payload lists are written without parentheses. Generic enum type parameters, like generic function and struct parameters, are limited to 32; each program may instantiate up to 4,096 distinct generic enums, with expansion depth limited to 256.
- `Name::Variant(value, ...)` constructs a variant positionally, and generic constructors infer type arguments from payloads when the arguments determine a unique concrete instantiation. Explicit arguments use `Option::<i64>::None` or `Option::<i64>::Some(7)`. A payload-free variant requires an expected concrete enum type or explicit type arguments; e.g. `let empty: Option<i64> = Option::None;`. Payload argument types and arity are checked statically, and arguments are evaluated once from left to right. Enums cannot yet be printed or compared for equality. Direct by-value recursive cycles across structs and enums are rejected after substitution; recursion guarded by `Array<T>` is permitted.
- `match value { Name::Variant(binding, ...) => expression, _ => expression }` is an expression over an enum value. The subject is evaluated exactly once. Supported patterns are qualified variants with flat positional bindings and the wildcard `_`; nested patterns, guards, literals, or-patterns, and struct patterns are not supported. Payload patterns require exactly one binding per payload field. Bindings are immutable and scoped to their arm only. Every arm must have the same result type, and there is no fallthrough.
- A match must cover every variant explicitly or include a wildcard. Duplicate variant arms, repeated bindings, a wildcard after all variants are already covered, duplicate wildcards, arms after a wildcard, wrong-enum patterns, and non-exhaustive matches are rejected by the checker. Bytecode v7 stores enum IDs, declaration-order variant metadata, payload types, and per-function match dispatch tables; the verifier rejects malformed metadata, invalid indexes, mismatched subject types, and invalid dispatch targets before VM execution.
- Array values nested in enum payloads preserve the existing copy-on-write value semantics across enum construction, local bindings, function arguments/returns, and payload extraction. VM and Native Go code generation consume the resolved enum and variant identities from typed IR and are covered by differential tests.
- Generic functions, structs, and enums are compile-time templates that are substituted structurally and monomorphized before regular type checking and backend lowering. A generic function such as `fn identity<T>(value: T) -> T { return value; }` accepts explicit arguments (`identity::<i64>(7)`) or infers them from ordinary arguments (`identity(7)`). Inference structurally matches parameter patterns—including nested `Array<T>`, `Box<T>`, `Option<T>`, and `Result<T, E>`—against known concrete argument types. Enum constructors infer arguments from payloads; a payload-free variant needs an expected concrete type or explicit arguments. Generic functions do not infer solely from the expected return type. If arguments do not determine a unique type-argument set, the compiler emits the relevant stable generic-inference diagnostic (`TYP081`, `TYP085`, or `TYP088`). Exact argument counts are required; traits and generic operator constraints are not implemented. The compiler caps type parameters per declaration at 32, distinct generic function instances at 4,096, generic struct instances at 4,096, generic enum instances at 4,096, and expanding-instantiation depth at 256. Across all three generic categories combined, at most 8,192 distinct instances may be created in one program. Generated Native Go source is limited to 64 MiB of bytes. Exact recursive instantiations reuse the cached instance, and cache hits do not consume the combined instance budget.
- `Array<T>` is a homogeneous dynamically sized array. Non-empty array literals infer `T`; a declared `Array<T>` parameter or return type is supported. Array indexing uses `values[index]`; runtime checks reject negative and out-of-range indices. `len` returns array element count. A mutable root binding can update an element with `values[index] = value`; nested paths such as `matrix[row][column] = value` are supported when each intermediate element is an array. The root binding must be declared with `let mut`, and the assigned value must match the element type. Indexed assignment uses copy-on-write: each shared array container along the path to the updated element is detached before writing, while uniquely owned containers are updated in place. This preserves the value semantics in [ADR 0003](adr/0003-array-value-semantics.md): copied bindings, function arguments, and returned arrays do not observe writes through one another. Slices use `values[start:end]`, with an inclusive start and exclusive end; both bounds are required and must be `i64`. The runtime checks `0 <= start <= end <= len(values)` and returns an independent array value containing the selected elements, so later writes to the slice cannot change the original array. Empty slices are valid. Empty array literals remain unsupported even when a variable has an `Array<T>` annotation. Array literals are limited to 262,144 elements in both backends; oversized array literals are rejected during compilation.
- `i64` addition, subtraction, multiplication, negation, and `min_i64 / -1` overflow produce runtime errors. Integer division truncates toward zero, and remainder follows Go's signed-integer rule (the remainder has the dividend's sign); for example, `-7 / 2` is `-3` and `-7 % 2` is `-1`. Integer division and remainder by zero produce runtime errors. The `%` operator accepts only `i64` operands; `min_i64 % -1` evaluates to `0` and does not trap. The minimum value can be written as `-9223372036854775808`; the corresponding positive magnitude is not a valid `i64` literal.
- Floating-point division by zero and non-finite results are runtime errors in the current implementation. This is a deliberate prototype policy, not an inherent property of IEEE-754.
- `print` renders an `f64` as the shortest decimal representation that round-trips to the same binary64 value. General-format exponent notation is used when the decimal exponent is less than -4 or at least 6; the exponent includes a sign and at least two digits. For example, `print(1000000.0);` renders `1e+06`. The formatting is locale-independent and must match between VM and Native.
- Strings are immutable. `len` counts Unicode code points. String literals and concatenation results are limited to 16 MiB of UTF-8 bytes in both backends; oversized literals are rejected during compilation, and concatenations that would exceed the limit fail at runtime with a source-positioned error.
- `print`, `len`, and `assert` are reserved built-ins. Variables cannot store `unit` values, and `print` rejects a `unit` argument; a call returning `unit` can be used as an expression statement. Rendering each individual `print` call is limited to 16 MiB of UTF-8 output, and cumulative stdout across all `print` calls is limited to 64 MiB. Both limits count UTF-8 bytes; the cumulative limit also counts each call's trailing newline. A `print` that would exceed either limit raises a runtime error before any of that call's output is written.

## Unsupported syntax in 0.1

Trait bounds, lambdas, closures, general/nested pattern matching beyond the flat enum `match` described above, `for`, `break`, `continue`, imports, modules, exceptions, `async/await`, macros, and user-defined operators are not implemented. `Option<T>` and `Result<T, E>` are ordinary generic enums that programs may declare; they are not built-in types. Arrays are supported with the limitations described above. The parser or type checker reports an error rather than accepting these as supported language features.
