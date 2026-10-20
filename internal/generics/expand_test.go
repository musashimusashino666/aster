package generics

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/ast"
	"github.com/aster-lang/aster/internal/limits"
	"github.com/aster-lang/aster/internal/parser"
	"github.com/aster-lang/aster/internal/token"
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

func TestExpandSpecializesInferredAndExplicitGenericStructs(t *testing.T) {
	program := parse(t, `struct Pair<T, U> { first: T, values: Array<U> }
let inferred = Pair(7, ["seven"]);
let explicit: Pair<i64, string> = Pair::<i64, string>(8, ["eight"]);`)
	expanded, err := Expand(program)
	if err != nil {
		t.Fatal(err)
	}
	var instances []*ast.StructDecl
	for _, decl := range expanded.Decls {
		if st, ok := decl.(*ast.StructDecl); ok && strings.HasPrefix(st.Name, "__aster_generic_struct_") {
			instances = append(instances, st)
			if len(st.TypeParams) != 0 {
				t.Fatalf("generated struct %s still has type parameters", st.Name)
			}
			if len(st.Fields) != 2 || st.Fields[0].Type.Name != "i64" || st.Fields[1].Type.Name != "Array" ||
				st.Fields[1].Type.Element == nil || st.Fields[1].Type.Element.Name != "string" {
				t.Fatalf("unexpected specialized struct: %#v", st)
			}
		}
	}
	if len(instances) != 1 {
		t.Fatalf("specialized struct count = %d, want one cached Pair<i64,string>", len(instances))
	}
}

func TestExpandSpecializesGenericStructTypeApplicationsInFunctionSignatures(t *testing.T) {
	program := parse(t, `struct Box<T> { value: T }
fn unwrap<T>(box: Box<T>) -> T { return box.value; }
let value = unwrap(Box(42));`)
	expanded, err := Expand(program)
	if err != nil {
		t.Fatal(err)
	}
	var sawBox, sawUnwrap bool
	for _, decl := range expanded.Decls {
		switch n := decl.(type) {
		case *ast.StructDecl:
			if strings.HasPrefix(n.Name, "__aster_generic_struct_") {
				sawBox = true
				if n.Fields[0].Type.Name != "i64" {
					t.Fatalf("Box field type = %#v, want i64", n.Fields[0].Type)
				}
			}
		case *ast.Function:
			if strings.HasPrefix(n.Name, "__aster_generic_fn_") {
				sawUnwrap = true
				if len(n.Params) != 1 || !strings.HasPrefix(n.Params[0].Type.Name, "__aster_generic_struct_") ||
					n.ReturnType == nil || n.ReturnType.Name != "i64" {
					t.Fatalf("specialized generic function signature = %#v", n)
				}
			}
		}
	}
	if !sawBox || !sawUnwrap {
		t.Fatalf("missing specialization: Box=%t unwrap=%t", sawBox, sawUnwrap)
	}
}

func TestExpandReusesRecursiveGenericStructInstance(t *testing.T) {
	program := parse(t, `struct Node<T> { children: Array<Node<T>> }
fn take(node: Node<i64>) { }`)
	expanded, err := Expand(program)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, decl := range expanded.Decls {
		if st, ok := decl.(*ast.StructDecl); ok && strings.HasPrefix(st.Name, "__aster_generic_struct_") {
			count++
			if st.Fields[0].Type.Name != "Array" || st.Fields[0].Type.Element == nil ||
				st.Fields[0].Type.Element.Name != st.Name {
				t.Fatalf("recursive Array field = %#v, want Array<%s>", st.Fields[0].Type, st.Name)
			}
		}
	}
	if count != 1 {
		t.Fatalf("recursive generic struct instance count = %d, want 1", count)
	}
}

func TestExpandRejectsExpandingGenericStructRecursion(t *testing.T) {
	program := parse(t, `struct Nest<T> { next: Nest<Array<T>> }
fn take(value: Nest<i64>) { }`)
	_, err := Expand(program)
	if err == nil || !strings.Contains(err.Error(), "TYP086") {
		t.Fatalf("expected TYP086 expanding-instantiation diagnostic, got %v", err)
	}
}

func TestExpandSpecializesGenericEnumsFromPayloadAndExplicitArguments(t *testing.T) {
	program := parse(t, `enum Option<T> { None, Some(T), }
let inferred = Option::Some(7);
let explicit = Option::<string>::Some("aster");
let typed: Option<i64> = Option::None;`)
	expanded, err := Expand(program)
	if err != nil {
		t.Fatal(err)
	}
	instances := map[string]*ast.EnumDecl{}
	for _, decl := range expanded.Decls {
		if enumDecl, ok := decl.(*ast.EnumDecl); ok && strings.HasPrefix(enumDecl.Name, "__aster_generic_enum_") {
			instances[enumDecl.Name] = enumDecl
			if len(enumDecl.TypeParams) != 0 {
				t.Fatalf("generated enum %s still has type parameters", enumDecl.Name)
			}
		}
	}
	if len(instances) != 2 {
		t.Fatalf("generic enum instances = %d, want Option<i64> and Option<string>", len(instances))
	}
	payloadTypes := map[string]bool{}
	for _, enumDecl := range instances {
		if len(enumDecl.Variants) != 2 || len(enumDecl.Variants[1].Payload) != 1 {
			t.Fatalf("unexpected specialized enum variants: %#v", enumDecl)
		}
		payloadTypes[enumDecl.Variants[1].Payload[0].Name] = true
	}
	if !payloadTypes["i64"] || !payloadTypes["string"] {
		t.Fatalf("specialized payload types = %#v, want i64 and string", payloadTypes)
	}
	for _, decl := range expanded.Decls {
		if sd, ok := decl.(*ast.StmtDecl); ok {
			if let, ok := sd.Statement.(*ast.LetStmt); ok {
				constructor, ok := let.Value.(*ast.EnumVariantExpr)
				if !ok || strings.HasPrefix(constructor.EnumName, "Option") || len(constructor.TypeArgs) != 0 {
					t.Fatalf("enum constructor was not rewritten to a concrete type: %#v", let.Value)
				}
			}
		}
	}
}

