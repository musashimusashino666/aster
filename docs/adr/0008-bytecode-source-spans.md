# ADR 0008: Bytecode source spans and runtime diagnostics

- **Status:** Accepted
- **Date:** 2026-10-21
- **Relationship:** Extends the source-location work tracked in [the roadmap](../roadmap.md) and builds on the shared typed IR contract. It updates the bytecode wire format defined by the [language specification](../language-spec.md); it does not change Aster expression semantics.

## Context

Lexer tokens, front-end diagnostics, AST nodes, and the shared typed IR now retain exclusive end positions. The bytecode instruction schema previously retained only a start line and column, so source ranges were lost before VM execution. Without instruction spans, VM runtime diagnostics could not expose the complete source range of the operation that failed.

The bytecode reader rejects unknown JSON fields and unsupported format versions. Adding source-span fields is therefore a wire-format change and must not be silently interpreted as the previous format.

## Decision

### 1. Version the instruction schema

- Bump the bytecode format from version 7 to version 8.
- Version 8 adds optional instruction fields: `offset`, `end_offset`, `end_line`, and `end_column`.
- The existing `line` and `column` fields remain the start position used by legacy diagnostics. Their values and the rendered `Diagnostic.Error()` text do not change.
- The current loader accepts version 8 and rejects older bytecode artifacts, which do not declare the new source-span schema.

### 2. Define span semantics

- End positions are exclusive: the end identifies the first position after the source range.
- Offsets are zero-based UTF-8 byte offsets. Lines and columns are one-based, and columns count UTF-8 bytes, matching the current lexer contract.
- For an instruction emitted from a typed-IR node, the start position is that node's existing diagnostic anchor (`Pos()`) and the end position is its exclusive `EndPos()`. A legacy diagnostic anchor is not guaranteed to be the syntactic beginning of a compound expression.
- Synthetic instructions that have no source node may omit the span entirely. A partially populated span is invalid.
- The bytecode verifier rejects negative offsets, a missing end line/column, end offsets before start offsets, and end line/column positions before their starts.

### 3. Preserve spans at VM runtime failures

- The VM copies the active instruction's span into structured runtime `Diagnostic` fields when complete metadata is present.
- The Native runtime registers typed-IR source ranges by their existing start line/column anchors. When launched by `aster run --engine native`, it can emit a private structured stderr record that the toolchain wrapper consumes and converts into the same `Diagnostic` shape.
- If a programmatic or synthetic instruction has no span, the VM preserves the legacy start-only diagnostic path. Native instructions/helpers with no registered span likewise retain start-only diagnostics.
- Human-readable diagnostic text remains start-only for compatibility. Standalone Native executables do not use the CLI's private protocol by default; displaying end ranges remains deferred.

## Consequences

- Bytecode v8 makes source spans available after serialization and VM loading, while existing start-location text and diagnostic codes remain stable.
- The strict bytecode version check prevents older or partially populated artifacts from being misinterpreted as v8.
- Source locations still identify the operation's existing diagnostic anchor, not necessarily its complete syntactic left edge. Improving start-anchor semantics is separate from adding end positions.
- CLI-run Native runtime diagnostics now receive structured source spans through the toolchain's private stderr protocol. Standalone Native executables still use start-only diagnostic text, and the user-facing range renderer and consistent syntactic start-anchor semantics remain outstanding in the roadmap.

## Validation

- Test valid and malformed instruction spans, including partial metadata and reversed ranges.
- Round-trip span metadata through JSON bytecode serialization and the strict reader.
- Execute a failing operation through the VM and assert the structured diagnostic retains its expected start and exclusive end positions, diagnostic code, and legacy text.
- Run the full CI matrix on Go stable and Go 1.23.x, including race detection, static analysis, examples, and the 32-bit check.
