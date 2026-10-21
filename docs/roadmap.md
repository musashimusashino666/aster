# Aster roadmap

This roadmap is a planning document, not a commitment that work is already in progress. Checkboxes describe the implementation and verification status in the current source tree; the language specification defines the exact supported subset, while the READMEs provide summaries.

## Phase 0.1 — core language prototype (current)

- [x] Lexer and source locations
- [x] Parser and AST
- [x] Static type checks for the implemented core
- [x] Bytecode compiler and VM
- [x] Native executable path through Go code generation
- [x] CLI, examples, regression tests, initial docs

## Phase 0.2 — correctness and developer experience

- [x] Bytecode control-flow, operand-stack, abstract-type, call-signature, and local-initialization verifier
- [x] Control-flow and operand-stack-height verifier (stack underflow, merge consistency, and termination stack state)
- [x] Fuzz targets for lexer, parser, bytecode loader, and bytecode instruction validation (broader sustained fuzz campaigns remain future work)
- [x] Expand differential testing between VM and native executable (fixed-seed generated-program coverage compares 64 typed expressions across arithmetic, booleans, strings, and arrays; broader property-based and fuzz campaigns remain future work)
- [x] Source-preserving formatter (aster fmt and aster fmt --check; original comments and string-literal spellings are retained)
- [x] Stable diagnostic codes for lexer, parser, and type-checker errors
- [x] Stable runtime, bytecode-reader/verifier, and native-toolchain diagnostic codes; source path and start line/column propagated for VM and Native runtime failures
- [x] Full end-to-end source spans including end offsets/columns for front-end, bytecode, and runtime diagnostics (lexer tokens, lexer/parser diagnostics, parsed AST nodes, shared typed IR, Bytecode v9 instructions, and CLI/standalone Native runtime diagnostics carry an exclusive range plus the legacy diagnostic anchor; CLI and standalone Native output render the syntactic range without changing `Diagnostic.Error()`; the start-position convention is specified and tested. Generic-expansion errors attach ranges and render the source path when their anchors map to source-backed AST metadata. Regression coverage includes function inference, struct/enum recursion, generic argument counts, enum payload inference, type references, and the position-only fallback for synthetic or unmapped locations)
- [x] Document current VM/Native resource guarantees, CLI deadlines, and non-guarantees in [ADR 0005](adr/0005-native-resource-guarantees.md)
- [x] Define and implement the deterministic Native fuel budget (100,000,000 units charged per executed statement, function entry, and loop iteration; see [ADR 0009](adr/0009-native-fuel-budget.md))
- [ ] Process-wide memory/CPU isolation with guarantees distinct from wrapper deadlines, Native fuel, and per-value limits (the context-deadline-race-fixed Linux cgroup v2 CLI implementation at `665c4401d5021d896a9bda0a7034fbda82107627` passed the strict delegated-hierarchy acceptance suite with zero skips on Windows 11 / WSL2, Ubuntu 26.04.1 LTS, kernel `6.18.54.1-microsoft-standard-WSL2`, Go 1.26.8. A 50,000 µs CPU-time budget produced 60,458 µs post-termination usage / 10,458 µs observed overshoot at 10 ms polling. Both new context-cancellation race tests, toolchain diagnostic precedence, descendant accounting after leader exit, local `oom_group_kill` classification, quota throttling, memory controls, process termination, and cleanup passed. `go test ./...` and targeted resource/toolchain race detector tests passed on this exact SHA. This validates only the recorded WSL2 setup, not bare-metal Linux or a worst-case bound. Windows/macOS backends, overhead benchmarking, and a standalone-artifact launcher remain pending; see [Issue #85](https://github.com/musashimusashino666/aster/issues/85) and [ADR 0010](adr/0010-process-resource-isolation.md).)
- [x] Return-flow analysis for literal-boolean `if` conditions and literal-true `while` loops (nonconstant loops remain conservative)
- [x] Benchmark harness with pinned VM workloads and reproducible methodology (numeric loops, recursion, function calls, array traversal, string/Unicode operations, and array formatting; end-to-end parse/build and process-memory harnesses remain future work)

## Phase 0.3 — practical language features

- [x] Dynamically sized arrays, type checking, and bounds-safe reads
- [x] Mutable array indexing with value semantics (checked nested assignment and copy-on-write path updates; follow [ADR 0003: Array value semantics](adr/0003-array-value-semantics.md))
- [x] Half-open array slices (`values[start:end]`) returning independent array values; start is inclusive and end is exclusive
- [x] Non-generic structs: nominal declarations, construction, read-only field selection, and VM/Native value-semantics parity (implemented by PR #61; direct field assignment, Struct display, and equality remain unsupported; see [ADR 0006](adr/0006-nominal-data-types-and-pattern-matching.md))
- [x] Enums with positional payloads and exhaustive flat `match` expressions, including generic enum monomorphization, payload type substitution/inference, concrete-instance exhaustiveness checks, Bytecode v7 verification, VM/Native COW parity, and malformed-bytecode/type-checking tests (see ADR 0006 and ADR 0007)
- [x] User-defined generics for functions, structs, and enums, including canonical instance caching and compile-time expansion limits (traits, bounds, and generic operator constraints remain unsupported)
- [x] Generic enum declarations support ordinary `Option<T>` / `Result<T, E>` definitions with exhaustive matching; they are user-defined types rather than compiler-specific built-ins
- [x] Explicit numeric casts with `i64(value)` / `f64(value)`, defined truncation and range rules, and VM/Native parity tests (implicit numeric promotion remains unsupported)
- [ ] Module/import system and package lockfile
- [ ] Standard library for paths, files, JSON, time, and command-line parsing
- [ ] Language Server Protocol implementation

## Phase 0.4 — backend and platform expansion

- [x] Typed intermediate representation shared by bytecode and native backends (checker exposes a source-spanned IR with resolved local/function identities and explicit checked operations; both native codegen and bytecode compilation consume it)
- [ ] Independent native backend using LLVM or Cranelift
- [ ] WebAssembly backend plus explicit host API bindings
- [ ] Cross-compilation and target capability manifests
- [ ] C ABI / FFI conventions and generated bindings

## Later domain packs

- [ ] Web services, HTTP, JSON and database connectors
- [ ] Array/tensor APIs and integration with established AI runtimes
- [ ] Game-loop, rendering and engine integration
- [ ] Android/iOS shared-logic package and native host bindings
- [ ] Embedded profile with configurable runtime, allocator, and startup code

Every roadmap item must include specification updates, tests, performance evidence where relevant, and an explicit support-level declaration.
