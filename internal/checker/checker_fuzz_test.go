package checker

import (
	"testing"

	"github.com/aster-lang/aster/internal/parser"
)

// FuzzAnalyzeNoPanic exercises parsing, static checking, and typed-IR lowering
// together. Source that is rejected by the parser is outside the checker's
// input domain; accepted source must either produce typed IR or a diagnostic,
// never panic.
func FuzzAnalyzeNoPanic(f *testing.F) {
	for _, source := range []string{
		"",
		"print(1 + 2 * 3);",
		"fn identity(x: Array<i64>) -> Array<i64> { return x; }",
		"fn f(x: i64) -> i64 { if x > 0 { return x; } else { return -x; } }",
		"let mut values = [1, 2, 3]; values = [4, 5]; print(len(values));",
		"fn f() -> i64 { while true { } }",
		"fn f(x: i64) -> i64 { return x; } print(f());",
		"let values = [1, true];",
		"fn f() { let value = []; }",
		"fn nested(x: Array<Array<i64>>) -> Array<Array<i64>> { return x; }",
	} {
		f.Add(source)
	}

	f.Fuzz(func(t *testing.T, source string) {
		tree, err := parser.Parse(source, "fuzz.ast")
		if err != nil {
			return
		}
		_, _ = Analyze(tree)
	})
}
