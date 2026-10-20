# Aster roadmap

This roadmap is a planning document, not a commitment that work is already in progress.

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
- [ ] Full end-to-end source spans including end offsets/columns for front-end, bytecode, and runtime diagnostics
- [x] Document current VM/Native resource guarantees, CLI deadlines, and non-guarantees in [ADR 0005](adr/0005-native-resource-guarantees.md)
- [ ] Define and implement any required Native instruction/fuel budget and process-wide resource isolation, with guarantees distinct from wrapper deadlines and per-value limits
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
