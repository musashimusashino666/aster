# Diagnostic codes

Aster attaches stable diagnostic codes to lexer, parser, type-checker, runtime, bytecode reader/verifier, and native toolchain errors. The human-readable message remains available. Source diagnostics preserve the `line:column: phase error: message` prefix; runtime errors include the source path when available and use `source:line:column: runtime error: message [CODE]`. Errors without a source location do not receive a fabricated line or column.

Codes are intended for tests, editor integrations, and bug reports. A code identifies the failure class; message text may become clearer over time. Runtime classification is shared between VM and generated Native code and is regression-tested to preserve backend parity. The Go toolchain diagnostic wraps the original error, so `errors.Is` and `errors.As` remain usable. Runtime locations currently identify the operation's start position; end offsets/columns are not yet propagated as complete source spans. The `i64(f64)` conversion range failure is coded separately and covers NaN, infinities, and values outside `[-2^63, 2^63)`.

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
| `PAR007` | Reserved: previously used for unsupported array-element assignment in Aster 0.1 |
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
| `TYP047` | Indexed assignment value type mismatch |
| `TYP048` | Slicing a value that is not an array |
| `TYP049` | Slice bounds must be `i64` |
| `TYP050` | Numeric conversion call does not have exactly one argument |
| `TYP051` | Numeric conversion argument is not `i64` or `f64` |
| `TYP052` | Duplicate, reserved, or conflicting struct declaration |
| `TYP053` | Duplicate struct field |
| `TYP054` | Struct field has an invalid or unsupported type |
| `TYP055` | Field selection applied to a non-struct value |
| `TYP056` | Unknown struct field |
| `TYP057` | Struct constructor has the wrong arity |
| `TYP058` | Struct constructor argument type mismatch |
| `TYP059` | Direct recursive by-value struct layout |
| `TYP060` | Struct printing is not yet supported |
| `TYP061` | Invalid or reserved struct type name |
| `TYP062` | Invalid struct field name
| `TYP063` | Duplicate, reserved, or conflicting enum declaration |
| `TYP064` | Invalid or duplicate enum variant |
| `TYP065` | Enum payload has an invalid or unsupported type |
| `TYP066` | Unknown enum type or variant |
| `TYP067` | Enum variant constructor has the wrong payload arity |
| `TYP068` | Enum constructor payload type mismatch |
| `TYP069` | Match subject does not have an enum type |
| `TYP070` | Match pattern belongs to a different enum than its subject |
| `TYP071` | Enum match pattern has the wrong payload-binding count |
| `TYP072` | Duplicate enum match arm |
| `TYP073` | Unreachable enum match arm |
| `TYP074` | Non-exhaustive enum match |
| `TYP075` | Match arm result types do not agree |
| `TYP076` | Duplicate or invalid match payload binding |
| `TYP077` | Recursive by-value enum/nominal layout |
| `TYP078` | Compile-time resource limit exceeded |


## Runtime

| Code | Meaning |
|---|---|
| `RUN000` | Other runtime failure or unclassified runtime safeguard |
| `RUN001` | Assertion failed |
| `RUN002` | Integer arithmetic overflow |
| `RUN003` | Division by zero |
| `RUN004` | Remainder by zero |
| `RUN005` | Non-finite floating-point result |
| `RUN006` | Array index out of bounds |
| `RUN007` | Invalid slice bounds |
| `RUN008` | `f64` to `i64` conversion out of range |
| `RUN009` | String byte-size limit exceeded |
| `RUN010` | Rendered print value or array nesting limit exceeded |
| `RUN011` | Cumulative stdout limit exceeded |
| `RUN012` | Call-depth limit exceeded |
| `RUN013` | VM instruction budget exceeded |
| `RUN014` | VM operand-stack limit exceeded |
| `RUN015` | Cumulative VM local-slot limit exceeded |
| `RUN016` | Output writer failed |
| `RUN017` | Array-size limit exceeded |

## Bytecode input and verification

| Code | Meaning |
|---|---|
| `BC000` | Other bytecode reader or verifier error |
| `BC001` | Invalid bytecode file format |
| `BC002` | Unsupported bytecode format version |
| `BC003` | Bytecode resource limit exceeded |
| `BC004` | Bytecode JSON decoding failed |
| `BC005` | Bytecode structure, instruction, or control-flow validation failed |
| `BC006` | Bytecode abstract-type, operand-stack, or local-state validation failed |

## Native toolchain

| Code | Meaning |
|---|---|
| `TOOL000` | Other native toolchain error |
| `TOOL001` | Native toolchain operation canceled |
| `TOOL002` | Native build or execution timed out |
| `TOOL003` | Required Go or `gofmt` tool unavailable |
| `TOOL004` | Formatting generated Go source failed |
| `TOOL005` | Native Go build failed |
| `TOOL006` | Built native program failed to execute |
| `TOOL007` | Toolchain filesystem or output handling failed |

Toolchain errors do not claim an Aster expression location: formatting, compilation, and process-launch failures often originate in generated Go code or the host environment. Their messages retain the Go tool output when available. Bytecode file errors include the supplied bytecode path, while malformed instructions may identify a function/instruction index rather than a source span.
