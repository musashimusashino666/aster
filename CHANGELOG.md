# Changelog

## [Unreleased]

## [0.1.0] - 2026-10-21

- Documented the release/tagging process, strict bytecode-format compatibility, and the Go 1.26+ support policy.

- Include the original source path when rendering generic-expansion diagnostics with validated source ranges, without changing the legacy `Diagnostic.Error()` text.

- Added structured source ranges to generic-expansion diagnostics when their legacy anchors map to source-backed AST nodes, while preserving stable diagnostic anchors and the existing `Diagnostic.Error()` text.
- Defined a separate syntactic source-range start for compound expressions, propagated it through typed IR, bytecode v9, VM and Native diagnostics, and rendered ranges in standalone Native executables while keeping legacy diagnostic anchors stable.

- Added CLI rendering of validated exclusive source ranges while preserving the legacy programmatic `Diagnostic.Error()` format; added regression coverage for wrapped diagnostics and invalid-span fallback.

- Added explicit `i64(value)` and `f64(value)` casts, documented truncation/range semantics, and VM/Native parity coverage; bumped JSON bytecode format to v5 for `TO_I64` and `TO_F64`.

- Added mutable nested array-element assignment with copy-on-write path updates and VM/Native parity coverage; bumped JSON bytecode format to v3 for `SET_INDEX_PATH`.
- Added checked half-open array slicing with independent result values and VM/Native parity coverage; bumped JSON bytecode format to v4 for `SLICE`.

- Improved return-flow analysis for literal-boolean branches and literal-true loops; both backends lower constant conditions consistently.

- Added abstract bytecode type verification for stack operations, function calls, returns, homogeneous arrays, and definite local initialization; bumped JSON bytecode format to v2 to carry parameter types.
- Fixed native code generation for annotated array types and non-call expression statements.
- Rejected invalid `unit` bindings and print arguments, reserved compiler-internal function names, and improved integer-boundary and unsupported array-assignment diagnostics.
- Normalized CLI value flags after positional arguments, with support for both single- and double-dash `engine` spelling.
- Added bytecode control-flow and operand-stack-height verification, including stack underflow and malformed termination-state checks.
- Expanded regression coverage for VM/native parity and malformed bytecode; added fuzz targets for bytecode loading and validation and an example-to-bytecode verification test.
- Restored generated-artifact ignore rules and pinned GitHub Actions dependencies to immutable commit SHAs.

### Initial prototype capabilities

- Added the lexer, parser, AST, source formatter, static type checker, and typed intermediate representation.
- Added the statically typed language core: functions, recursion, conditionals, loops, arrays, structs, enums, flat exhaustive `match`, and user-defined generic functions, structs, and enums.
- Added versioned JSON bytecode compilation, strict bytecode verification, and a bounded stack-based virtual machine.
- Added native executable generation by lowering supported Aster code to Go and invoking the host Go toolchain.
- Added CLI commands for `check`, `fmt`, `run`, `compile`, `exec`, `build`, and `version`, with examples and regression, differential, and fuzz coverage.
- Added language, architecture, diagnostics, resource-limit, security, and roadmap documentation, plus GitHub Actions CI.
- Documented unsupported language features, resource-budget distinctions, and Native execution's lack of a general-purpose sandbox.
