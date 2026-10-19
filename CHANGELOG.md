# Changelog

## Unreleased

- Fixed native code generation for annotated array types and non-call expression statements.
- Rejected invalid `unit` bindings and print arguments, reserved compiler-internal function names, and improved integer-boundary and unsupported array-assignment diagnostics.
- Normalized CLI value flags after positional arguments, with support for both single- and double-dash `engine` spelling.
- Added bytecode control-flow and operand-stack-height verification, including stack underflow and malformed termination-state checks.
- Expanded regression coverage for VM/native parity and malformed bytecode; updated implementation and security documentation.
- Restored generated-artifact ignore rules and pinned GitHub Actions dependencies to immutable commit SHAs.

## Unreleased

- Fixed native code generation for annotated array types and non-call expression statements.
- Rejected invalid `unit` bindings and print arguments, reserved compiler-internal function names, and improved integer-boundary and unsupported array-assignment diagnostics.
- Normalized CLI value flags after positional arguments, with support for both single- and double-dash `engine` spelling.
- Added bytecode control-flow and operand-stack-height verification, including stack underflow and malformed termination-state checks.
- Expanded regression coverage for VM/native parity and malformed bytecode; updated implementation and security documentation.
- Restored generated-artifact ignore rules and pinned GitHub Actions dependencies to immutable commit SHAs.

## 0.1.0 — initial prototype

- Added lexer, parser, AST, and static type checker.
- Added bytecode compiler and bounded stack-based VM.
- Added native executable generation through Go source lowering.
- Added CLI commands for check, run, compile, exec, build, and version.
- Added examples, language/architecture/security docs, CI, and initial regression tests.
- Documented unsupported language features and backend limitations.
