package parser

import "testing"

func FuzzParserNoPanic(f *testing.F) {
	for _, seed := range []string{"", "print(1 + 2 * 3);", "fn f(x: Array<i64>) -> i64 { return x[0]; }", "if true { let x = [1, 2]; }", "{", "))))[[["} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) { _, _ = Parse(source, "fuzz.ast") })
}
