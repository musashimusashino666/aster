# Security and safety model

Aster 0.1 is a prototype. It has not undergone an independent security audit and must not be used as the sole safety boundary for untrusted or safety-critical workloads.

## Current safeguards

- Static type checking before execution.
- Immutable bindings by default and explicit `let mut` for mutation.
- Runtime type checks in the bytecode VM even after static checking.
- Checked signed-integer arithmetic and explicit division-by-zero errors.
- Recursion, operand-stack depth, bytecode-size, and instruction-budget limits in the VM.
- Bytecode version and structural validation, including bounded pool/code sizes.
- Native code is a normal host executable and has the same process privileges as its caller.
- The VM has no comprehensive total allocation quota for strings and arrays; memory exhaustion remains possible for programs that repeatedly grow data.


## Current gaps

- No complete bytecode control-flow / stack-depth verifier.
- No sandbox for file, network, or process access; those libraries are not present yet.
- Native code is a normal host executable and has the same process privileges as its caller.
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
