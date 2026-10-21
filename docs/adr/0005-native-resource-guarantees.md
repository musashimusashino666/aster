# ADR 0005: Native resource guarantees and enforcement boundaries

- **Status:** Accepted
- **Date:** 2026-10-20
- **Relationship:** Clarifies the resource and threat-model consequences of [ADR 0001: Use generated Go as the first native backend](0001-native-backend.md). It does not change language semantics or implement new limits.
- **Implementation update (2026-10-21):** Native fuel is now implemented under [ADR 0009](0009-native-fuel-budget.md). The process-wide memory guarantee remains unimplemented.

## Context

Aster has two execution paths with different resource controls. The bytecode VM enforces a deterministic instruction budget and bounded VM data structures. The Native backend emits an ordinary Go executable and enforces several per-value and output limits, but it does not charge each language operation against a fuel budget and does not account for all process memory.

The Native toolchain also has two different deadlines:

- `aster build` bounds formatting and compilation with a two-minute build context.
- `aster run --engine native` uses a five-minute total context for the temporary Native build and the subsequent program execution.

The latter is a CLI wrapper deadline, not a limit embedded in the resulting executable. A program produced by `aster build` has no Aster-controlled wall-clock supervisor when launched separately. The deadline also is not a deterministic operation budget: runtime depends on host scheduling, generated Go code, garbage collection, and the machine.

These distinctions must be documented before adding another limit. A per-string or per-array bound does not imply a process-wide memory bound: a program can create many individually bounded values, hold nested values, detach COW storage, and cause temporary allocations.

## Decision

### 1. State the current guarantees by enforcement layer

The following limits are part of the current Aster implementation. They are not a sandbox or a comprehensive process resource guarantee.

| Resource | Bytecode VM | Generated Native backend |
|---|---|---|
| Aster function call depth | Maximum 4,096 | Maximum 4,096 |
| String value size | Maximum 16 MiB of UTF-8 bytes | Maximum 16 MiB of UTF-8 bytes |
| Array length | Maximum 262,144 elements, checked by bytecode validation and runtime operations | Maximum 262,144 elements for generated array literals; slices cannot extend beyond their source range |
| Rendered value per `print` | Maximum 16 MiB | Maximum 16 MiB |
| Cumulative stdout | Maximum 64 MiB, including each `print` newline | Maximum 64 MiB, including each `print` newline |
| Instruction/fuel budget | Maximum 100,000,000 VM instruction steps per execution | No Native instruction/fuel budget |
| Operand stack | Maximum 1,048,576 VM values | VM stack cap does not apply |
| Active local slots | Maximum 262,144 slots across live VM call frames | VM local-slot budget does not apply |
| Bytecode JSON input | Maximum 64 MiB, plus structural and aggregate validation limits | Not applicable to generated Native execution |
| Whole-process memory | No comprehensive process-wide allocation quota | No comprehensive process-wide memory quota |

The per-value limits constrain individual language values and output. They do not establish a total memory bound. The VM instruction budget bounds VM dispatch work, not all host-side costs such as bytecode validation. The VM stack and active-local-slot limits bound selected VM structures, not every allocation in the process.

### 2. Specify wrapper deadlines separately from executable guarantees

- `aster build` applies a two-minute deadline to native source generation and host Go-toolchain work; that timeout does not limit the executable it creates.
- `aster run --engine native` applies a five-minute deadline spanning native source generation, temporary build, and execution. The deadline clock starts before source loading and analysis. Source loading, parsing, and type checking are not cooperatively preemptible, so expiry during those phases is observed immediately after the phase returns. Native source generation is context-aware, and host-toolchain child processes are canceled at the deadline. This deadline is not a deterministic fuel budget.
- A standalone executable created by `aster build` has no Aster-provided execution timeout or comprehensive memory cap. Any limit supplied by the operating system, container, job runner, or caller is external to this contract.

### 3. Do not describe either backend as a sandbox

Generated Native code executes with the privileges of its host process. The current language has no Aster-level file, network, or process API, but the Native artifact is still a normal host executable. The VM is also not a security boundary for hostile code.

Documentation and release notes must distinguish:

1. **Per-value and output limits** enforced by the language runtime or code generator.
2. **VM execution budgets** enforced by the interpreter.
3. **CLI deadlines** enforced only while the CLI owns the child process.
4. **Process-wide CPU and memory controls** enforced by a supervisor or operating system.

Do not claim that a single per-value cap, Go runtime memory setting, or VM local-slot budget is equivalent to a total process-memory limit.

### 4. Requirements for future resource enforcement

Any future Native instruction budget must define what consumes budget, how loops and function calls are charged, how budget exhaustion is diagnosed, and how VM/Native failure class and exit-status parity is tested.

A hard whole-process memory ceiling for standalone executables requires a separately specified strategy that accounts for all relevant allocations and clearly names its scope. A language-level allocation counter alone is not a guarantee on resident process memory; a platform-enforced limit or supervised out-of-process execution may be necessary. Do not implement a partial cap and advertise it as a complete memory boundary.

This ADR records the resource contract and the criteria for future work. [ADR 0009](0009-native-fuel-budget.md) now defines the Native fuel value and accounting unit. This ADR still does not add a comprehensive process-memory allocator, a portable OS-level memory ceiling, or a sandbox.

## Validation

- Keep VM/Native regression tests for call depth, string growth, array bounds, rendered output, cumulative stdout, and runtime failure diagnostics.
- Keep tests for the VM instruction, operand-stack, and active-local-slot limits, and for bytecode loader size/structure limits.
- Keep the toolchain tests that verify the two-minute build context and caller-supplied cancellation/deadlines.
- Update this ADR and [the security model](../security-model.md) whenever a new limit changes the supported guarantee. Tests must distinguish a program-level resource diagnostic from a CLI timeout or an external process termination.

## Consequences

- Current Native per-value and output safeguards remain useful, but Native programs are not guaranteed to terminate within a fixed amount of CPU work or stay under a process-wide memory ceiling.
- The five-minute `aster run --engine native` deadline is useful for a CLI invocation, but it must not be presented as an execution limit on standalone binaries.
- VM and Native can retain different internal budgets only when the documentation explicitly identifies the affected execution path. Cross-backend parity tests remain required for language-visible failures.
- Stronger resource isolation is deferred until an implementation proposal defines measurable guarantees, diagnostics, platform coverage, and adversarial tests.
