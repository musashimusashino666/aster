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

func TestParsesI64MinimumLiteral(t *testing.T) {
	tree, err := Parse("let minimum: i64 = -9223372036854775808;", "test.ast")
	if err != nil {
		t.Fatal(err)
	}
	stmt := tree.Decls[0].(*ast.StmtDecl).Statement.(*ast.LetStmt)
	literal, ok := stmt.Value.(*ast.LiteralExpr)
	if !ok || literal.Int != -1<<63 {
		t.Fatalf("minimum literal = %#v, want i64 minimum", stmt.Value)
	}
}

func TestRejectsPositiveI64MinimumMagnitude(t *testing.T) {
	_, err := Parse("let tooLarge = 9223372036854775808;", "test.ast")
	if err == nil || !strings.Contains(err.Error(), "integer literal out of range") {
		t.Fatalf("expected out-of-range integer diagnostic, got %v", err)
	}
}

func TestParsesSimpleArrayElementAssignment(t *testing.T) {
	program, err := Parse("let mut values = [1]; values[0] = 2;", "test.ast")
	if err != nil {
		t.Fatal(err)
	}
	stmt := program.Decls[1].(*ast.StmtDecl).Statement
	if _, ok := stmt.(*ast.IndexAssignStmt); !ok {
		t.Fatalf("statement type = %T, want *ast.IndexAssignStmt", stmt)
	}
}

func TestPositiveI64MinimumMagnitudeExplainsUnaryMinusRequirement(t *testing.T) {
	_, err := Parse("let tooLarge = 0 - 9223372036854775808;", "test.ast")
	if err == nil || !strings.Contains(err.Error(), "only immediately after unary '-'") {
		t.Fatalf("expected actionable i64 boundary diagnostic, got %v", err)
	}
}

func TestParsesNestedArrayElementAssignment(t *testing.T) {
	program, err := Parse("let mut matrix = [[1, 2], [3, 4]]; matrix[0][1] = 9;", "indexed-assignment.ast")
	if err != nil {
		t.Fatal(err)
	}
	stmt := program.Decls[1].(*ast.StmtDecl).Statement
	assign, ok := stmt.(*ast.IndexAssignStmt)
	if !ok {
		t.Fatalf("statement type = %T, want *ast.IndexAssignStmt", stmt)
	}
	outer, ok := assign.Target.(*ast.IndexExpr)
	if !ok {
		t.Fatalf("target type = %T, want *ast.IndexExpr", assign.Target)
	}
	inner, ok := outer.Collection.(*ast.IndexExpr)
	if !ok {
		t.Fatalf("target collection type = %T, want nested *ast.IndexExpr", outer.Collection)
	}
	if root, ok := inner.Collection.(*ast.IdentifierExpr); !ok || root.Name != "matrix" {
		t.Fatalf("indexed assignment root = %#v, want matrix", inner.Collection)
	}
}

func TestParsesHalfOpenArraySlice(t *testing.T) {
	program, err := Parse("let values = [1, 2, 3, 4]; let part = values[1:3];", "slice.ast")
	if err != nil {
		t.Fatal(err)
	}
	stmt := program.Decls[1].(*ast.StmtDecl).Statement.(*ast.LetStmt)
	slice, ok := stmt.Value.(*ast.SliceExpr)
	if !ok {
		t.Fatalf("expression type = %T, want *ast.SliceExpr", stmt.Value)
	}
	start, ok := slice.Start.(*ast.LiteralExpr)
	if !ok || start.Int != 1 {
		t.Fatalf("slice start = %#v, want 1", slice.Start)
	}
	end, ok := slice.End.(*ast.LiteralExpr)
	if !ok || end.Int != 3 {
		t.Fatalf("slice end = %#v, want 3", slice.End)
	}
}

func TestRejectsMissingSliceBounds(t *testing.T) {
	for _, source := range []string{"let values = [1]; let part = values[:1];", "let values = [1]; let part = values[0:];"} {
		if _, err := Parse(source, "slice.ast"); err == nil {
			t.Errorf("expected missing slice bounds to be rejected for %q", source)
		}
	}
}

func TestParsesStructDeclarationAndFieldSelection(t *testing.T) {
	program, err := Parse(`struct Point { x: i64, y: i64, }
let point = Point(3, 4);
let horizontal = point.x;`, "struct.ast")
	if err != nil {
		t.Fatal(err)
	}
	if len(program.Decls) != 3 {
		t.Fatalf("declaration count = %d, want struct plus two statements", len(program.Decls))
	}
	decl, ok := program.Decls[0].(*ast.StructDecl)
	if !ok || decl.Name != "Point" || len(decl.Fields) != 2 {
		t.Fatalf("struct declaration = %#v", program.Decls[0])
	}
	let := program.Decls[2].(*ast.StmtDecl).Statement.(*ast.LetStmt)
	field, ok := let.Value.(*ast.FieldExpr)
	if !ok || field.Name != "x" {
		t.Fatalf("field expression = %#v", let.Value)
	}
}

func TestParsesEnumPayloadsAndMatchExpression(t *testing.T) {
	source := `enum Message { Quit, Move(i64, i64), Write(string), }
let message = Message::Move(10, 20);
let label = match message {
	Message::Quit => "quit",
	Message::Move(x, y) => "move",
	Message::Write(text) => text,
};`
	tree, err := Parse(source, "enum-match.ast")
	if err != nil {
		t.Fatal(err)
	}
	enumDecl, ok := tree.Decls[0].(*ast.EnumDecl)
	if !ok {
		t.Fatalf("first declaration = %T, want enum declaration", tree.Decls[0])
	}
	if len(enumDecl.Variants) != 3 || len(enumDecl.Variants[1].Payload) != 2 {
		t.Fatalf("unexpected enum variants: %#v", enumDecl.Variants)
	}
	letDecl := tree.Decls[2].(*ast.StmtDecl).Statement.(*ast.LetStmt)
	match, ok := letDecl.Value.(*ast.MatchExpr)
	if !ok {
		t.Fatalf("initializer = %T, want match expression", letDecl.Value)
	}
	if len(match.Arms) != 3 || match.Arms[1].Pattern.VariantName != "Move" || len(match.Arms[1].Pattern.Bindings) != 2 {
		t.Fatalf("unexpected match arms: %#v", match.Arms)
	}
}
