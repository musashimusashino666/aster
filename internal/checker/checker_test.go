package checker

import (
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/ast"
	"github.com/aster-lang/aster/internal/ir"
	"github.com/aster-lang/aster/internal/parser"
)

func checkSource(t *testing.T, src string) error {
	t.Helper()
	p, err := parser.Parse(src, "test.ast")
	if err != nil {
		return err
	}
	return Check(p)
}

func TestAnalyzeExposesExpressionTypesAndFunctionSignatures(t *testing.T) {
	tree, err := parser.Parse(`fn identity(value: i64) -> i64 { return value; } let result = identity(1 + 2);`, "typed.ast")
	if err != nil {
		t.Fatal(err)
	}
	typed, err := Analyze(tree)
	if err != nil {
		t.Fatal(err)
	}
	info, ok := typed.Function("identity")
	if !ok {
		t.Fatal("checker result is missing identity signature")
	}
	if info.Return != I64 || len(info.Params) != 1 || info.Params[0] != I64 {
		t.Fatalf("unexpected identity signature: %#v", info)
	}

	stmt := tree.Decls[1].(*ast.StmtDecl).Statement.(*ast.LetStmt)
	call, ok := stmt.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("initializer should be a call, got %T", stmt.Value)
	}
	if got, ok := typed.TypeOf(call); !ok || got != I64 {
		t.Fatalf("call type = %q, present=%v; want i64", got, ok)
	}
	add, ok := call.Args[0].(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("call argument should be binary expression, got %T", call.Args[0])
	}
	if got, ok := typed.TypeOf(add); !ok || got != I64 {
		t.Fatalf("binary expression type = %q, present=%v; want i64", got, ok)
	}
	if got, ok := typed.TypeOf(add.Left); !ok || got != I64 {
		t.Fatalf("left literal type = %q, present=%v; want i64", got, ok)
	}
}

