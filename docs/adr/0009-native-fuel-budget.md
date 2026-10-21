# ADR 0009: Deterministic Native fuel budget

- **Status:** Accepted
- **Date:** 2026-10-21
- **Relationship:** Extends [ADR 0005: Native resource guarantees](0005-native-resource-guarantees.md). It supersedes ADR 0005's decision to defer a Native fuel budget, but it does not define a process-wide memory quota or sandbox.

## Context

The bytecode VM has a deterministic 100,000,000-step instruction budget. Generated Native programs previously had per-value and output limits, call-depth limits, and (when invoked by the CLI) a wall-clock deadline, but no language-level counter to stop an empty infinite loop. A CLI deadline is not embedded in a standalone executable, and it is not equivalent to a deterministic work budget.

Per-value limits also do not guarantee a process-wide memory ceiling. This ADR therefore defines fuel independently from memory isolation.

## Decision

### 1. Fuel limit and accounting unit

- Every generated Native program has a single process-wide budget of 100,000,000 Native fuel units per execution.
- One fuel unit is charged for each executed IR statement.
- One fuel unit is charged on each Aster function entry.
- One fuel unit is charged on every loop iteration, including loops with a constant-true condition and an empty body. This is additional to the one-time charge for reaching the loop statement.
- The fuel counter is monotonic and shared across the call stack. It is not reset on function entry or loop entry.
- The VM's 100,000,000 bytecode instruction-step budget remains unchanged. Native fuel units are a different metric and must not be described as one-to-one equivalent VM instructions.

### 2. Exhaustion diagnostic

- Fuel exhaustion raises a runtime failure with the message form `instruction budget exceeded (maximum 100000000 Native fuel units)` and stable diagnostic code `RUN013`.
- The failure uses the source position of the statement, function call site, or loop that attempted to consume fuel. When an end position is available, CLI-run Native execution returns it in structured diagnostic fields.
- The standalone generated executable still renders the legacy start-only error text. The fuel check itself is enforced in the executable, independent of the CLI timeout.
- Classification and tests must remain aligned with the VM's `RUN013` failure class; the numeric threshold and accounting metric need not match the VM's step count exactly.

### 3. Guarantees and non-guarantees

- Fuel bounds the number of charged language-level control/evaluation events. It is a deterministic work guard, not a wall-clock guarantee: one charged statement can still perform a comparatively expensive bounded operation.
- Existing string, array, output, nesting, and call-depth checks remain in effect.
- `aster run --engine native` retains its separate five-minute wrapper deadline. `aster build` retains its build deadline, which does not apply to a later standalone run.
- This ADR does not add a comprehensive process-wide memory quota, byte-accurate allocator accounting, OS resource limits, or sandboxing. Those require a separate, platform-scoped design and adversarial validation.

## Consequences

- Empty infinite loops and loops with statements consume fuel and terminate with `RUN013` after exhausting the budget, including in standalone Native executables.
- Runtime fuel accounting may differ from the VM for the same source program. Cross-backend tests should compare the public failure class for intentionally exhausted programs when their budgets can be lowered for tests; they should not assert identical fuel-consumption counts.
- The existing Native wall-clock deadline and per-value limits remain separate controls. A Native fuel budget must not be advertised as a hard memory cap or sandbox.

## Validation

- Unit-test the `RUN013` diagnostic-classifier parity between VM and generated Native runtime.
- Lower the fuel constant in a test-generated executable and verify that an empty infinite loop terminates with `RUN013` and a source span.
- Run the full CI matrix on Go stable and Go 1.23.x, including race detection, static analysis, examples, and the 32-bit check.