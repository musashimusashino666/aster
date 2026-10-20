# Aster

[日本語のREADMEはこちら](README.ja.md)。

**Aster** is an early-stage, statically typed programming language prototype intended to explore a consistent language core with multiple execution backends. The repository contains a working lexer, parser, static type checker, bytecode compiler/VM, a native backend through generated Go, a CLI, tests, and design documentation.

> **Status and scope:** Aster 0.1 is a functioning prototype, not a production-ready general-purpose language. It is not yet suitable for safety-critical systems or a replacement for Rust, C++, Python, TypeScript, Java, Swift, or Kotlin. The project explicitly documents incomplete areas instead of claiming full support.

## Design goals

- Static type checking with local type inference.
- A small, explicit core language and readable diagnostics.
- Runtime checks for integer overflow and invalid arithmetic.
- A portable bytecode format and a reference virtual machine.
- A separate native-build path, initially implemented by translating checked Aster programs to Go and invoking the Go toolchain.
- Behavior parity for the supported semantic subset, covered by shared regression tests; backend-specific resource limits and host-runtime constraints are documented separately.
- Clear boundaries between the language core and future domain libraries.

## Current implementation status

| Capability | Aster 0.1 status |
|---|---|
| Lexer and recursive-descent / Pratt parser | Implemented |
| Source formatter | Implemented; preserves comments and raw literal spellings |
| Static types (`i64`, `f64`, `bool`, `string`, `unit`) | Implemented |
| Explicit numeric casts (`i64(value)`, `f64(value)`) | Implemented; checked `f64`-to-`i64` truncation and range limits |
| Type inference for local variables | Implemented |
| Functions, recursion, conditionals, loops, return | Implemented |
| Immutable bindings by default; `let mut` for assignment | Implemented |
| Arrays (`Array<T>`), homogeneous literals, `len`, checked indexing and slicing | Implemented; `let mut` supports nested indexed assignment with value semantics and slices return independent array values |
| `Option`, `Result`, enums, structs, user generics | Planned; not implemented |
| Bytecode compiler and JSON bytecode file | Implemented |
| Bytecode structural, stack-height, abstract type, and local-initialization verification | Implemented |
| Bytecode VM with runtime checks and resource limits | Implemented |
| Native executable build | Implemented via Aster-to-Go code generation and host Go compiler |
| Cross-platform executable | Depends on the Go toolchain and its target support |
| WebAssembly target | Planned; not implemented |
| Standard library beyond `print`, `len`, `assert` | Not implemented |
| Imports, modules, package registry, async/await, closures | Planned; not implemented |
| Production security audit and benchmark suite | Not yet completed |

## Requirements

- Go 1.23 or newer for building the Aster CLI and native backend.
- Both `go` and `gofmt` must be available on `PATH`; `gofmt` is normally included with the Go distribution.
- No third-party Go modules are required by the current implementation.

## Build

From the repository root:

```bash
go test ./...
mkdir -p ./bin
go build -o ./bin/aster ./cmd/aster
```

On Windows, use an appropriate executable extension such as `aster.exe`.

## Quick start

Run a source file on the reference bytecode VM:

```bash
./bin/aster run examples/hello.ast
```

Run the same program through the native backend:

```bash
./bin/aster run --engine native examples/hello.ast
```

Check a file without running it:

```bash
./bin/aster check examples/fibonacci.ast
```

Format a source file in place, or check formatting without writing:

```bash
./bin/aster fmt examples/hello.ast
./bin/aster fmt --check examples/hello.ast
```

The formatter validates syntax before writing, preserves comments and original string-literal escapes, and uses four spaces for indentation.

Compile source to a versioned Aster bytecode JSON file and execute it:

```bash
./bin/aster compile -o fibonacci.abc.json examples/fibonacci.ast
./bin/aster exec fibonacci.abc.json
```

Build a native executable:

```bash
./bin/aster build -o ./fibonacci examples/fibonacci.ast
./fibonacci
```

The native build currently uses generated Go source and the installed Go compiler. Aster users do not need to edit generated source, but Go is a build-time dependency. The VM and native backend are distinct execution paths; they are not two independent language implementations. Both backends enforce function call-depth, string-size, array-size, and printed-output limits. The VM additionally enforces an instruction budget, operand-stack limit, and active-local-slot budget; these VM-specific budgets are not a process sandbox. Standalone native builds have a two-minute deadline; `aster run --engine native` has a five-minute total build-and-run deadline. Native programs run with the host process's privileges and are not a sandbox for untrusted source.

## Language tour

```aster
fn fibonacci(n: i64) -> i64 {
    if n <= 1 {
        return n;
    } else {
        return fibonacci(n - 1) + fibonacci(n - 2);
    }
}

let mut total: i64 = 0;
let mut index: i64 = 0;
while index < 10 {
    total = total + fibonacci(index);
    index = index + 1;
}
print(total);
```

