# Aster 0.1 language specification (implemented subset)

This document describes the syntax and semantics implemented in the current prototype. It is not a promise that planned features already exist.

## Source model

A source file is UTF-8 text; an optional leading UTF-8 BOM is ignored. The current lexer supports ASCII identifiers, decimal integer and floating-point literals, quoted strings with `\n`, `\r`, `\t`, `\0`, `\\`, and `\"` escapes, `//` line comments, and nested `/* ... */` comments. Source positions are currently byte-based columns. Unexpected non-ASCII characters are reported by Unicode code point.

Identifiers begin with an ASCII letter or `_`, followed by ASCII letters, digits, or `_`. A file contains struct declarations, function declarations, and top-level statements. Struct type names and functions are visible throughout the file, including forward references and recursion. Imports and multiple compilation units are not supported yet.

## EBNF sketch

```ebnf
program       = { struct | function | statement } ;
struct        = "struct" identifier "{" [ fields ] "}" ;
fields        = field { "," field } [ "," ] ;
field         = identifier ":" type ;
function      = "fn" identifier "(" [ parameters ] ")"
                [ "->" type ] block ;
parameters    = parameter { "," parameter } ;
parameter     = identifier ":" type ;
type          = "i64" | "f64" | "bool" | "string" | "unit"
              | identifier | "Array" "<" type ">" ;
block         = "{" { statement } "}" ;
statement     = block
              | "let" [ "mut" ] identifier [ ":" type ] "=" expression ";"
              | identifier "=" expression ";"
              | "if" expression block [ "else" ( block | "if" expression block ) ]
              | "while" expression block
              | "return" [ expression ] ";"
              | expression ";" ;
expression    = ... | "[" [ expression { "," expression } ] "]"
                | expression "[" expression "]"
                | expression "." identifier ;
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

The JSON bytecode format is versioned independently of source syntax. Version 2 introduced function parameter type metadata so the loader can verify call arguments, stack value types, return values, array element types, and local initialization before VM execution. Version 3 added `SET_INDEX_PATH` for checked nested array-element assignment. Version 4 added `SLICE` for checked half-open array slicing. Version 5 adds `TO_I64` and `TO_F64` for explicit numeric casts. Version 6 adds nominal struct metadata plus `BUILD_STRUCT` and `GET_FIELD`; the current loader accepts version 6 and rejects older artifacts because they do not declare the instruction set and verification semantics required by the current VM.

- Bindings are immutable by default. Assignment is permitted only to `let mut` variables.
- A variable without a type annotation uses its initializer's type.
- Nested type constructors such as `Array<Array<i64>>` are limited to 256 levels. The parser and type checker reject deeper types, and the bytecode verifier uses the same shared limit.
- To bound recursive parsing and recursive AST traversal, syntax nesting is limited to 256 levels. The parser rejects excessively nested blocks, parenthesized/unary expressions, and expression ASTs made deep by operator, call, or index chains with a diagnostic error rather than passing an excessively deep tree to later stages.
- Implicit numeric conversions are not performed. Explicit casts use `i64(value)` and `f64(value)`: both accept an `i64` or `f64` argument, and a cast to the same type leaves the value unchanged. `f64(i64_value)` uses IEEE-754 binary64 conversion and may lose integer precision. `i64(f64_value)` truncates the fractional part toward zero and rejects NaN, infinities, and values outside the half-open range `[-2^63, 2^63)` with a runtime error.
- `if` and `while` conditions must have type `bool`.
- A function without an explicit return type returns `unit`.
- A non-unit function must not reach the end on any statically recognized path without returning a value of its declared type. Return-flow analysis recognizes direct returns, blocks, both branches of an unknown-condition `if/else`, literal-boolean `if` conditions (only the selected branch affects fallthrough), and literal-true `while` loops as non-fallthrough because `break` is not supported. A nonconstant `while` may execute zero times; a literal-false `while` executes no body and falls through.
- `&&` and `||` short-circuit.
- A non-generic `struct Name { field: Type, ... }` declares a nominal type visible across the source file. `Name(value1, value2, ...)` constructs a struct positionally in declaration order, with constructor arity and field types checked statically. `value.field` reads a declared field. Struct fields are read-only in this initial slice; direct field assignment is not supported. Structs cannot yet be printed or compared for equality; the checker rejects such operations rather than exposing the Native Go representation. Direct recursive by-value layouts are rejected, while recursion through `Array<T>` is permitted. Arrays stored in struct fields preserve array copy-on-write value semantics across construction, field extraction, and function argument/return boundaries. See [ADR 0006](adr/0006-nominal-data-types-and-pattern-matching.md) for the staged rollout and remaining questions.
- `Array<T>` is a homogeneous dynamically sized array. Non-empty array literals infer `T`; a declared `Array<T>` parameter or return type is supported. Array indexing uses `values[index]`; runtime checks reject negative and out-of-range indices. `len` returns array element count. A mutable root binding can update an element with `values[index] = value`; nested paths such as `matrix[row][column] = value` are supported when each intermediate element is an array. The root binding must be declared with `let mut`, and the assigned value must match the element type. Indexed assignment uses copy-on-write: each shared array container along the path to the updated element is detached before writing, while uniquely owned containers are updated in place. This preserves the value semantics in [ADR 0003](adr/0003-array-value-semantics.md): copied bindings, function arguments, and returned arrays do not observe writes through one another. Slices use `values[start:end]`, with an inclusive start and exclusive end; both bounds are required and must be `i64`. The runtime checks `0 <= start <= end <= len(values)` and returns an independent array value containing the selected elements, so later writes to the slice cannot change the original array. Empty slices are valid. Empty array literals remain unsupported even when a variable has an `Array<T>` annotation. Array literals are limited to 262,144 elements in both backends; oversized array literals are rejected during compilation.
- `i64` addition, subtraction, multiplication, negation, and `min_i64 / -1` overflow produce runtime errors. Integer division truncates toward zero, and remainder follows Go's signed-integer rule (the remainder has the dividend's sign); for example, `-7 / 2` is `-3` and `-7 % 2` is `-1`. Integer division and remainder by zero produce runtime errors. The `%` operator accepts only `i64` operands; `min_i64 % -1` evaluates to `0` and does not trap. The minimum value can be written as `-9223372036854775808`; the corresponding positive magnitude is not a valid `i64` literal.
- Floating-point division by zero and non-finite results are runtime errors in the current implementation. This is a deliberate prototype policy, not an inherent property of IEEE-754.
- Strings are immutable. `len` counts Unicode code points. String literals and concatenation results are limited to 16 MiB of UTF-8 bytes in both backends; oversized literals are rejected during compilation, and concatenations that would exceed the limit fail at runtime with a source-positioned error.
- `print`, `len`, and `assert` are reserved built-ins. Variables cannot store `unit` values, and `print` rejects a `unit` argument; a call returning `unit` can be used as an expression statement. Rendering each individual `print` call is limited to 16 MiB of UTF-8 output, and cumulative stdout across all `print` calls is limited to 64 MiB. Both limits count UTF-8 bytes; the cumulative limit also counts each call's trailing newline. A `print` that would exceed either limit raises a runtime error before any of that call's output is written.

## Unsupported syntax in 0.1

Enums, user-defined generics, lambdas, closures, pattern matching, `for`, `break`, `continue`, imports, modules, exceptions, `Option`, `Result`, `async/await`, macros, and user-defined operators are not implemented. Arrays are supported with the limitations described above. The parser or type checker reports an error rather than accepting these as supported language features.
