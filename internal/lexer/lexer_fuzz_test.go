package lexer

import "testing"

func FuzzLexerNoPanic(f *testing.F) {
	for _, seed := range []string{"", "let x = 1;", "/* nested /* x */ y */ print(\"ok\");", "fn f(a: Array<i64>) -> i64 { return a[0]; }", "\xff\xfe", "\\\"unterminated"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) { _, _ = New(source).Lex() })
}
