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
- `internal/checker`: two-pass function discovery and static semantic checks.
- `internal/bytecode`: bytecode model, structural validation, JSON serialization, compiler.
- `internal/vm`: stack-based VM, call frames, checked arithmetic, built-ins, resource limits.
- `internal/codegen`: native Go source generation. Identifier mangling prevents Aster names from colliding with Go keywords; helper functions implement Aster arithmetic checks.
- `internal/toolchain`: isolated temporary build directories, formatting and Go compiler invocation.
- `cmd/aster`: user-facing CLI.

## Bytecode

Bytecode is a versioned JSON format to make early inspection and debugging easy. It is not yet a compact, signed, or stable distribution format. The loader rejects unknown JSON fields, validates the format version, checks instruction references and applies structural size limits. Untrusted bytecode should still be treated cautiously until a full control-flow and stack verifier is added.

## Native backend

The current native backend does not emit machine code itself and does not use LLVM yet. It generates Go source from a type-checked AST and calls the Go compiler. This gives the prototype a practical native-executable path while preserving a clean boundary for a future LLVM/Cranelift backend. It also means Go is currently a build-time dependency, and generated programs inherit platform availability from the Go toolchain.

## Intended future backend abstraction

The next architectural step is a backend interface that consumes a typed, normalized intermediate representation rather than raw AST nodes. This will allow bytecode, native, WebAssembly, and future accelerator backends to share semantic decisions and lowerings. That IR should preserve source spans and explicit overflow/error behavior.
