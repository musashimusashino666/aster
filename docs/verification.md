# Verification record for Aster 0.1.0

The source archive was checked in a Linux amd64 environment with Go 1.23.x.

Verified commands:

```bash
gofmt -w .
go test ./...
go vet ./...
go test -race ./...
make check
make examples
go build -trimpath -o /tmp/aster-final ./cmd/aster
/tmp/aster-final check examples/arrays.ast
/tmp/aster-final compile -o /tmp/aster-final-arrays.abc.json examples/arrays.ast
/tmp/aster-final exec /tmp/aster-final-arrays.abc.json
/tmp/aster-final build -o /tmp/aster-final-native examples/arrays.ast
/tmp/aster-final-native
/tmp/aster-final run --engine native examples/arrays.ast
```

The bytecode VM, saved bytecode execution, and generated native executable produced matching output for the array example. Automated tests also cover parser/type-checker errors, recursive calls, loops, short-circuit evaluation, array bounds, signed integer overflow, bytecode structure checks, and VM/native output parity.

This record is limited to the commands above and the implemented subset. It is not a certification, security audit, cross-platform test report, or claim of production readiness.
