# Security and safety model

Aster 0.1 is a prototype. It has not undergone an independent security audit and must not be used as the sole safety boundary for untrusted or safety-critical workloads.

## Current safeguards

- Static type checking before execution.
- Immutable bindings by default and explicit `let mut` for mutation.
- Runtime type checks in the bytecode VM even after static checking.
- Checked signed-integer arithmetic and explicit division-by-zero errors.
- Call depth is limited to 4,096 in both the VM and generated native programs. The VM additionally limits operand-stack depth, bytecode size, and instruction count. Active VM call frames are also limited to a cumulative 262,144 local slots; this budget is checked before allocating a new frame.
- Both backends enforce language-level limits of 16 MiB of UTF-8 bytes per string, 262,144 elements per array, and at most 16 MiB of rendered UTF-8 bytes for each individual `print` call. This is not a cumulative stdout limit: multiple calls may produce more than 16 MiB in total. The VM bytecode loader separately caps JSON input at 64 MiB and preflights constant-pool, function, instruction, parameter-type metadata, type-name length, string-byte, and array limits while streaming JSON tokens, before expanding the full program into Go structs.
- Version 2 bytecode metadata includes function parameter types. Validation checks structure and resource bounds, then performs control-flow dataflow over operand-stack types and local initialization; stack nodes are shared across states and total local-state cells are capped to keep verification work bounded. Operand-stack heights and types must agree at merge points.
- Native code generation uses mangled identifiers and checked arithmetic helpers; regression tests cover checked arithmetic for annotated array elements.
- Temporary build directories are removed after native compilation.

## Current gaps

- The bytecode verifier now checks abstract value types, call signatures, return types, array element consistency, and definite local initialization. It still does not prove all semantic invariants or provide a sandbox.
- No sandbox for file, network, or process access; those libraries are not present yet.
- Native code is a normal host executable and has the same process privileges as its caller. The shared call-depth, string, array, and per-`print` rendered-value limits apply to Native as well as the VM, but the VM's instruction budget, operand-stack cap, cumulative local-slot budget, and bytecode-loader limits do not apply to generated native executables. Since neither backend has a cumulative stdout-byte budget, and Native also lacks an instruction budget, output volume is not a reliable resource boundary. Native execution is not a sandbox.
- The VM has per-value, active-local-slot, and decoded-constant bounds, but no comprehensive process-wide byte-accurate allocation quota. Stacks, temporary copies, nested values, and repeated allocations can still consume CPU and memory; the VM is not a sandbox. The local-slot budget is a bounded frame-allocation safeguard, not a complete memory limit.
- No dependency signature verification or package lockfile yet.
- Fuzz targets exist for the lexer, parser, and bytecode validation, but there is no sustained fuzzing campaign, formal semantics, independent audit, or release attestation.
- The type system does not yet model ownership or borrowing; the design proposal's full memory-safety model is future work, not implemented behavior.

## Release requirements before security-sensitive use

1. Define formal safety invariants for the VM and native backend.
2. Extend regression coverage for verifier invariants and cumulative resource limits; the current control-flow, stack-height, type, call-signature, return-type, and local-initialization analyses are implemented.
3. Review the scheduled fuzz campaign results and saved failure corpora; increase fuzz duration and expand lexer/parser/compiler/bytecode targets as the implementation grows. Continue expanding differential backend tests for outputs, failure classes, and exit status.
4. Audit unsafe assumptions at every FFI and host-runtime boundary.
5. Establish vulnerability reporting, patched-release policy, and signed artifacts.
6. Clearly distinguish memory safety from application-level correctness and sandboxing.
