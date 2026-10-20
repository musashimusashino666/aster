package generics

import (
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/ast"
	"github.com/aster-lang/aster/internal/parser"
)

func parse(t *testing.T, src string) *ast.Program {
	t.Helper()
	program, err := parser.Parse(src, "generics.ast")
	if err != nil { t.Fatal(err) }
	return program
}

func TestExpandRewritesExplicitGenericCallsToConcreteFunctions(t *testing.T) {
	program := parse(t, `fn identity<T>(value: T) -> T { return value; }
print(identity::<i64>(7));
print(identity::<string>("ok"));`)
	expanded, err := Expand(program)
	if err != nil { t.Fatal(err) }
	var instances []*ast.Function
	for _, decl := range expanded.Decls {
		if fn, ok := decl.(*ast.Function); ok && strings.HasPrefix(fn.Name, "__aster_generic_fn_") {
			instances = append(instances, fn)
		}
	}
	if len(instances) != 2 { t.Fatalf("generic instance count = %d, want 2", len(instances)) }
	if instances[0].Params[0].Type.Name != "i64" || instances[1].Params[0].Type.Name != "string" {
		t.Fatalf("unexpected specialized parameter types: %#v %#v", instances[0].Params[0].Type, instances[1].Params[0].Type)
	}
	first := expanded.Decls[0].(*ast.StmtDecl).Statement.(*ast.ExprStmt).Expr.(*ast.CallExpr)
	inner, ok := first.Args[0].(*ast.CallExpr)
	if !ok || len(inner.TypeArgs) != 0 {
		t.Fatalf("explicit type arguments were not erased from nested call: %#v", first.Args[0])
	}
	callee, ok := inner.Callee.(*ast.IdentifierExpr)
	if !ok || !strings.HasPrefix(callee.Name, "__aster_generic_fn_") {
		t.Fatalf("generic call was not rewritten to an instance: %#v", inner.Callee)
	}
}

func TestExpandRejectsInferenceUntilInferenceSliceLands(t *testing.T) {
	program := parse(t, `fn identity<T>(value: T) -> T { return value; } let x = identity(1);`)
	_, err := Expand(program)
	if err == nil || !strings.Contains(err.Error(), "TYP081") { t.Fatalf("expected TYP081, got %v", err) }
}

func TestExpandRejectsGrowingRecursiveInstantiation(t *testing.T) {
	program := parse(t, `fn grow<T>(value: T) -> T { return grow::<Array<T>>([value]); } let x = grow::<i64>(1);`)
	_, err := Expand(program)
	if err == nil || !strings.Contains(err.Error(), "TYP083") { t.Fatalf("expected TYP083, got %v", err) }
}
