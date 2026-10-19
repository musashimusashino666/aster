# ADR 0001: Use generated Go as the first native backend

- Status: accepted for the 0.1 prototype
- Date: 2026-10-19

## Context

Aster should have a real executable build path while its own compiler infrastructure is still small. Writing a machine-code backend from scratch would significantly expand the prototype's scope.

## Decision

Lower the checked Aster AST to Go source, then invoke the host Go compiler in an isolated temporary directory. The VM remains the reference runtime. The code generator uses identifier mangling and explicit checked-arithmetic helpers.

## Consequences

- Aster can build real host executables with a mature compiler backend.
- Go is a build-time dependency for native output.
- The backend is not a direct Aster-to-machine-code compiler, and the generated executable inherits host platform capabilities.
- A future typed IR and LLVM/Cranelift backend can replace or complement this path without changing source syntax.