func TestAcceptsRecursionAndTypeInference(t *testing.T) {
	src := `fn fact(n: i64) -> i64 { if n <= 1 { return 1; } else { return n * fact(n - 1); } } let mut result = fact(5); result = result + 1; print(result);`
	if err := checkSource(t, src); err != nil {
		t.Fatal(err)
	}
}
func TestRejectsImmutableAssignment(t *testing.T) {
	err := checkSource(t, "let x = 1; x = 2;")
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("unexpected error: %v", err)
	}
}
func TestRejectsTypeMismatch(t *testing.T) {
	err := checkSource(t, `let x: i64 = "bad";`)
	if err == nil || !strings.Contains(err.Error(), "cannot assign string") {
		t.Fatalf("unexpected error: %v", err)
	}
}
func TestRequiresReturnOnEveryPath(t *testing.T) {
	err := checkSource(t, `fn f(n: i64) -> i64 { if n > 0 { return n; } }`)
	if err == nil || !strings.Contains(err.Error(), "must return i64") {
		t.Fatalf("unexpected error: %v", err)
	}
}
func TestRejectsBadCallArity(t *testing.T) {
	err := checkSource(t, `fn f(x: i64) -> i64 { return x; } print(f());`)
	if err == nil || !strings.Contains(err.Error(), "expects 1 arguments") {
		t.Fatalf("unexpected error: %v", err)
	}
}
func TestRejectsUnreachableCode(t *testing.T) {
	err := checkSource(t, `fn f() { return; print(1); }`)
	if err == nil || !strings.Contains(err.Error(), "unreachable statement") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestArraysAreHomogeneousAndTyped(t *testing.T) {
	good := `fn sum(items: Array<i64>) -> i64 { let mut i: i64 = 0; let mut s: i64 = 0; while i < len(items) { s = s + items[i]; i = i + 1; } return s; } print(sum([1, 2, 3]));`
	if err := checkSource(t, good); err != nil {
		t.Fatal(err)
	}
	bad := `let items = [1, "two"];`
	if err := checkSource(t, bad); err == nil || !strings.Contains(err.Error(), "array elements must have one type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRejectsUnitValueInInferredBinding(t *testing.T) {
	err := checkSource(t, "let x = print(1);")
	if err == nil || !strings.Contains(err.Error(), "variables cannot hold unit values") {
		t.Fatalf("expected unit binding to be rejected, got %v", err)
	}
}

func TestRejectsPrintingUnitValue(t *testing.T) {
	err := checkSource(t, "print(print(1));")
	if err == nil || !strings.Contains(err.Error(), "print cannot display a unit value") {
		t.Fatalf("expected unit print argument to be rejected, got %v", err)
	}
}

func TestAnalyzeBuildsTypedIRWithResolvedSymbolsAndCheckedOperations(t *testing.T) {
	source := `fn increment(value: i64) -> i64 { return value + 1; } let mut count = increment(2); { let count = 9; print(count); } count = count + 1; print(count);`
	tree, err := parser.Parse(source, "typed-ir.ast")
	if err != nil {
		t.Fatal(err)
	}
	typed, err := Analyze(tree)
	if err != nil {
		t.Fatal(err)
	}
	if typed.IR == nil {
		t.Fatal("checker result is missing normalized IR")
	}
	if typed.IR.SourceName != "typed-ir.ast" || len(typed.IR.Functions) != 1 {
		t.Fatalf("unexpected IR program metadata: %#v", typed.IR)
	}

	fn := typed.IR.Functions[0]
	if fn.ID != 0 || fn.Name != "increment" || fn.ReturnType != ir.Type(I64) || len(fn.Params) != 1 {
		t.Fatalf("unexpected lowered function: %#v", fn)
	}
	if fn.Params[0].ID != 0 || fn.Params[0].Type != ir.Type(I64) {
		t.Fatalf("unexpected parameter symbol: %#v", fn.Params[0])
	}
	ret, ok := fn.Body.Statements[0].(*ir.ReturnStmt)
	if !ok {
		t.Fatalf("function body statement = %T, want return", fn.Body.Statements[0])
	}
	add, ok := ret.Value.(*ir.BinaryExpr)
	if !ok || add.Op != ir.OpAddI64Checked || add.Pos().Line != 1 {
		t.Fatalf("return expression = %#v, want source-spanned checked i64 addition", ret.Value)
	}
	local, ok := add.Left.(*ir.LocalExpr)
	if !ok || local.Symbol != fn.Params[0].ID || local.ExprType() != ir.Type(I64) {
		t.Fatalf("left operand did not resolve to the parameter symbol: %#v", add.Left)
	}

	if len(typed.IR.Body) != 4 {
		t.Fatalf("top-level IR statement count = %d, want 4", len(typed.IR.Body))
	}
	topLet, ok := typed.IR.Body[0].(*ir.LetStmt)
	if !ok {
		t.Fatalf("first top-level statement = %T, want let", typed.IR.Body[0])
	}
	call, ok := topLet.Value.(*ir.CallExpr)
	if !ok || call.Target != ir.CallUserFunction || call.Function != fn.ID {
		t.Fatalf("initializer call did not resolve to function identity: %#v", topLet.Value)
	}
	block, ok := typed.IR.Body[1].(*ir.BlockStmt)
	if !ok {
		t.Fatalf("second top-level statement = %T, want block", typed.IR.Body[1])
	}
	shadow, ok := block.Statements[0].(*ir.LetStmt)
	if !ok || shadow.Local.Name != "count" || shadow.Local.ID == topLet.Local.ID {
		t.Fatalf("shadowing did not allocate a distinct local symbol: outer=%#v inner=%#v", topLet.Local, shadow)
	}
	printStmt, ok := block.Statements[1].(*ir.ExprStmt)
	if !ok {
		t.Fatalf("block print statement = %T, want expression", block.Statements[1])
	}
	printCall, ok := printStmt.Expr.(*ir.CallExpr)
	if !ok || printCall.Target != ir.CallPrint {
		t.Fatalf("print was not lowered to a builtin call: %#v", printStmt.Expr)
	}
	printLocal, ok := printCall.Args[0].(*ir.LocalExpr)
	if !ok || printLocal.Symbol != shadow.Local.ID {
		t.Fatalf("shadowed identifier resolves to the wrong symbol: %#v", printCall.Args[0])
	}
	assign, ok := typed.IR.Body[2].(*ir.AssignStmt)
	if !ok || assign.Target != topLet.Local.ID {
		t.Fatalf("assignment target did not resolve to outer symbol: %#v", typed.IR.Body[2])
	}
}
