# ADR 0012: VM-managed cumulative allocation budget

- **Status:** Proposed
- **Date:** 2026-10-21
- **Related:** [ADR 0003: Array value semantics](0003-array-value-semantics.md), [ADR 0004: Copy-on-write array storage](0004-array-copy-on-write.md), [ADR 0005: Native resource guarantees](0005-native-resource-guarantees.md), [ADR 0010: Process-wide Native resource isolation](0010-process-resource-isolation.md), [Issue #107](https://github.com/musashimusashino666/aster/issues/107)

## Context

The VM already limits individual strings and arrays, bytecode input, verifier work, operand-stack depth, instruction count, call depth, and cumulative active local slots. These checks do not bound the total work-associated memory that one execution can cause the VM to allocate. A program can repeatedly create temporary strings or arrays, detach copy-on-write storage, or build nested values without exceeding a single value's limit. The Go runtime can also retain allocations until garbage collection.

A process RSS ceiling is a separate OS-level guarantee. A VM-local accounting mechanism must not be described as a process-wide memory quota, a precise heap measurement, or a sandbox.

A "currently live bytes" meter would require reliable ownership/release events for all value copies, slices, aggregate children, and COW storage. Ordinary Go values can be copied without invoking Aster code, and Go garbage collection does not provide deterministic release callbacks. Guessing when aliases die would therefore risk undercounting or double-counting.

## Decision proposed

Implement a **deterministic cumulative VM-managed allocation budget** for a single VM execution. The meter charges a conservative, documented byte-equivalent amount before VM-controlled allocations occur. Charges are monotonic for the duration of an execution and are not refunded when values become unreachable.

This policy intentionally bounds cumulative VM-managed allocation activity rather than estimating live heap size. Programs that allocate and discard repeatedly may exhaust the budget even if their live working set is small. That is the conservative trade-off for deterministic accounting without introducing reference counting/finalizers or depending on the Go garbage collector's schedule.

The proposed default budget is **256 MiB of accounted allocations per VM run**, subject to implementation review and measurement before acceptance. It must be an internal named constant initially, not an implicit guarantee about actual RSS. Any future public override requires separate API/design review and must remain bounded.

## Accounting contract

The implementation should centralize overflow-safe checks in a VM-owned accountant. Every charge must be checked before the corresponding allocation or copy; if the charge would exceed the remaining budget, reject the operation without performing that allocation.

Charge the following:

1. **Initial program footprint used by the VM:** estimate the program's retained constants and runtime metadata before executing the entry function, including string/array/struct/enum constants, function instruction tables, match tables, and relevant metadata. This admission-time estimate complements (but does not replace) bytecode loader/validator limits, which execute earlier.
2. **VM execution storage:** initial and grown operand-stack backing storage, each call-frame/local-slot allocation, and runtime bookkeeping allocated by the VM where it can be charged consistently.
3. **New string payloads:** UTF-8 payload bytes for runtime string concatenation and any other VM-created string payload. Copying an immutable Go string value between VM slots does not allocate or copy the underlying string and should not be charged as another payload.
4. **New aggregate storage:** backing storage for array literals/construction, slices, struct field arrays and enum payload arrays, using documented fixed header/element-size estimates and the allocated capacity where available.
5. **Copy-on-write detaches:** every new array backing store allocated to detach a shared container, including nested arrays detached along the mutation path. Do not charge an alias as if it had copied the backing payload; do charge a real copied backing store.
6. **Other VM-controlled buffers:** any additional buffer introduced by VM execution must either be accounted or explicitly documented as outside the first version before it ships.

The accounting model must use checked integer arithmetic. It must not wrap on large capacities, element counts, or repeated charges. For operations such as array copy, slice creation, and string concatenation, compute the intended charge and verify it before allocating the destination.

### Representation and copy-on-write

The meter does not attempt to determine when an object becomes unreachable and never decrements its total. An alias of an existing array header incurs only any separately allocated VM slot/frame storage; copying a shared array's backing slice during detachment is charged by the full destination storage size. A COW operation must not charge a child payload merely because a second header points to the same child. When detachment copies container headers or child values into a new outer slice, charge the new outer backing storage and separately charge any child payload that the operation actually duplicates.

The initial implementation may estimate Go object/header costs using constants derived from the supported target architecture, or use architecture-independent logical sizes for Aster value slots. The chosen formula must be explicit and stable enough for reproducible tests. It must not claim to account for allocator size classes, fragmentation, runtime metadata, GC overhead, or every hidden Go allocation.

## Failure behavior

- Add a stable VM diagnostic for cumulative managed-allocation budget exhaustion. Reserve a new runtime code only after the public diagnostic table and compatibility consequences are reviewed; do not overload RUN013, which is instruction/fuel exhaustion.
- Include the configured/default budget and the attempted charge in the diagnostic, without overflowing while formatting the error.
- Preserve source positions for instruction-triggered allocations when an instruction location is available. Admission-time program-footprint failure may use the existing VM/program-level diagnostic location.
- Do not convert unrelated allocation panics, OS OOM kills, or Native process exits into this diagnostic. The meter only reports its own pre-allocation budget rejection.
- A failed charge must leave the value graph and storage ownership unchanged. In particular, a rejected COW detach must happen before the backing slice is copied or mutated.

## Explicit non-goals and limitations

This budget is not:
- a precise measure of Go heap bytes or peak RSS;
- an OS-enforced process-memory limit;
- a hard guarantee against all host allocations or an operating-system OOM kill;
- a limit on compiler/source-analysis/build memory before VM startup;
- applicable to generated Native execution or standalone Native artifacts;
- a file/network/syscall/privilege sandbox.

The bytecode loader retains its separate input, JSON nesting, constant-pool, verifier, and metadata limits. Resource isolation for Native remains the subject of ADR 0010 and Issues #92/#93/#94.

## Rollout and acceptance plan

1. Review the default budget and the accounting formula with representative program footprints; record why the selected allowance is suitable for the prototype's current value representation.
2. Add an overflow-safe accountant and a central pre-allocation charge API before changing every allocation site.
3. Charge program constants/metadata during VM admission and charge runtime allocations at their actual creation sites.
4. Audit array construction, string concatenation, slicing, COW detachment, aggregate construction, stack growth, call-frame allocation, and output/display buffers. For each unmetered allocation, either add accounting or document the omission.
5. Add tests for just-under/at/over-budget cases, integer overflow in accounting, retained arrays, nested arrays, repeated COW detachments, slices, strings, function arguments/returns, constants, and rejected writes leaving data unchanged.
6. Add stress tests showing that cumulative allocation activity fails deterministically before the next over-budget allocation, regardless of garbage collection timing.
7. Run go test ./..., go test -race ./..., go vet ./..., examples, and Go 1.26.x/stable CI. Compare memory and runtime overhead with representative programs.
8. Update docs/security-model.md and the relevant runtime/diagnostic documentation to describe the implemented accounting formula and its blind spots before marking this ADR Accepted.

Until those acceptance steps are complete, the budget is a proposal, not implemented behavior or a security guarantee.

## Consequences

- Allocation-heavy VM programs have a deterministic cumulative allocation ceiling independent of Go GC timing.
- Workloads that repeatedly allocate short-lived values may be rejected despite a small live set; this is an intended implication of the monotonic budget.
- The implementation avoids unsafe assumptions about alias lifetime, but accounting remains an estimate and cannot replace OS memory controls.
- Array COW correctness and the new budget interact: the budget must charge detach copies without changing observable value semantics, and a failed budget check must occur before mutation.
