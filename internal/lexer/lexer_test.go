package lexer

import (
	"testing"

	"github.com/aster-lang/aster/internal/token"
)

func TestLexKeywordsNumbersStringsAndOperators(t *testing.T) {
	src := `fn mainx() { let mut n: i64 = 12; n = n + 0.5; print("a\n\"b"); if true && !false { return; } }`
	got, err := New(src).Lex()
	if err != nil {
		t.Fatal(err)
	}
	want := []token.Kind{token.Fn, token.Ident, token.LParen, token.RParen, token.LBrace, token.Let, token.Mut, token.Ident, token.Colon, token.Ident, token.Assign, token.Int, token.Semicolon, token.Ident, token.Assign, token.Ident, token.Plus, token.Float, token.Semicolon, token.Ident, token.LParen, token.String, token.RParen, token.Semicolon, token.If, token.True, token.AndAnd, token.Bang, token.False, token.LBrace, token.Return, token.Semicolon, token.RBrace, token.RBrace, token.EOF}
	if len(got) != len(want) {
		t.Fatalf("got %d tokens, want %d", len(got), len(want))
	}
	for i, k := range want {
		if got[i].Kind != k {
			t.Errorf("token %d = %s, want %s", i, got[i].Kind, k)
		}
	}
	if got[21].Lexeme != "a\n\"b" {
		t.Fatalf("string escape decoding: got %q", got[21].Lexeme)
	}
}
func TestNestedBlockComments(t *testing.T) {
	got, err := New("/* outer /* nested */ comment */ let x = 1;").Lex()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 2 || got[0].Kind != token.Let {
		t.Fatalf("unexpected token stream: %#v", got)
	}
}
func TestUnterminatedBlockComment(t *testing.T) {
	if _, err := New("/* never closed").Lex(); err == nil {
		t.Fatal("expected unterminated comment error")
	}
}
func TestInvalidCharacter(t *testing.T) {
	if _, err := New("let x = @;").Lex(); err == nil {
		t.Fatal("expected invalid character error")
	}
}
