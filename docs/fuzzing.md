# Fuzzing and differential tests

Aster keeps deterministic fuzz seeds in the Go test packages so ordinary CI can
exercise the targets without a long runtime. A separate GitHub Actions workflow
provides sustained runs on a schedule.

## Scheduled campaign

`.github/workflows/fuzz.yml` runs each target for 10 minutes every Monday at
04:23 UTC. The workflow can also be started manually, with a per-target duration
of 5, 10, or 20 minutes. Targets run as independent jobs so one crash does not
prevent the remaining targets from running.

| Area | Target | Coverage |
|---|---|---|
| Lexer | `FuzzLexerNoPanic` | Arbitrary source strings and tokenization |
| Parser | `FuzzParserNoPanic` | Syntax parsing, including malformed and deeply nested source |
| Type checker | `FuzzAnalyzeNoPanic` | Accepted ASTs through static analysis and typed-IR lowering |
| Bytecode reader | `FuzzReadBytecodeNeverPanics` | Untrusted JSON bytecode input and decode limits |
| Bytecode verifier | `FuzzValidateInstructionOperandsNeverPanics` | Generated instruction opcodes and operand ranges |
| Structured bytecode verifier | `FuzzValidateStructuredProgramsNeverPanics` | Bounded generated programs with functions, constants, control flow, types, and instruction sequences |

A failing run uploads any persisted Go fuzz corpus as an artifact for 14 days.
When a failure is found, preserve the minimized input under the corresponding
package's `testdata/fuzz/<TargetName>/` directory and add a focused regression
test when practical.

## Run locally

Run a single target for 10 minutes:

```sh
go test -run=^$ -fuzz=^FuzzLexerNoPanic$ -fuzztime=10m ./internal/lexer
go test -run=^$ -fuzz=^FuzzParserNoPanic$ -fuzztime=10m ./internal/parser
go test -run=^$ -fuzz=^FuzzAnalyzeNoPanic$ -fuzztime=10m ./internal/checker
go test -run=^$ -fuzz=^FuzzReadBytecodeNeverPanics$ -fuzztime=10m ./internal/bytecode
go test -run=^$ -fuzz=^FuzzValidateInstructionOperandsNeverPanics$ -fuzztime=10m ./internal/bytecode
go test -run=^$ -fuzz=^FuzzValidateStructuredProgramsNeverPanics$ -fuzztime=10m ./internal/bytecode
```

Run deterministic VM/Native output and runtime-failure comparisons:

```sh
go test ./tests -run='TestSeeded(Array|Scalar)ProgramsMatchBackends|TestSeededRuntimeFailureClassAndExitStatusMatch'
```

These differential tests print the seed and generated source on failure. The
same seed always produces the same program, allowing a regression to be replayed
locally without waiting for the scheduled campaign.
