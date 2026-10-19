package parser

import (
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/ast"
)

func TestParsesPrecedence(t *testing.T) {
	p, err := Parse("let result = 1 + 2 * 3;", "test.ast")
	if err != nil {
		t.Fatal(err)
	}
	stmt := p.Decls[0].(*ast.StmtDecl).Statement.(*ast.LetStmt)
	add, ok := stmt.Value.(*ast.BinaryExpr)
	if !ok || add.Op != "+" {
		t.Fatalf("root should be addition, got %#v", stmt.Value)
	}
	if mul, ok := add.Right.(*ast.BinaryExpr); !ok || mul.Op != "*" {
		t.Fatalf("right side should be multiplication, got %#v", add.Right)
	}
}
func TestParserReportsMissingSemicolon(t *testing.T) {
	_, err := Parse("let x = 1", "test.ast")
	if err == nil || !strings.Contains(err.Error(), "expected ';'") {
		t.Fatalf("unexpected error: %v", err)
	}
}
func TestParsesRecursiveFunction(t *testing.T) {
	p, err := Parse(`fn f(n: i64) -> i64 { if n == 0 { return 0; } else { return f(n - 1); } }`, "test.ast")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Decls) != 1 {
		t.Fatalf("got %d declarations", len(p.Decls))
	}
}

func TestParserReportsEndOfInputAfterOperator(t *testing.T) {
	_, err := Parse("let x = 1 +", "test.ast")
	if err == nil || !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("expected EOF diagnostic, got %v", err)
	}
}

func TestRejectsCompilerInternalFunctionName(t *testing.T) {
	_, err := Parse("fn __aster_main() -> i64 { return 42; }", "test.ast")
	if err == nil || !strings.Contains(err.Error(), "reserved for compiler internals") {
		t.Fatalf("expected compiler-internal function name to be rejected, got %v", err)
	}
}
