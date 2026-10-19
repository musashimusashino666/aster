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

## Design principles

- Prefer explicit rules over implicit conversions.
- Preserve source positions through parsing and compilation.
- Keep built-ins small; domain functionality belongs in libraries.
- New bytecode operations require validation and VM tests.
- Never claim a target is supported until CI actually builds and executes a representative program for it.
