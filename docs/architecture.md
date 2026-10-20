# Aster architecture

## Pipeline

```text
Aster source
  -> lexer / tokens
  -> parser / AST
  -> static checker
  -> one of two backends
       -> Aster bytecode -> validated bytecode VM
       -> Go source generation -> host Go compiler -> native executable
```

The checker is run before both backends. This ensures that type errors are rejected consistently before execution, although runtime behavior and diagnostics still need ongoing parity testing.

## Packages

- `internal/token`: token kinds and source positions.
- `internal/lexer`: source-to-token conversion; no parser dependency.
- `internal/ast`: syntax tree definitions.
- `internal/parser`: recursive-descent statements and Pratt expressions.
- `internal/checker`: two-pass function discovery, static semantic checks, source type-depth validation, and `TypedProgram` output containing expression types, resolved function signatures, and normalized IR. `Check` remains a validation-only API; `Analyze` returns the semantic metadata.
- `internal/langtypes`: shared source type-name validation and maximum nesting depth used across frontend and bytecode validation.
- `internal/ir`: normalized typed nodes that preserve source positions, resolved local/function identities, and checked arithmetic/indexing operations. The native backend consumes this IR; bytecode migration is still in progress.
- `internal/limits`: shared string-size and array-length limits consumed by bytecode validation and native code generation.
- `internal/bytecode`: bytecode model, structural and resource validation, control-flow/stack-height/type/local-initialization verification, JSON serialization, compiler. The compiler shares the checker's conservative `AlwaysReturns` rule, but still lowers AST nodes directly.
- `internal/vm`: stack-based VM, call frames, checked arithmetic, built-ins, resource limits.
- `internal/codegen`: native Go source generation. `GenerateTyped` consumes checker-produced expression types and function signatures instead of re-inferring them; identifier mangling prevents Aster names from colliding with Go keywords, and helper functions implement Aster arithmetic checks.
- `internal/toolchain`: isolated temporary build directories, formatting and Go compiler invocation.
- `cmd/aster`: user-facing CLI.

## Bytecode

Bytecode is a versioned JSON format to make early inspection and debugging easy. It is not yet a compact, signed, or stable distribution format. The loader rejects unknown JSON fields, validates the format version, checks instruction references, and applies pre-decode and structural size limits. Control-flow dataflow verifies operand-stack heights and types at merge points, local initialization, operator and builtin types, function call signatures, and return types. It also rejects underflow and fallthrough past a function end and verifies disconnected instruction regions. These checks reduce malformed-bytecode risk but do not prove every semantic invariant or provide a sandbox. The VM separately enforces runtime limits, including a cap on active local slots across call frames.

## Native backend

The current native backend does not emit machine code itself and does not use LLVM yet. It generates Go source from a type-checked AST and calls the Go compiler. This gives the prototype a practical native-executable path while preserving a clean boundary for a future LLVM/Cranelift backend. It also means Go is currently a build-time dependency, and generated programs inherit platform availability from the Go toolchain.

## Intended future backend abstraction

The typed-program bridge now exposes checker-produced expression types and function signatures. A first normalized IR stage also preserves source positions, function and local identities, builtin targets, and explicit checked operations for arithmetic and indexing; the native Go generator consumes it. Return-flow analysis is shared by the checker and bytecode compiler. Next, migrate bytecode generation to the same IR while preserving bytecode verification and VM/native parity tests, then remove remaining AST-based backend lowering before adding another backend.
