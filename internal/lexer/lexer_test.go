package lexer

import (
	"strings"
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

func TestLexIgnoresLeadingUTF8BOM(t *testing.T) {
	got, err := New("\uFEFFlet value = 1;").Lex()
	if err != nil {
		t.Fatalf("lexing source with leading UTF-8 BOM failed: %v", err)
	}
	if len(got) < 2 || got[0].Kind != token.Let || got[0].Pos.Offset != len("\uFEFF") {
		t.Fatalf("unexpected token stream after BOM: %#v", got)
	}
}

func TestUnexpectedUnicodeCharacterReportsCodePoint(t *testing.T) {
	_, err := New("let x = ；;").Lex()
	if err == nil {
		t.Fatal("expected invalid Unicode character error")
	}
	if !strings.Contains(err.Error(), "U+FF1B") {
		t.Fatalf("diagnostic should identify Unicode code point U+FF1B, got %q", err)
	}
}

func TestLexSkipsUnicodeWhitespace(t *testing.T) {
	got, err := New("　let value = 1;").Lex()
	if err != nil {
		t.Fatalf("lexing after Unicode whitespace failed: %v", err)
	}
	if len(got) < 2 || got[0].Kind != token.Let || got[0].Pos.Offset != len("　") {
		t.Fatalf("unexpected token stream after Unicode whitespace: %#v", got)
	}
}
