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
- [ ] Expand differential testing between VM and native executable (current tests cover selected outputs and runtime error classes; generated-program/property-based coverage remains future work)
- [ ] Formatter and stable diagnostic codes
- [ ] More exact return-flow analysis and error spans
- [ ] Benchmark harness with pinned workloads and reproducible methodology

## Phase 0.3 — practical language features

- [x] Dynamically sized arrays, type checking, and bounds-safe reads
- [ ] Mutable array indexing and slices (follow [ADR 0003: Array value semantics](adr/0003-array-value-semantics.md))
- [ ] Structs, enums, `Option<T>`, and `Result<T, E>`
- [ ] Explicit conversion syntax and well-defined numeric promotion rules
- [ ] Module/import system and package lockfile
- [ ] Standard library for paths, files, JSON, time, and command-line parsing
- [ ] Language Server Protocol implementation

## Phase 0.4 — backend and platform expansion

- [ ] Typed intermediate representation shared by bytecode and native backends (introduce incrementally, preserving source spans and explicit runtime semantics)
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