func TestExpandMonomorphizesGenericEnumMatchBindings(t *testing.T) {
	program := parse(t, `enum Option<T> { None, Some(T), }
fn unwrap<T>(value: Option<T>) -> T {
	return match value { Option::Some(item) => item, Option::None => 0, };
}
let value = unwrap(Option::Some(42));`)
	expanded, err := Expand(program)
	if err != nil {
		t.Fatal(err)
	}
	var specialized *ast.Function
	for _, decl := range expanded.Decls {
		if fn, ok := decl.(*ast.Function); ok && strings.HasPrefix(fn.Name, "__aster_generic_fn_") {
			specialized = fn
		}
	}
	if specialized == nil || specialized.ReturnType == nil || specialized.ReturnType.Name != "i64" {
		t.Fatalf("generic enum consumer was not specialized: %#v", specialized)
	}
	match, ok := specialized.Body.Statements[0].(*ast.ReturnStmt).Value.(*ast.MatchExpr)
	if !ok || match.Arms[0].Pattern.EnumName == "Option" || match.Arms[0].Pattern.EnumName == "" {
		t.Fatalf("match patterns were not rewritten to the concrete enum identity: %#v", specialized.Body)
	}
}

func TestExpandReusesRecursiveGenericEnumInstance(t *testing.T) {
	program := parse(t, `enum List<T> { Nil, Cons(T, Array<List<T>>), }
fn consume(value: List<i64>) { }`)
	expanded, err := Expand(program)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, decl := range expanded.Decls {
		if enumDecl, ok := decl.(*ast.EnumDecl); ok && strings.HasPrefix(enumDecl.Name, "__aster_generic_enum_") {
			count++
			payload := enumDecl.Variants[1].Payload[1]
			if payload.Name != "Array" || payload.Element == nil || payload.Element.Name != enumDecl.Name {
				t.Fatalf("recursive enum payload = %#v, want Array<%s>", payload, enumDecl.Name)
			}
		}
	}
	if count != 1 {
		t.Fatalf("recursive generic enum instance count = %d, want one cached instance", count)
	}
}

func TestExpandRejectsGrowingGenericEnumInstantiation(t *testing.T) {
	program := parse(t, `enum Nest<T> { End, Next(Nest<Array<T>>), }
fn consume(value: Nest<i64>) { }`)
	_, err := Expand(program)
	if err == nil || !strings.Contains(err.Error(), "TYP089") {
		t.Fatalf("expected TYP089 expanding-instantiation diagnostic, got %v", err)
	}
}

func TestGenericEnumInstantiationLimitAndCacheHit(t *testing.T) {
	e := &expander{
		enumInstances:    make(map[string]*ast.EnumDecl, limits.MaxGenericEnumInstantiations),
		enumInstanceInfo: make(map[string]structInstanceInfo),
		enumDecls:        make(map[string]*ast.EnumDecl),
		knownTypes:       map[string]bool{"i64": true},
	}
	for i := 0; i < limits.MaxGenericEnumInstantiations-1; i++ {
		key := fmt.Sprintf("dummy<%d>", i)
		e.enumInstances[key] = &ast.EnumDecl{Name: key}
	}
	e.enumInstances["Option<i64>"] = &ast.EnumDecl{Name: "__cached_option"}
	template := &ast.EnumDecl{
		Name: "Option", TypeParams: []string{"T"},
		Variants: []ast.EnumVariant{{Name: "None"}},
	}
	name, err := e.instantiateEnum(template, []ast.TypeRef{{Name: "i64"}}, token.Position{}, nil, 0)
	if err != nil || name != "__cached_option" {
		t.Fatalf("cache hit at the instance limit = (%q, %v), want cached instance", name, err)
	}
	delete(e.enumInstances, "Option<i64>")
	e.enumInstances["at-limit"] = &ast.EnumDecl{Name: "at-limit"}
	if len(e.enumInstances) != limits.MaxGenericEnumInstantiations {
		t.Fatalf("test setup has %d instances, want limit %d", len(e.enumInstances), limits.MaxGenericEnumInstantiations)
	}
	_, err = e.instantiateEnum(template, []ast.TypeRef{{Name: "i64"}}, token.Position{}, nil, 0)
	if err == nil || !strings.Contains(err.Error(), "TYP089") {
		t.Fatalf("expected TYP089 at the exact generic enum instance limit, got %v", err)
	}
}
