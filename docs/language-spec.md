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

- Bindings are immutable by default. Assignment is permitted only to `let mut` variables.
- A variable without a type annotation uses its initializer's type.
- Implicit numeric conversions are not performed.
- `if` and `while` conditions must have type `bool`.
- A function without an explicit return type returns `unit`.
- Non-unit functions must return the declared type on all statically recognized paths. Current return-flow analysis handles direct returns, blocks, and `if/else`; it does not treat loops as guaranteed to execute.
- `&&` and `||` short-circuit.
- `Array<T>` is a homogeneous dynamically sized array. Non-empty array literals infer `T`; a declared `Array<T>` parameter or return type is supported. Array indexing uses `values[index]`; runtime checks reject negative and out-of-range indices. `len` returns array element count. Array elements cannot be assigned to in 0.1, and empty array literals are rejected in 0.1, even when a variable has an `Array<T>` annotation.
- `i64` addition, subtraction, multiplication, negation, and `min_i64 / -1` overflow produce runtime errors. Integer division and remainder by zero produce runtime errors.
- Floating-point division by zero and non-finite results are runtime errors in the current implementation. This is a deliberate prototype policy, not an inherent property of IEEE-754.
- Strings are immutable. `len` counts Unicode code points.
- `print`, `len`, and `assert` are reserved built-ins. Variables cannot store `unit` values, and `print` rejects a `unit` argument; a call returning `unit` can be used as an expression statement.

## Unsupported syntax in 0.1

Structs, enums, user-defined generics, lambdas, closures, pattern matching, `for`, `break`, `continue`, imports, modules, exceptions, `Option`, `Result`, `async/await`, macros, user-defined operators, and explicit casts are not implemented. Arrays are supported with the limitations described above. The parser or type checker reports an error rather than accepting these as supported language features.