### Types

- `i64`: signed 64-bit integer; overflow traps at runtime.
- `f64`: IEEE-754 64-bit floating point; non-finite arithmetic results are rejected by the current runtime.
- `bool`: `true` or `false`.
- `string`: UTF-8 text. `len(string)` counts Unicode code points, not bytes or grapheme clusters.
- `unit`: the return type of functions that do not return a value. It cannot be stored in a variable.
- `Array<T>`: dynamically sized homogeneous arrays; literals infer their element type, indexing is bounds-checked, and `len` returns element count. `let mut` bindings support checked element assignment, including nested paths; updates copy the indexed array path so copied values remain independent. Slices use the half-open form `values[start:end]`, with required `i64` bounds and an independent result; empty slices are valid. Empty array literals are not supported yet.

A variable may be annotated (`let score: i64 = 42;`) or inferred (`let score = 42;`). Bindings are immutable unless declared with `let mut`.

### Built-ins

- `print(value);` writes one value followed by a newline.
- `len(string)` returns the number of Unicode code points.
- `assert(condition);` terminates execution with an error if the condition is false.

### Operators

Unary: `-`, `!`

Arithmetic: `+`, `-`, `*`, `/`, `%`

Comparison: `==`, `!=`, `<`, `<=`, `>`, `>=`

Logical: `&&`, `||` (short-circuiting)

Numeric operands must have matching types; there is no implicit `i64`/`f64` conversion. Use `i64(value)` or `f64(value)` for explicit casts. `f64(i64_value)` follows IEEE-754 binary64 conversion and may lose precision; `i64(f64_value)` truncates toward zero and rejects values outside `[-2^63, 2^63)`. Array element assignment requires a mutable root binding, for example `let mut values = [1, 2]; values[0] = 9;`. Nested indexed assignments preserve array value semantics. Slicing uses `values[start:end]` with an inclusive start and exclusive end; both bounds are required and the result is an independent array value.

## CLI reference

```text
aster check <file.ast>
aster fmt [--check] <file.ast>
aster run [--engine vm|native] <file.ast>
aster compile [-o file.abc.json] <file.ast>
aster exec <file.abc.json>
aster build [--target native] [-o output] <file.ast>
aster version
```

Only `native` is a supported build target in this version. Unsupported targets fail clearly instead of silently producing a different artifact.

## Repository layout

```text
.
├── cmd/aster/           CLI entry point
├── internal/
│   ├── ast/              AST nodes
│   ├── bytecode/         bytecode format, verifier, and typed-IR compiler
│   ├── checker/          static type checker and typed-IR producer
│   ├── diagnostic/       stable front-end diagnostic codes
│   ├── codegen/          native Go backend
│   ├── lexer/            tokenization and source locations
│   ├── parser/           syntax parser
│   ├── token/            token definitions
│   ├── toolchain/        native build invocation
│   └── vm/               bytecode interpreter
├── docs/                 specification, architecture, security, roadmap
├── bench/                benchmark methodology
├── examples/             working Aster programs
├── stdlib/               standard-library design boundary
├── tests/                integration and regression tests
├── go.mod
├── Makefile
└── .github/workflows/    CI workflow
```

## Engineering policy

- Source must pass formatting and `go test ./...` before merge.
- Changes to language behavior require specification and regression-test updates.
- Bytecode format changes require a format-version increment when compatibility is broken.
- Native and VM behavior should be tested against the same examples, especially overflow, division by zero, branches, recursion, and short-circuit logic.
- Do not mark a backend or domain library as supported until CI exercises it.

See [`docs/diagnostics.md`](docs/diagnostics.md) for stable lexer, parser, and type-checker diagnostic codes.

## Local checks and CI

Run the same local checks used by the project automation:

```bash
make check
make examples
```

The GitHub Actions workflow at [`.github/workflows/ci.yml`](.github/workflows/ci.yml) checks Go formatting, runs the test suite and race detector, runs `go vet`, builds the CLI, and executes the examples on Go 1.23 and the latest stable Go release.

## Roadmap

See [`docs/roadmap.md`](docs/roadmap.md) for milestones. The bytecode verifier, shared typed-IR backend boundary, source formatter, and stable lexer/parser/type-checker diagnostic codes are implemented. Near-term work is focused on end-to-end source spans, continued fuzzing and differential testing, runtime/toolchain diagnostic codes, records and enums, `Option`/`Result`, and modules. A richer standard library and an official conformance suite follow as the core semantics stabilize. WebAssembly, AI runtime integration, and embedded profiles remain longer-term work.

## License

MIT; see [`LICENSE`](LICENSE).
