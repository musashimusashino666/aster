# Aster architecture

## Pipeline

```text
Aster source
  -> lexer / tokens
  -> parser / AST
  -> static checker / typed IR
  -> one of two backends
       -> typed IR -> Aster bytecode -> validated bytecode VM
       -> typed IR -> Go source generation -> host Go compiler -> native executable
```

The checker is run before both backends. This ensures that type errors are rejected consistently before execution, although runtime behavior and diagnostics still need ongoing parity testing.

## Packages

- `internal/token`: token kinds and source positions.
- `internal/lexer`: source-to-token conversion; no parser dependency.
- `internal/ast`: syntax tree definitions.
- `internal/parser`: recursive-descent statements and Pratt expressions.
- `internal/checker`: two-pass function discovery, static semantic checks, source type-depth validation, and `TypedProgram` output containing expression types, resolved function signatures, and normalized IR. `Check` remains a validation-only API; `Analyze` returns the semantic metadata.
- `internal/langtypes`: shared source type-name validation and maximum nesting depth used across frontend and bytecode validation.
- `internal/ir`: normalized typed nodes that preserve source positions, resolved local/function identities, builtin call targets, and checked arithmetic/indexing operations. Both native Go codegen and bytecode compilation consume this IR.
- `internal/limits`: shared string-size and array-length limits consumed by bytecode validation and native code generation.
- `internal/bytecode`: bytecode model, structural and resource validation, control-flow/stack-height/type/local-initialization verification, JSON serialization, and compiler. `CompileTyped` lowers normalized IR; `Compile` is an AST compatibility wrapper that runs analysis first.
- `internal/vm`: stack-based VM, call frames, checked arithmetic, built-ins, resource limits.
- `internal/codegen`: native Go source generation. `GenerateTyped` consumes checker-produced expression types and function signatures instead of re-inferring them; identifier mangling prevents Aster names from colliding with Go keywords, and helper functions implement Aster arithmetic checks.
- `internal/toolchain`: isolated temporary build directories, formatting and Go compiler invocation.
- `cmd/aster`: user-facing CLI.

## Bytecode

Bytecode is a versioned JSON format to make early inspection and debugging easy. It is not yet a compact, signed, or stable distribution format. The loader rejects unknown JSON fields, validates the format version, checks instruction references, and applies pre-decode and structural size limits. Control-flow dataflow verifies operand-stack heights and types at merge points, local initialization, operator and builtin types, function call signatures, and return types. It also rejects underflow and fallthrough past a function end and verifies disconnected instruction regions. These checks reduce malformed-bytecode risk but do not prove every semantic invariant or provide a sandbox. The VM separately enforces runtime limits, including a cap on active local slots across call frames.

## Native backend

The current native backend does not emit machine code itself and does not use LLVM yet. The checker lowers the AST into normalized typed IR, which is consumed by both the bytecode compiler and the native Go code generator. The native backend generates Go source from this typed IR and invokes the Go compiler. AST-facing compatibility entry points perform analysis before invoking the typed compilation path; they are not a separate AST-based backend. This gives the prototype a practical native-executable path while preserving a clean boundary for a future LLVM/Cranelift backend. Go remains a build-time dependency, and generated programs inherit platform availability from the Go toolchain.

## Intended future backend abstraction

The typed-program bridge exposes checker-produced expression types and function signatures. The normalized IR preserves source positions, function and local identities, builtin targets, and explicit checked operations for arithmetic and indexing; both backend compilers consume it. The checker and IR compiler apply the same conservative return-flow semantics. Future language features should extend this shared semantic representation instead of adding backend-specific type inference or AST lowering.
