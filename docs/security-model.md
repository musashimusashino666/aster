# Security and safety model

Aster 0.1 is a prototype. It has not undergone an independent security audit and must not be used as the sole safety boundary for untrusted or safety-critical workloads.

## Current safeguards

- Static type checking before execution.
- Immutable bindings by default and explicit `let mut` for mutation.
- Runtime type checks in the bytecode VM even after static checking.
- Checked signed-integer arithmetic and explicit division-by-zero errors.
- Recursion, operand-stack depth, bytecode-size, and instruction-budget limits in the VM.
- Per-value allocation bounds in the VM: strings are limited to 16 MiB of UTF-8 bytes, arrays to 262,144 elements, rendered output to 16 MiB, and loaded bytecode JSON to 64 MiB. The loader preflights constant-pool, function, instruction, parameter-type metadata, type-name length, string-byte, and array limits while streaming JSON tokens, before expanding the full program into Go structs.
- Version 2 bytecode metadata includes function parameter types. Validation checks structure and resource bounds, then performs control-flow dataflow over operand-stack types and local initialization; stack nodes are shared across states and total local-state cells are capped to keep verification work bounded. Operand-stack heights and types must agree at merge points.
- Native code generation uses mangled identifiers and checked arithmetic helpers; regression tests cover checked arithmetic for annotated array elements.
- Temporary build directories are removed after native compilation.

## Current gaps

- The bytecode verifier now checks abstract value types, call signatures, return types, array element consistency, and definite local initialization. It still does not prove all semantic invariants or provide a sandbox.
- No sandbox for file, network, or process access; those libraries are not present yet.
- Native code is a normal host executable and has the same process privileges as its caller. VM instruction, call-depth, operand-stack, string, array, and printed-output limits do not currently apply to generated native executables.
- The VM has per-value and decoded-constant bounds, but no comprehensive process-wide allocation quota. Repeated temporary allocations can still consume CPU and memory, and the VM is not a sandbox.
- No dependency signature verification or package lockfile yet.
- No fuzzing campaign, formal semantics, independent audit, or release attestation.
- The type system does not yet model ownership or borrowing; the design proposal's full memory-safety model is future work, not implemented behavior.

## Release requirements before security-sensitive use

1. Define formal safety invariants for the VM and native backend.
2. Add a verified control-flow / operand-stack analysis for bytecode.
3. Add lexer/parser/compiler fuzz tests and differential backend tests.
4. Audit unsafe assumptions at every FFI and host-runtime boundary.
5. Establish vulnerability reporting, patched-release policy, and signed artifacts.
6. Clearly distinguish memory safety from application-level correctness and sandboxing.
