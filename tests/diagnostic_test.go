package tests

import (
	"errors"
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/checker"
	"github.com/aster-lang/aster/internal/diagnostic"
	"github.com/aster-lang/aster/internal/lexer"
	"github.com/aster-lang/aster/internal/parser"
)

func TestFrontendDiagnosticsExposeStableCodes(t *testing.T) {
	t.Run("lexer", func(t *testing.T) {
		_, err := lexer.New(string([]byte{0xff})).Lex()
		requireDiagnosticCode(t, err, diagnostic.LexerInvalidUTF8, "lexer error:")
	})
	t.Run("parser", func(t *testing.T) {
		_, err := parser.Parse("let value = ;", "diagnostic.ast")
		requireDiagnosticCode(t, err, diagnostic.ParserExpectedExpression, "parser error:")
	})
	t.Run("type checker", func(t *testing.T) {
		tree, err := parser.Parse(`let mut value: i64 = "wrong";`, "diagnostic.ast")
		if err != nil {
			t.Fatal(err)
		}
		_, err = checker.Analyze(tree)
		requireDiagnosticCode(t, err, diagnostic.TypeBindingTypeMismatch, "type error:")
	})
}

func requireDiagnosticCode(t *testing.T, err error, wantCode, wantPrefix string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected diagnostic error")
	}
	var got *diagnostic.Diagnostic
	if !errors.As(err, &got) {
		t.Fatalf("error type = %T, want *diagnostic.Diagnostic: %v", err, err)
	}
	if got.Code != wantCode {
		t.Fatalf("diagnostic code = %q, want %q", got.Code, wantCode)
	}
	if !strings.Contains(err.Error(), wantPrefix) || !strings.HasSuffix(err.Error(), "["+wantCode+"]") {
		t.Fatalf("formatted diagnostic %q must preserve prefix %q and end with [%s]", err, wantPrefix, wantCode)
	}
}
