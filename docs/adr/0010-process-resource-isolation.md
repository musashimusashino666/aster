# ADR 0010: Process-wide Native resource isolation

- **Status:** Proposed
- **Date:** 2026-10-21
- **Relationship:** Complements [ADR 0005: Native resource guarantees](0005-native-resource-guarantees.md) and [ADR 0009: Deterministic Native fuel budget](0009-native-fuel-budget.md). This proposal does not change Aster language semantics; the initial opt-in Linux implementation is tracked separately and does not imply cross-platform support.

## Context

Aster has several distinct resource controls. They are not interchangeable:

- The VM instruction-step budget and the Native fuel budget bound different language-level work counters.
- The existing five-minute deadline for `aster run --engine native` is a CLI wall-clock deadline spanning analysis, build, and execution. It is not embedded in a standalone executable.
- Per-string, per-array, rendered-value, and cumulative-output limits restrict particular values or output, not all memory held by a process.
- An operating-system resource controller can constrain a workload process or process tree, but the metrics and guarantees depend on the OS and available privileges.

A per-value limit or deterministic fuel budget does not bound total process memory. A process-wide CPU rate limit also differs from a cumulative CPU-time budget and from a wall-clock timeout. The design must report these as separate guarantees and must not call resource isolation a sandbox.

## Proposed decision

### 1. Define each resource by its metric and enforcement point

| Control | Defined metric | Intended guarantee | Does not mean |
|---|---|---|---|
| Native fuel | Count of charged IR statements, function entries, and loop iterations | Deterministic bound on the specified language-level events; exhaustion is RUN013 | CPU seconds, memory ceiling, or sandbox |
| Wall-clock deadline | Elapsed real time measured by a supervisor | Supervisor requests termination when the deadline expires; process-tree termination depends on the OS backend | CPU-time budget or hard memory cap |
| CPU rate limit | Maximum scheduled CPU throughput for the workload, expressed as a fraction of one logical CPU | OS throttles the workload above the configured rate | Cumulative CPU-time limit or wall-clock deadline |
| Cumulative CPU-time budget | Aggregated CPU time for the workload process tree, with the OS-specific accounting metric reported | Supervisor stops the workload after the observed aggregate reaches its threshold; any measurement interval and overshoot must be documented | Exact instruction count or necessarily a hard per-process kernel-enforced limit |
| Process memory limit | OS-accounted memory charged to the workload process tree | OS-backed containment at the configured accounting boundary, with platform-specific behavior and possible OOM termination | Exact language-heap bytes, a guarantee that instantaneous usage never briefly exceeds the configured figure, or sandboxing |

The selected backend must expose which of these controls it actually enforces. The CLI must not describe an unavailable or monitor-only capability as a hard OS-enforced limit.

### 2. Scope and enforcement contract

- Treat the generated Native program and any descendants it starts as one workload. The supervisor and the host Go compiler are outside the runtime memory/CPU accounting boundary in the first implementation stage; the existing build deadline remains separate.
- Apply resource controls before user program code starts. Assigning a process to a job or control group after it begins executing leaves a race and is not acceptable for a hard-limit claim.
- Make constrained execution an explicit resource profile in the first rollout. If that profile is requested but a required hard limit cannot be installed, fail before running user code with a structured “resource limit unavailable” diagnostic. Do not silently fall back to unrestricted execution.
- Do not change existing default execution behavior until platform support, overhead, compatibility, and default thresholds have been measured and reviewed. Initial defaults should be chosen from workload measurements, not guessed in this ADR.
- The first implementation stage should target supervised Native runtime execution through the CLI. A standalone binary produced by a build command must not be advertised as process-isolated until its startup path can establish the same OS boundary before user code begins. The packaging/supervisor strategy for standalone artifacts remains an explicit implementation gate.
- Resource isolation is not filesystem, network, syscall, or privilege isolation. Native programs continue to run with the caller's privileges unless a separate sandbox design is adopted.

### 3. Platform strategy and constraints

**Linux**

- Prefer a dedicated cgroup v2 for each workload when the caller has permission to create or use a delegated cgroup.
- Use memory.max for the cgroup RAM-accounting boundary and set memory.swap.max to 0 so the workload cannot extend the budget with cgroup-accounted swap. Use cpu.max for CPU throughput and cpu.stat for measured aggregate CPU usage. Configure and inspect the available controller files before launch; if memory.swap.max cannot be set, fail closed.
- Treat memory.max as an OS memory-accounting limit, not as a promise that observed usage can never be temporarily above the configured value. The kernel may invoke OOM handling when memory cannot be reclaimed. Confirm a cgroup OOM event before classifying a termination as a configured memory-limit failure.
- Enforce a cumulative CPU-time budget by monitoring the cgroup's measured usage and terminating the whole workload when the threshold is reached. The polling interval and resulting maximum observed overshoot must be measured and documented.
- If cgroup v2 or required controller delegation is unavailable, constrained mode must fail closed rather than claim that RLIMIT_AS or a language allocation counter provides an equivalent Go-process memory cap.

**Windows**

- Use a Windows Job Object to group the target process and descendants, apply a job-wide memory limit, and enable CPU-rate hard-cap controls where requested.
- Create and configure the job and arrange for the child to be assigned before any Aster program code runs. Validate job memory accounting and CPU-time semantics separately: a Job Object's user-mode job-time limit is not automatically identical to aggregate user-plus-kernel CPU time.
- If a requested aggregate metric or termination cause cannot be observed reliably on the supported Windows version, report that capability as unavailable instead of inferring it from an exit code alone.

**macOS**

