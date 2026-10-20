package parser

import (
	"strings"
	"testing"
)

func FuzzParserNoPanic(f *testing.F) {
	for _, seed := range []string{"", "print(1 + 2 * 3);", "fn f(x: Array<i64>) -> i64 { return x[0]; }", "if true { let x = [1, 2]; }", "{", "))))[[[", "let x = " + strings.Repeat("(", maxSyntaxNestingDepth+1) + "1" + strings.Repeat(")", maxSyntaxNestingDepth+1) + ";", strings.Repeat("{", maxSyntaxNestingDepth+1) + strings.Repeat("}", maxSyntaxNestingDepth+1), "let x = " + strings.Repeat("1 + ", maxSyntaxNestingDepth+1) + "1;"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) { _, _ = Parse(source, "fuzz.ast") })
}
