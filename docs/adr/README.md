# Architecture Decision Records

This directory is the canonical home for Aster Architecture Decision Records (ADRs). Each record captures the context, decision, and consequences of a design choice. Normative language behavior belongs in the relevant specification; ADRs explain why that design was chosen and preserve its history.

## Index

| ADR | Decision | Status | Date |
|---|---|---|---|
| [ADR 0001: Use generated Go as the first native backend](0001-native-backend.md) | Generated Go as the initial native backend | Accepted | 2026-10-19 |
| [ADR 0002: Checked arithmetic in the prototype](0002-arithmetic.md) | Checked arithmetic semantics | Accepted | 2026-10-19 |
| [ADR 0003: Array value semantics](0003-array-value-semantics.md) | Arrays have independent value semantics | Accepted | 2026-10-20 |
| [ADR 0004: Copy-on-write array storage](0004-array-copy-on-write.md) | Copy-on-write implementation for array mutation | Accepted | 2026-10-20 |
| [ADR 0005: Native resource guarantees and enforcement boundaries](0005-native-resource-guarantees.md) | Current VM/Native resource guarantees and enforcement boundaries | Accepted | 2026-10-20 |
| [ADR 0006: Nominal data types and pattern-matching rollout](0006-nominal-data-types-and-pattern-matching.md) | Staged rollout for structs, enums/match, generics, and Option/Result | Accepted | 2026-10-20 |
| [ADR 0007: User-defined generics and monomorphization](0007-user-defined-generics-and-monomorphization.md) | Generic syntax, inference, substitution, monomorphization, and resource bounds | Accepted | 2026-10-20 |

## Numbering and file names

- Assign the next unused positive integer and render it as four digits, starting with `0001`.
- Name each file `NNNN-short-kebab-title.md` and use the matching heading form `# ADR NNNN: Title`.
- Identifiers are permanent: do not reuse or renumber them, even if a record is moved or renamed. Gaps are acceptable.
- Keep all ADRs in `docs/adr/`. The former `docs/decisions/` directory is not used.

## Metadata and status

Every ADR must include a status and an ISO date (`YYYY-MM-DD`). The accepted status values are:

- **Proposed** — under discussion; not yet an adopted decision.
- **Accepted** — the decision is adopted.
- **Superseded** — a later ADR replaces this decision; link to the successor in this record.
- **Withdrawn** — the proposal was withdrawn without being adopted.

When one ADR supersedes another, the newer record must identify the prior ADR, and the prior record must point to the successor. Superseded and withdrawn records remain in the index as part of the project's decision history.

## References and maintenance

- Link related ADRs with relative Markdown links, using the file names listed in the index.
- Add every new ADR to this index in number order and keep its title, status, and date accurate.
- Update specifications, the roadmap, and contributor documentation when the decision changes implemented language behavior or development policy.
- Do not silently rewrite the decision in an accepted ADR. For a material reversal, write a new ADR and state what it supersedes.
