# Front-end diagnostic codes

Aster's lexer, parser, and type checker attach stable diagnostic codes to source errors. The human-readable message remains available, and the traditional `line:column: phase error: message` prefix is preserved; the code is appended as `[CODE]`.

These codes are intended for tests, editor integrations, and bug reports. A code identifies the error class; message text may become clearer over time. Codes are not yet assigned to every runtime, bytecode-loader, code-generation, or toolchain error.

## Lexer

| Code | Meaning |
|---|---|
| `LEX000` | Other lexical error |
| `LEX001` | Source is not valid UTF-8 |
| `LEX002` | Integer literal is outside the `i64` range |
| `LEX003` | Invalid floating-point literal |
| `LEX004` | Newline in string literal |
| `LEX005` | Unsupported string escape sequence |
| `LEX006` | Unterminated string literal |
| `LEX007` | Unexpected character |
| `LEX008` | Unterminated block comment |

## Parser

| Code | Meaning |
|---|---|
| `PAR000` | Other syntax error |
| `PAR001` | Syntax nesting limit exceeded |
| `PAR002` | Expression nesting limit exceeded |
| `PAR003` | Parser made no progress (internal parser safeguard) |
| `PAR004` | Function name uses a reserved compiler-internal prefix |
| `PAR005` | `main` is reserved for the top-level program |
| `PAR006` | Type nesting limit exceeded |
| `PAR007` | Array element assignment is not supported in Aster 0.1 |
| `PAR008` | Integer literal is out of range |
| `PAR009` | Invalid floating-point literal |
| `PAR010` | Expected an expression |

## Type checker

| Code | Meaning |
|---|---|
| `TYP000` | Other type-checking error |
| `TYP001` | Duplicate function |
| `TYP002` | Reserved parameter name |
| `TYP003` | Invalid type annotation |
| `TYP004` | Parameter cannot have type `unit` |
| `TYP005` | Reserved function name |
| `TYP006` | Duplicate parameter |
| `TYP007` | Non-`unit` function does not return on every recognized path |
| `TYP008` | Unreachable statement |
| `TYP009` | Reserved binding name |
| `TYP010` | Duplicate variable in the same scope |
| `TYP011` | Variable cannot have type `unit` |
| `TYP012` | Initializer type does not match the declared type |
| `TYP013` | A variable cannot hold a `unit` value |
| `TYP014` | Undefined variable |
| `TYP015` | Assignment to an immutable variable |
| `TYP016` | Assignment type mismatch |
| `TYP017` | `if` condition is not `bool` |
| `TYP018` | `while` condition is not `bool` |
| `TYP019` | `return` outside a function |
| `TYP020` | Return type mismatch |
| `TYP021` | Unsupported statement node |
| `TYP022` | Cannot infer element type of an empty array |
| `TYP023` | Array cannot contain `unit` elements |
| `TYP024` | Heterogeneous array elements |
| `TYP025` | Type nesting limit exceeded |
| `TYP026` | Indexing a value that is not an array |
| `TYP027` | Array index is not `i64` |
| `TYP028` | Unary minus requires a numeric operand |
| `TYP029` | Logical negation requires `bool` |
| `TYP030` | Unsupported operands for `+` |
| `TYP031` | Arithmetic operands have mismatched types |
| `TYP032` | Remainder requires `i64` operands |
| `TYP033` | Operator requires matching numeric operands or strings |
| `TYP034` | Values are not comparable |
| `TYP035` | Logical operator requires `bool` operands |
| `TYP036` | Only named functions can be called |
| `TYP037` | Incorrect argument count for `print` |
| `TYP038` | `print` cannot display `unit` |
| `TYP039` | Incorrect argument count for `len` |
| `TYP040` | Unsupported argument type for `len` |
| `TYP041` | Incorrect argument count for `assert` |
| `TYP042` | `assert` requires a `bool` condition |
| `TYP043` | Undefined function |
| `TYP044` | Incorrect function argument count |
| `TYP045` | Function argument type mismatch |
| `TYP046` | Unsupported expression node |
