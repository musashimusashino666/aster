# Changelog

## Unreleased

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

## 0.1.0 — initial prototype

- Added lexer, parser, AST, and static type checker.
- Added bytecode compiler and bounded stack-based VM.
- Added native executable generation through Go source lowering.
- Added CLI commands for check, run, compile, exec, build, and version.
- Added examples, language/architecture/security docs, CI, and initial regression tests.
- Documented unsupported language features and backend limitations.
