# ADR 0008: Bytecode source spans and runtime diagnostics

- **Status:** Accepted
- **Date:** 2026-10-21
- **Relationship:** Extends the source-location work tracked in [the roadmap](../roadmap.md) and builds on the shared typed IR contract. It updates the bytecode wire format defined by the [language specification](../language-spec.md); it does not change Aster expression semantics.

## Context

Lexer tokens, front-end diagnostics, AST nodes, and the shared typed IR now retain exclusive end positions. The bytecode instruction schema previously retained only a start line and column, so source ranges were lost before VM execution. Without instruction spans, VM runtime diagnostics could not expose the complete source range of the operation that failed.

The bytecode reader rejects unknown JSON fields and unsupported format versions. Adding source-span fields is therefore a wire-format change and must not be silently interpreted as the previous format.

## Decision

### 1. Version the instruction schema

- Bump the bytecode format from version 8 to version 9.
- Version 8 added `offset`, `end_offset`, `end_line`, and `end_column`. Version 9 adds optional instruction fields `range_start_offset`, `range_start_line`, and `range_start_column` to distinguish the syntactic left edge from the legacy diagnostic anchor.
- The existing `line` and `column` fields remain the start position used by legacy diagnostics. Their values and the rendered `Diagnostic.Error()` text do not change.
- The current loader accepts version 9 and rejects older bytecode artifacts, which do not declare the source-range-start schema.

### 2. Define span semantics

- End positions are exclusive: the end identifies the first position after the source range.
- Offsets are zero-based UTF-8 byte offsets. Lines and columns are one-based, and columns count UTF-8 bytes, matching the current lexer contract.
- For an instruction emitted from a typed-IR node, `line`/`column` and `offset` remain its legacy diagnostic anchor (`Pos()`); `range_start_*` marks the syntactic left edge and `end_*` is exclusive. Compound expressions whose legacy anchor points at an operator or postfix delimiter now render from the left edge of the expression. Parentheses contribute their opening delimiter to the range when they group an expression.
- Synthetic instructions that have no source node may omit the span entirely. A partially populated span is invalid.
- The bytecode verifier rejects negative offsets, a missing end line/column, end offsets before start offsets, and end line/column positions before their starts.

### 3. Preserve spans at VM runtime failures

- The VM copies the active instruction's span into structured runtime `Diagnostic` fields when complete metadata is present.
- The Native runtime registers typed-IR source ranges by their existing start line/column anchors. When launched by `aster run --engine native`, it can emit a private structured stderr record that the toolchain wrapper consumes and converts into the same `Diagnostic` shape.
- If a programmatic or synthetic instruction has no span, the VM preserves the legacy start-only diagnostic path. Native instructions/helpers with no registered span likewise retain start-only diagnostics.
- The programmatic `Diagnostic.Error()` text remains start-only for compatibility. The CLI's explicit user-facing renderer displays a valid exclusive range when structured span metadata is available. Standalone Native executables do not use the CLI's private protocol by default and remain start-only.

## Consequences

- Bytecode v8 made exclusive end positions available after serialization and VM loading. Bytecode v9 adds the syntactic range start while preserving legacy diagnostic anchors and `Diagnostic.Error()` text.
- The strict bytecode version check prevents older or partially populated artifacts from being misinterpreted as v9.
- Structured diagnostics now distinguish the legacy operation anchor from the displayed source-range start; consumers can keep stable anchor semantics while rendering a syntactically complete range.
- CLI-run Native runtime diagnostics receive structured source spans through the toolchain's private stderr protocol, and the CLI renders their exclusive ranges without changing `Diagnostic.Error()`. Standalone Native executables display the syntactic source range when valid span metadata exists; the legacy diagnostic anchor remains unchanged in structured records.

## Validation

- Test valid and malformed instruction spans, including partial metadata and reversed ranges.
- Round-trip span metadata through JSON bytecode serialization and the strict reader.
- Execute a failing operation through the VM and assert the structured diagnostic retains its expected start and exclusive end positions, diagnostic code, and legacy text.
- Test user-facing range rendering for valid spans, wrapped diagnostics, and start-only fallback; verify diagnostic anchors and displayed range starts independently, and compare VM/Native diagnostics.
- Run the full CI matrix on Go 1.26.x (the minimum supported line) and stable, including race detection, static analysis, examples, and the 32-bit check.
