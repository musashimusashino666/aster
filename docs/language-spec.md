# Aster 0.1 language specification (implemented subset)

This document describes the syntax and semantics implemented in the current prototype. It is not a promise that planned features already exist.

## Source model

A source file is UTF-8 text. The current lexer supports ASCII identifiers, decimal integer and floating-point literals, quoted strings with `\n`, `\r`, `\t`, `\0`, `\\`, and `\"` escapes, `//` line comments, and nested `/* ... */` comments. Source positions are currently byte-based columns.

Identifiers begin with an ASCII letter or `_`, followed by ASCII letters, digits, or `_`. A file contains function declarations and top-level statements. Functions are visible throughout the file, including forward references and recursion. Imports and multiple compilation units are not supported yet.

## EBNF sketch

```ebnf
program       = { function | statement } ;
function      = "fn" identifier "(" [ parameters ] ")"
                [ "->" type ] block ;
parameters    = parameter { "," parameter } ;
parameter     = identifier ":" type ;
type          = "i64" | "f64" | "bool" | "string" | "unit" | "Array" "<" type ">" ;
block         = "{" { statement } "}" ;
statement     = block
              | "let" [ "mut" ] identifier [ ":" type ] "=" expression ";"
              | identifier "=" expression ";"
              | "if" expression block [ "else" ( block | "if" expression block ) ]
              | "while" expression block
              | "return" [ expression ] ";"
              | expression ";" ;
expression    = ... | "[" [ expression { "," expression } ] "]"
                | expression "[" expression "]" ;
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

The JSON bytecode format is versioned independently of source syntax. Version 2 records function parameter types so the loader can verify call arguments, stack value types, return values, array element types, and local initialization before VM execution. Version 1 bytecode is rejected explicitly rather than being interpreted without the metadata required for typed verification.

- Bindings are immutable by default. Assignment is permitted only to `let mut` variables.
- A variable without a type annotation uses its initializer's type.
- Nested type constructors such as `Array<Array<i64>>` are limited to 256 levels. The parser and type checker reject deeper types, and the bytecode verifier uses the same shared limit.
- To bound recursive parsing and recursive AST traversal, syntax nesting is limited to 256 levels. The parser rejects excessively nested blocks, parenthesized/unary expressions, and expression ASTs made deep by operator, call, or index chains with a diagnostic error rather than passing an excessively deep tree to later stages.
- Implicit numeric conversions are not performed.
- `if` and `while` conditions must have type `bool`.
- A function without an explicit return type returns `unit`.
- Non-unit functions must return the declared type on all statically recognized paths. Current return-flow analysis handles direct returns, blocks, and `if/else`; it does not treat loops as guaranteed to execute.
- `&&` and `||` short-circuit.
- `Array<T>` is a homogeneous dynamically sized array. Non-empty array literals infer `T`; a declared `Array<T>` parameter or return type is supported. Array indexing uses `values[index]`; runtime checks reject negative and out-of-range indices. `len` returns array element count. Array elements cannot be assigned to in 0.1, and empty array literals are rejected in 0.1, even when a variable has an `Array<T>` annotation. Array literals are limited to 262,144 elements in both backends; oversized array literals are rejected during compilation. The planned semantics for mutable indexing and slices are value-based (no alias-visible writes), as recorded in [ADR 0003](adr/0003-array-value-semantics.md); this does not enable those operations in 0.1.
- `i64` addition, subtraction, multiplication, negation, and `min_i64 / -1` overflow produce runtime errors. Integer division and remainder by zero produce runtime errors. The `%` operator accepts only `i64` operands; `min_i64 % -1` evaluates to `0` and does not trap. The minimum value can be written as `-9223372036854775808`; the corresponding positive magnitude is not a valid `i64` literal.
- Floating-point division by zero and non-finite results are runtime errors in the current implementation. This is a deliberate prototype policy, not an inherent property of IEEE-754.
- Strings are immutable. `len` counts Unicode code points. String literals and concatenation results are limited to 16 MiB of UTF-8 bytes in both backends; oversized literals are rejected during compilation, and concatenations that would exceed the limit fail at runtime with a source-positioned error.
- `print`, `len`, and `assert` are reserved built-ins. Variables cannot store `unit` values, and `print` rejects a `unit` argument; a call returning `unit` can be used as an expression statement.

## Unsupported syntax in 0.1

Structs, enums, user-defined generics, lambdas, closures, pattern matching, `for`, `break`, `continue`, imports, modules, exceptions, `Option`, `Result`, `async/await`, macros, user-defined operators, and explicit casts are not implemented. Arrays are supported with the limitations described above. The parser or type checker reports an error rather than accepting these as supported language features.
