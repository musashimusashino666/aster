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

func TestTokenEndPositionsAreExclusive(t *testing.T) {
	source := "let value = 12;\nprint(\"a\\nλ\");"
	got, err := New(source).Lex()
	if err != nil {
		t.Fatalf("lex source: %v", err)
	}
	if len(got) == 0 || got[len(got)-1].Kind != token.EOF {
		t.Fatalf("expected EOF token, got %#v", got)
	}

	for i, tok := range got {
		if tok.End.Offset < tok.Pos.Offset {
			t.Errorf("token %d has reversed offsets: start=%+v end=%+v", i, tok.Pos, tok.End)
			continue
		}
		if tok.Kind == token.EOF {
			if tok.Pos != tok.End || tok.Pos.Offset != len(source) {
				t.Errorf("EOF must have a zero-width span at end of source: pos=%+v end=%+v", tok.Pos, tok.End)
			}
			continue
		}
		if tok.Pos.Offset < 0 || tok.End.Offset > len(source) {
			t.Errorf("token %d span falls outside source: start=%+v end=%+v", i, tok.Pos, tok.End)
			continue
		}
		raw := source[tok.Pos.Offset:tok.End.Offset]
		if raw != tok.Raw {
			t.Errorf("token %d (%s) source slice = %q, raw = %q", i, tok.Kind, raw, tok.Raw)
		}
		if tok.End.Offset != tok.Pos.Offset+len(tok.Raw) {
			t.Errorf("token %d end offset = %d, want exclusive byte offset %d", i, tok.End.Offset, tok.Pos.Offset+len(tok.Raw))
		}
	}

	if got[0].Pos != (token.Position{Offset: 0, Line: 1, Column: 1}) ||
		got[0].End != (token.Position{Offset: 3, Line: 1, Column: 4}) {
		t.Fatalf("keyword span = %+v..%+v, want 0:1:1..3:1:4", got[0].Pos, got[0].End)
	}
	for _, tok := range got {
		if tok.Kind == token.Ident && tok.Lexeme == "print" {
			if tok.Pos.Line != 2 || tok.Pos.Column != 1 || tok.End.Line != 2 || tok.End.Column != 6 {
				t.Fatalf("print span = %+v..%+v, want line 2 columns 1..6", tok.Pos, tok.End)
			}
			return
		}
	}
	t.Fatal("print identifier token not found")
}
