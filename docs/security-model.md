# Security and safety model

Aster 0.1 is a prototype. It has not undergone an independent security audit and must not be used as the sole safety boundary for untrusted or safety-critical workloads.

## Current safeguards

- Static type checking before execution.
- Immutable bindings by default and explicit `let mut` for mutation.
- Runtime type checks in the bytecode VM even after static checking.
- Checked signed-integer arithmetic and explicit division-by-zero errors.
- Call depth is limited to 4,096 in both the VM and generated native programs. The VM additionally limits operand-stack depth, bytecode size, and instruction count. Active VM call frames are also limited to a cumulative 262,144 local slots; this budget is checked before allocating a new frame.
- Both backends enforce language-level limits of 16 MiB of UTF-8 bytes per string, 262,144 elements per array, 16 MiB of rendered UTF-8 bytes per `print`, and 64 MiB of cumulative stdout. The stdout budget counts UTF-8 bytes, including the newline per `print`; an output that would exceed the remaining budget is rejected before being written. The VM bytecode loader separately caps JSON input at 64 MiB and preflights constant-pool, function, instruction, parameter-type metadata, type-name length, string-byte, and array limits while streaming JSON tokens, before expanding the full program into Go structs.
- Version 2 bytecode metadata includes function parameter types. Validation checks structure and resource bounds, then performs control-flow dataflow over operand-stack types and local initialization; stack nodes are shared across states and total local-state cells are capped to keep verification work bounded. Operand-stack heights and types must agree at merge points.
- Native code generation uses mangled identifiers and checked arithmetic helpers; regression tests cover checked arithmetic for annotated array elements.
- Generated Native programs consume a deterministic budget of 100,000,000 fuel units per execution: one unit per executed IR statement, function entry, and loop iteration (including empty loops). Exhaustion is reported as `RUN013`. This metric differs from the VM's bytecode instruction-step budget; neither metric is a process-memory quota.
- Native build operations are bounded by a two-minute context. The `aster run --engine native` CLI path uses a five-minute total context for both the temporary Native build and execution. These are wrapper deadlines, not limits embedded in a standalone executable.
- Temporary build directories are removed after native compilation.
- [ADR 0005](adr/0005-native-resource-guarantees.md) defines the scope and limits of the current VM/Native resource guarantees.

## Current gaps

- The bytecode verifier now checks abstract value types, call signatures, return types, array element consistency, and definite local initialization. It still does not prove all semantic invariants or provide a sandbox.
- No sandbox for file, network, or process access; those libraries are not present yet.
- Native code is a normal host executable and has the same process privileges as its caller. Shared call-depth, string, array, rendered-value, cumulative stdout, and deterministic Native fuel limits apply to generated executables. The VM's operand-stack cap, active local-slot budget, and bytecode-loader limits do not apply to Native. The VM instruction-step budget and Native fuel budget use different accounting units. `aster run --engine native` has a five-minute CLI deadline spanning native source generation, build, and execution; a standalone executable produced by `aster build` has no Aster-provided wall-clock deadline, but it still enforces the Native fuel budget. Source loading, parsing, and type checking are not cooperatively preemptible, so a deadline expiring during those phases is detected when the phase returns. The cumulative stdout-byte budget bounds output volume but is not a complete resource boundary; neither backend has a comprehensive process-wide memory cap. Native execution is not a sandbox. See [ADR 0005](adr/0005-native-resource-guarantees.md) and [ADR 0009](adr/0009-native-fuel-budget.md).
- Neither backend has a comprehensive process-wide byte-accurate allocation quota. Per-value limits do not bound the number of simultaneously retained values, nested values, COW detach/copy allocations, temporary buffers, stack metadata, or Go runtime overhead. The VM's active-local-slot budget is a bounded frame-allocation safeguard, not a complete memory limit; the Native backend has no equivalent active-slot budget. Neither backend is a sandbox.
- No dependency signature verification or package lockfile yet.
- A scheduled fuzz campaign now runs the lexer, parser, type checker, bytecode reader, instruction verifier, and structured-program verifier for 10 minutes per target each week. This is an initial automated baseline, not a substitute for long-duration campaigns, corpus review, formal semantics, independent audit, or release attestation.
- The type system does not yet model ownership or borrowing; the design proposal's full memory-safety model is future work, not implemented behavior.

## Release requirements before security-sensitive use

1. Continue validating formal resource invariants and decide whether a process-wide memory boundary is needed. [ADR 0005](adr/0005-native-resource-guarantees.md) and [ADR 0009](adr/0009-native-fuel-budget.md) distinguish implemented fuel budgets from unimplemented process-wide memory isolation.
2. Extend regression coverage for verifier invariants and cumulative resource limits; the current control-flow, stack-height, type, call-signature, return-type, and local-initialization analyses are implemented.
3. Review the scheduled fuzz campaign results and saved failure corpora; increase fuzz duration and expand lexer/parser/compiler/bytecode targets as the implementation grows. Continue expanding differential backend tests for outputs, failure classes, and exit status.
4. Audit unsafe assumptions at every FFI and host-runtime boundary.
5. Establish vulnerability reporting, patched-release policy, and signed artifacts.
6. Clearly distinguish memory safety from application-level correctness and sandboxing.
