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
| Static types (`i64`, `f64`, `bool`, `string`, `unit`) | Implemented |
| Type inference for local variables | Implemented |
| Functions, recursion, conditionals, loops, return | Implemented |
| Immutable bindings by default; `let mut` for assignment | Implemented |
| Arrays (`Array<T>`), homogeneous literals, `len`, checked indexing | Implemented; array elements are currently read-only |
| `Option`, `Result`, enums, structs, user generics | Planned; not implemented |
| Bytecode compiler and JSON bytecode file | Implemented |
| Bytecode VM with runtime checks and resource limits | Implemented |
| Native executable build | Implemented via Aster-to-Go code generation and host Go compiler |
| Cross-platform executable | Depends on the Go toolchain and its target support |
| WebAssembly target | Planned; not implemented |
| Standard library beyond `print`, `len`, `assert` | Not implemented |
| Imports, modules, package registry, async/await, closures | Planned; not implemented |
| Production security audit and benchmark suite | Not yet completed |

## Requirements

- Go 1.23 or newer for building the Aster CLI and native backend.
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

The native build currently uses a generated Go source file and the installed Go compiler. Aster users do not need to edit generated source, but Go is a build-time dependency. The VM and native backend are distinct execution paths; they are not two independent language implementations. VM instruction/call-depth limits do not currently apply to generated native executables; native programs run with the host process's privileges.

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
- `Array<T>`: dynamically sized homogeneous arrays; literals infer their element type, indexing is bounds-checked, and `len` returns element count. Empty literals and element assignment are not supported yet.

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

Numeric operands must have matching types; there is no implicit `i64`/`f64` conversion in 0.1. Arrays are currently read-only by element; `let mut` can reassign the binding, but `values[0] = ...` is not yet supported.

## CLI reference

```text
aster check <file.ast>
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
│   ├── bytecode/         bytecode format and AST-to-bytecode compiler
│   ├── checker/          static type checker
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

## Local checks and CI

Run the same local checks used by the project automation:

```bash
make check
make examples
```

The GitHub Actions workflow at [`.github/workflows/ci.yml`](.github/workflows/ci.yml) checks Go formatting, runs the test suite and race detector, runs `go vet`, builds the CLI, and executes the examples on Go 1.23 and the latest stable Go release.

## Roadmap

See [`docs/roadmap.md`](docs/roadmap.md) for milestones. The next priorities are stronger bytecode verification, a richer standard library, mutable array indexing, records, a formatter, modules, and an official conformance suite. WebAssembly, AI runtime integration, and embedded profiles remain longer-term work.

## License

MIT; see [`LICENSE.txt`](LICENSE.txt).
