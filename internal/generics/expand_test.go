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
	if err != nil {
		t.Fatal(err)
	}
	return program
}

func TestExpandRewritesExplicitGenericCallsToConcreteFunctions(t *testing.T) {
	program := parse(t, `fn identity<T>(value: T) -> T { return value; }
print(identity::<i64>(7));
print(identity::<string>("ok"));`)
	expanded, err := Expand(program)
	if err != nil {
		t.Fatal(err)
	}
	var instances []*ast.Function
	for _, decl := range expanded.Decls {
		if fn, ok := decl.(*ast.Function); ok && strings.HasPrefix(fn.Name, "__aster_generic_fn_") {
			instances = append(instances, fn)
		}
	}
	if len(instances) != 2 {
		t.Fatalf("generic instance count = %d, want 2", len(instances))
	}
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

func TestExpandInfersGenericFunctionTypeArguments(t *testing.T) {
	program := parse(t, `fn identity<T>(value: T) -> T { return value; } let x = identity(1); let y = identity("text");`)
	expanded, err := Expand(program)
	if err != nil {
		t.Fatal(err)
	}
	var instances []*ast.Function
	for _, decl := range expanded.Decls {
		if fn, ok := decl.(*ast.Function); ok && strings.HasPrefix(fn.Name, "__aster_generic_fn_") {
			instances = append(instances, fn)
		}
	}
	if len(instances) != 2 || instances[0].Params[0].Type.Name != "i64" || instances[1].Params[0].Type.Name != "string" {
		t.Fatalf("inferred instances = %#v", instances)
	}
}

func TestExpandRejectsUnresolvedGenericFunctionTypeArguments(t *testing.T) {
	program := parse(t, `fn make<T>() -> T { return 1; } let x = make();`)
	_, err := Expand(program)
	if err == nil || !strings.Contains(err.Error(), "TYP081") {
		t.Fatalf("expected TYP081, got %v", err)
	}
}

func TestExpandInfersNestedArrayTypeArguments(t *testing.T) {
	program := parse(t, `fn first<T>(values: Array<T>) -> T { return values[0]; } let value = first([3, 4]);`)
	expanded, err := Expand(program)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, decl := range expanded.Decls {
		if fn, ok := decl.(*ast.Function); ok && strings.HasPrefix(fn.Name, "__aster_generic_fn_") {
			found = true
			if fn.Params[0].Type.Name != "Array" || fn.Params[0].Type.Element == nil || fn.Params[0].Type.Element.Name != "i64" {
				t.Fatalf("nested type substitution = %#v", fn.Params[0].Type)
			}
		}
	}
	if !found {
		t.Fatal("expected a monomorphized generic function")
	}
}

func TestExpandReusesIdenticalRecursiveInstantiation(t *testing.T) {
	program := parse(t, `fn recur<T>(value: T) -> T { return recur::<T>(value); } let result = recur::<i64>(7);`)
	expanded, err := Expand(program)
	if err != nil {
		t.Fatal(err)
	}
	var instances []*ast.Function
	for _, decl := range expanded.Decls {
		if fn, ok := decl.(*ast.Function); ok && strings.HasPrefix(fn.Name, "__aster_generic_fn_") {
			instances = append(instances, fn)
		}
	}
	if len(instances) != 1 {
		t.Fatalf("recursive function instance count = %d, want one cached instance", len(instances))
	}
	returnStmt := instances[0].Body.Statements[0].(*ast.ReturnStmt)
	call := returnStmt.Value.(*ast.CallExpr)
	callee := call.Callee.(*ast.IdentifierExpr)
	if callee.Name != instances[0].Name {
		t.Fatalf("recursive call target = %q, want cached instance %q", callee.Name, instances[0].Name)
	}
}

func TestExpandRejectsGrowingRecursiveInstantiation(t *testing.T) {
	program := parse(t, `fn grow<T>(value: T) -> T { return grow::<Array<T>>([value]); } let x = grow::<i64>(1);`)
	_, err := Expand(program)
	if err == nil || !strings.Contains(err.Error(), "TYP083") {
		t.Fatalf("expected TYP083, got %v", err)
	}
}
