# Contributing

## Development loop

```bash
gofmt -w .
go test ./...
go vet ./...
go build ./cmd/aster
```

## Language changes

For a syntax or semantic change:

1. Update `docs/language-spec.md` first or in the same change.
2. Add lexer/parser/checker tests for valid and invalid cases.
3. Update bytecode compilation and both execution paths.
4. Add parity tests for arithmetic, error cases, and output.
5. Update README feature status and roadmap.

## Architectural decision records (ADRs)

- Store all decision records in `docs/adr/`; `docs/decisions/` is not a canonical location.
- Allocate the next unused four-digit identifier. Use `NNNN-short-kebab-title.md` and the heading `ADR NNNN: Title`.
- Never reuse or renumber an identifier, including when a record is moved or its title changes.
- Include **Status** and **Date** metadata. Use one of `Proposed`, `Accepted`, `Superseded`, or `Withdrawn` as the status.
- Add each record to [the ADR index](adr/README.md), and use relative Markdown links when referencing other ADRs.
- Treat accepted records as durable design history. A material reversal should be captured in a new ADR that identifies the record it supersedes; do not silently rewrite the original decision.

## Design principles

- Prefer explicit rules over implicit conversions.
- Preserve source positions through parsing and compilation.
- Keep built-ins small; domain functionality belongs in libraries.
- New bytecode operations require validation and VM tests.
- Never claim a target is supported until CI actually builds and executes a representative program for it.