- setrlimit provides per-process resource limits such as RLIMIT_CPU, but these controls do not by themselves establish a comprehensive memory ceiling over a process tree. A data-segment or legacy resident-set limit must not be described as a portable whole-process memory quota.
- The first implementation must report hard process-tree memory isolation as unsupported on macOS unless a separately validated outer supervisor or virtualized/containerized boundary supplies that guarantee.
- A wall-clock supervisor may terminate a process group, but process-group cleanup alone is not equivalent to a hard resource or security boundary.

### 4. Diagnostics and observability

- Keep RUN013 exclusively for Native fuel exhaustion. Do not reuse it for OS memory or CPU enforcement.
- The initial Linux implementation uses RUN018 only when both the cgroup `memory.events` `oom` and `oom_kill` counters increase during the workload. It sets `memory.oom.group=1` so a memcg OOM treats the workload as indivisible. RUN019 is used when measured aggregate `cpu.stat` usage reaches the configured CPU-time budget. Do not assign either code based solely on an exit status.
- Keep the existing CLI wall-clock timeout classification separate (currently TOOL002). A CPU-rate cap is throttling, not an exhaustion diagnostic.
- Structured resource diagnostics should include the resource type, configured threshold, observed OS metric when available, enforcement backend, termination cause, and source path when known.
- A process-wide limit is not reliably attributable to one Aster expression. Do not fabricate a source line or span for an OS/supervisor event. Preserve a source location only when the runtime can establish that the location is causally valid.
- Only report a configured memory-limit failure when the supervisor has evidence from the OS controller (for example, a relevant cgroup OOM event or job notification). An unexplained SIGKILL, host OOM, or abnormal exit must not be mislabeled as a confirmed Aster limit violation.

### 5. Validation and adversarial tests

- Unit-test resource-option validation, unsupported-capability handling, structured diagnostic encoding, metric units, and the rule that constrained mode never silently degrades.
- Use a dedicated helper workload that can allocate and retain memory, burn CPU, spawn descendants, and ignore ordinary graceful shutdown. It must exercise OS enforcement independently of the Aster fuel counter.
- On Linux, run integration tests in a delegated cgroup v2 where available. Verify the configured memory accounting boundary, CPU rate throttling, cumulative CPU measurement and overshoot, OOM-event classification, complete process-tree termination, and cgroup cleanup.
- On Windows, verify that the Job Object is installed before child execution, that descendants inherit the job, that memory/CPU limits are observed, and that cancellation terminates the job's processes.
- On macOS, verify that unsupported hard-memory requests fail closed and that any supported wall-clock/process-group behavior is reported only at the strength it actually provides.
- A platform test skipped because the runner lacks required permissions or controllers is not a passing isolation test. CI must report unsupported/skipped capability explicitly.
- Benchmark startup latency, runtime overhead, controller polling interval, cleanup latency, and measured CPU-time overshoot. Record OS and kernel/runtime versions with results.

## Initial Linux implementation scope

The first opt-in CLI implementation is intentionally narrower than the complete proposal:

- `aster run --engine native --resource-profile constrained` requires at least one explicit `--memory-limit`, `--cpu-rate`, or `--cpu-time-limit` value. Normal Native execution and standalone artifacts remain unchanged.
- Linux requires a delegated cgroup v2 directory where each requested controller is already enabled for child cgroups and `cgroup.kill` is available. `ASTER_CGROUP_ROOT` can select the delegated root; no controller is enabled or host hierarchy modified automatically.
- The CLI starts its internal launcher, moves that PID into the cgroup, and releases it to `exec` the Native artifact only after membership succeeds. User Aster code therefore cannot run before the boundary is installed.
- Memory uses `memory.max` plus `memory.swap.max=0`. CPU rate uses `cpu.max`; cumulative CPU time is monitored from `cpu.stat` at a 10 ms polling interval, so overshoot is possible and must be measured.
- A confirmed increase in cgroup `memory.events:oom_kill` maps to RUN018. CPU-time enforcement maps to RUN019. Missing controls, inability to establish the cgroup, or cleanup failure fail closed; those failures do not claim the requested limit was enforced.
- Windows and macOS implementations, CI integration that grants OS-controller capabilities, resource overhead/overshoot benchmarks, and isolated standalone artifacts remain outstanding. Until those are validated, no cross-platform process-isolation guarantee is made.

## Staged rollout

1. Review this ADR and settle the public configuration surface and initial measured thresholds.
2. Implement the capability model and Linux cgroup v2 backend behind explicit constrained mode, including adversarial integration tests.
3. Add a Windows Job Object backend and platform-specific CI.
4. Keep hard process-tree memory isolation unavailable on macOS until a suitable outer boundary is validated.
5. Integrate a standalone-artifact launcher only after it can install the OS boundary before executing user code; until then document standalone Native fuel and existing value limits without claiming process-wide isolation.

## References

- [Linux kernel documentation: Control Group v2](https://www.kernel.org/doc/html/v6.6/admin-guide/cgroup-v2.html)
- [Microsoft Learn: Job Objects](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects)
- [Microsoft Learn: Job Object CPU rate control](https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-jobobject_cpu_rate_control_information)
- [Apple: getrlimit and setrlimit](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/getrlimit.2.html)
- [Go: os/exec CommandContext](https://pkg.go.dev/os/exec#CommandContext)

## Open questions

- What API/CLI configuration surface should enable constrained execution, and which thresholds are justified by end-to-end measurements?
- Should the standalone Native artifact be a self-supervising launcher, a paired executable plus helper, or remain explicitly outside the isolation contract on hosts without a delegated OS controller?
- What is the project's minimum supported platform matrix for making a hard process-memory guarantee?
- Does the project want only resource isolation, or a future separate sandbox for filesystem, network, process, and syscall access?
