package checker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/ast"
	"github.com/aster-lang/aster/internal/ir"
	"github.com/aster-lang/aster/internal/limits"
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

func TestConstantBooleanConditionsImproveReturnFlow(t *testing.T) {
	valid := []struct {
		name   string
		source string
	}{
		{
			name:   "literal true if without else",
			source: "fn f() -> i64 { if true { return 1; } }",
		},
		{
			name:   "literal false if selects else",
			source: "fn f() -> i64 { if false { return 1; } else { return 2; } }",
		},
		{
			name:   "literal true while never falls through",
			source: "fn f() -> i64 { while true { } }",
		},
		{
			name:   "literal false while falls through to return",
			source: "fn f() -> i64 { while false { return 1; } return 2; }",
		},
	}
	for _, tt := range valid {
		t.Run(tt.name, func(t *testing.T) {
			if err := checkSource(t, tt.source); err != nil {
				t.Fatalf("expected source to type-check: %v", err)
			}
		})
	}

	invalid := []struct {
		name   string
		source string
	}{
		{
			name:   "literal false if without else",
			source: "fn f() -> i64 { if false { return 1; } }",
		},
		{
			name:   "nonconstant while may execute zero times",
			source: "fn f(condition: bool) -> i64 { while condition { return 1; } }",
		},
		{
			name:   "literal true branch that falls through",
			source: "fn f() -> i64 { if true { print(1); } }",
		},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			err := checkSource(t, tt.source)
			if err == nil || !strings.Contains(err.Error(), "must return i64") {
				t.Fatalf("expected missing-return diagnostic, got %v", err)
			}
		})
	}
}

func TestRejectsStatementsAfterNonFallthroughWhile(t *testing.T) {
	err := checkSource(t, "fn f() { while true { } print(1); }")
	if err == nil || !strings.Contains(err.Error(), "unreachable statement") {
		t.Fatalf("expected unreachable-statement diagnostic, got %v", err)
	}
}

func TestChecksIndexedArrayAssignment(t *testing.T) {
	valid := []string{
		"let mut values = [1, 2]; values[0] = 9;",
		"let mut matrix: Array<Array<i64>> = [[1, 2], [3, 4]]; matrix[0][1] = 9;",
		"let mut matrix: Array<Array<i64>> = [[1], [2]]; let mut row: Array<i64> = [8, 9]; matrix[0] = row;",
	}
	for _, source := range valid {
		if err := checkSource(t, source); err != nil {
			t.Errorf("expected indexed assignment to type-check for %q: %v", source, err)
		}
	}

	invalid := []struct {
		name, source, want string
	}{
		{"immutable root", "let values = [1, 2]; values[0] = 9;", "immutable variable"},
		{"value type mismatch", "let mut values = [1, 2]; values[0] = true;", "array element of type"},
		{"index type mismatch", "let mut values = [1, 2]; values[true] = 9;", "array index must be i64"},
		{"non-array root", "let mut value = 1; value[0] = 9;", "indexing requires an Array<T>"},
		{"nested non-array", "let mut matrix = [[1, 2]]; matrix[0][true] = 9;", "array index must be i64"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			err := checkSource(t, tt.source)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestChecksArraySliceExpressions(t *testing.T) {
	for _, source := range []string{
		"let values = [1, 2, 3]; let part = values[1:3]; print(part);",
		"let values = [[1, 2], [3, 4]]; let row = values[0][0:2]; print(row);",
		"let values = [1, 2]; let empty = values[1:1]; print(len(empty));",
	} {
		if err := checkSource(t, source); err != nil {
			t.Errorf("expected slice expression to type-check for %q: %v", source, err)
		}
	}

	invalid := []struct {
		name, source, want string
	}{
		{"non-array source", "let value = 1; let part = value[0:1];", "slicing requires an Array<T>"},
		{"start bound type", "let values = [1, 2]; let part = values[true:1];", "slice bounds must be i64"},
		{"end bound type", "let values = [1, 2]; let part = values[0:false];", "slice bounds must be i64"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			err := checkSource(t, tt.source)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestChecksExplicitNumericCasts(t *testing.T) {
	valid := []string{
		"print(i64(3.9));",
		"print(i64(-3.9));",
		"print(f64(42));",
		"print(i64(i64(7)));",
		"print(f64(f64(2.5)));",
	}
	for _, source := range valid {
		if err := checkSource(t, source); err != nil {
			t.Errorf("expected numeric cast to type-check for %q: %v", source, err)
		}
	}

	invalid := []struct {
		name, source, want string
	}{
		{"boolean argument", "print(i64(true));", "conversion requires i64 or f64"},
		{"wrong arity", "print(i64(1, 2));", "expects exactly one argument"},
		{"boolean f64 argument", "print(f64(false));", "conversion requires i64 or f64"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			err := checkSource(t, tt.source)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestChecksStructConstructionAndFieldSelection(t *testing.T) {
	valid := `struct Point { x: i64, y: i64 }
struct Box { point: Point, values: Array<i64> }
fn pass(value: Box) -> Box { return value; }
let mut values: Array<i64> = [1, 2, 3];
let boxValue = pass(Box(Point(4, 5), values));
print(boxValue.point.x);
print(boxValue.values[1]);`
	if err := checkSource(t, valid); err != nil {
		t.Fatalf("valid struct program failed: %v", err)
	}
}

func TestRejectsInvalidStructOperations(t *testing.T) {
	tests := []struct {
		name, source, want string
	}{
		{"unknown field", "struct Point { x: i64 } let p = Point(1); print(p.y);", "has no field 'y'"},
		{"constructor arity", "struct Point { x: i64 } let p = Point();", "expects 1 fields, got 0"},
		{"constructor type", "struct Point { x: i64 } let p = Point(true);", "expects i64, got bool"},
		{"non-struct field", "let value = 1; print(value.x);", "field selection requires a struct value"},
		{"duplicate field", "struct Point { x: i64, x: i64 }", "field 'x' is duplicated"},
		{"by-value recursive layout", "struct A { b: B } struct B { a: A }", "recursive by-value layout"},
		{"struct print unsupported", "struct Point { x: i64 } print(Point(1));", "print does not yet support values of type Point"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkSource(t, tt.source)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestAllowsRecursiveStructThroughArray(t *testing.T) {
	source := `struct Node { children: Array<Node> }
let leaf = Node([]);
print(len(leaf.children));`
	if err := checkSource(t, source); err != nil {
		t.Fatalf("array-indirected recursive struct should be accepted: %v", err)
	}
}

func TestRejectsReservedBuiltinTypeNamesForNominalDeclarations(t *testing.T) {
	for _, name := range []string{"bool", "string", "unit"} {
		t.Run("enum "+name, func(t *testing.T) {
			err := checkSource(t, "enum "+name+" { Value, }")
			if err == nil || !strings.Contains(err.Error(), "TYP063") {
				t.Fatalf("expected reserved enum name diagnostic TYP063, got %v", err)
			}
		})
		t.Run("struct "+name, func(t *testing.T) {
			err := checkSource(t, "struct "+name+" { value: i64 }")
			if err == nil || !strings.Contains(err.Error(), "TYP052") {
				t.Fatalf("expected reserved struct name diagnostic TYP052, got %v", err)
			}
		})
	}
}

func TestRejectsEnumMetadataOverSharedLimits(t *testing.T) {
	t.Run("enum type count", func(t *testing.T) {
		decls := make([]ast.Decl, limits.MaxEnumTypes+1)
		for i := range decls {
			decls[i] = &ast.EnumDecl{Name: fmt.Sprintf("Enum%d", i), Variants: []ast.EnumVariant{{Name: "Item"}}}
		}
		err := Check(&ast.Program{Decls: decls})
		if err == nil || !strings.Contains(err.Error(), "TYP078") {
			t.Fatalf("expected enum type resource limit diagnostic TYP078, got %v", err)
		}
	})
	t.Run("variant count", func(t *testing.T) {
		variants := make([]ast.EnumVariant, limits.MaxEnumVariants+1)
		for i := range variants {
			variants[i] = ast.EnumVariant{Name: fmt.Sprintf("Variant%d", i)}
		}
		err := Check(&ast.Program{Decls: []ast.Decl{&ast.EnumDecl{Name: "ManyVariants", Variants: variants}}})
		if err == nil || !strings.Contains(err.Error(), "TYP078") {
			t.Fatalf("expected enum variant resource limit diagnostic TYP078, got %v", err)
		}
	})
	t.Run("payload field count", func(t *testing.T) {
		payload := make([]ast.TypeRef, limits.MaxEnumPayloadFields+1)
		for i := range payload {
			payload[i] = ast.TypeRef{Name: "i64"}
		}
		err := Check(&ast.Program{Decls: []ast.Decl{&ast.EnumDecl{
			Name:     "ManyPayloads",
			Variants: []ast.EnumVariant{{Name: "Values", Payload: payload}},
		}}})
		if err == nil || !strings.Contains(err.Error(), "TYP078") {
			t.Fatalf("expected enum payload resource limit diagnostic TYP078, got %v", err)
		}
	})
}

func TestChecksExplicitGenericFunctionInstantiations(t *testing.T) {
	source := `fn identity<T>(value: T) -> T { return value; }
fn choose<T>(left: T, right: T, flag: bool) -> T {
	if flag { return left; }
	return right;
}
let integer = identity::<i64>(7);
let text = identity::<string>("ok");
let chosen = choose::<i64>(integer, 9, true);`
	if err := checkSource(t, source); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsGenericFunctionWithoutExplicitTypeArguments(t *testing.T) {
	err := checkSource(t, `fn identity<T>(value: T) -> T { return value; } let value = identity(1);`)
	if err == nil || !strings.Contains(err.Error(), "TYP081") {
		t.Fatalf("expected TYP081 explicit-type-argument diagnostic, got %v", err)
	}
}

func TestRejectsWrongGenericTypeArgumentCount(t *testing.T) {
	err := checkSource(t, `fn identity<T>(value: T) -> T { return value; } let value = identity::<i64, string>(1);`)
	if err == nil || !strings.Contains(err.Error(), "TYP080") {
		t.Fatalf("expected TYP080 generic-arity diagnostic, got %v", err)
	}
}

func TestRejectsDuplicateGenericTypeParameters(t *testing.T) {
	err := checkSource(t, `fn identity<T, T>(value: T) -> T { return value; } let value = identity::<i64>(1);`)
	if err == nil || !strings.Contains(err.Error(), "TYP079") {
		t.Fatalf("expected TYP079 invalid type parameter diagnostic, got %v", err)
	}
}

func TestRejectsTypeMismatchAfterGenericSubstitution(t *testing.T) {
	err := checkSource(t, `fn identity<T>(value: T) -> T { return value; } print(identity::<i64>("wrong"));`)
	if err == nil || !strings.Contains(err.Error(), "TYP045") {
		t.Fatalf("expected TYP045 concrete argument mismatch, got %v", err)
	}
}

func TestRejectsExplicitTypeArgumentsOnNonGenericFunction(t *testing.T) {
	err := checkSource(t, `let value = len::<i64>([1, 2]);`)
	if err == nil || !strings.Contains(err.Error(), "TYP082") {
		t.Fatalf("expected TYP082 non-generic target diagnostic, got %v", err)
	}
}

func TestAcceptsEnumPayloadAndExhaustiveMatch(t *testing.T) {
	source := `enum Result { Ok(i64), Err(string), }
fn value(result: Result) -> i64 {
	return match result {
		Result::Ok(number) => number,
		Result::Err(message) => 0,
	};
}
print(value(Result::Ok(7))); `
	if err := checkSource(t, source); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsNonExhaustiveEnumMatch(t *testing.T) {
	err := checkSource(t, `enum State { Ready, Busy, } let status = match State::Ready { State::Ready => 1, };`)
	if err == nil || !strings.Contains(err.Error(), "TYP074") {
		t.Fatalf("expected TYP074 non-exhaustive match diagnostic, got %v", err)
	}
}

func TestRejectsEnumMatchResultTypeMismatch(t *testing.T) {
	err := checkSource(t, `enum State { Ready, Busy, } let status = match State::Ready { State::Ready => 1, State::Busy => false, };`)
	if err == nil || !strings.Contains(err.Error(), "TYP075") {
		t.Fatalf("expected TYP075 match result type diagnostic, got %v", err)
	}
}

func TestRejectsDuplicateAndUnreachableMatchArms(t *testing.T) {
	duplicate := checkSource(t, `enum State { Ready, Busy, } let status = match State::Ready { State::Ready => 1, State::Ready => 2, State::Busy => 3, };`)
	if duplicate == nil || !strings.Contains(duplicate.Error(), "TYP072") {
		t.Fatalf("expected TYP072 duplicate-arm diagnostic, got %v", duplicate)
	}
	unreachable := checkSource(t, `enum State { Ready, Busy, } let status = match State::Ready { _ => 1, State::Busy => 2, };`)
	if unreachable == nil || !strings.Contains(unreachable.Error(), "TYP073") {
		t.Fatalf("expected TYP073 unreachable-arm diagnostic, got %v", unreachable)
	}
}

func TestAcceptsWildcardAndArrayGuardedRecursiveEnumLayout(t *testing.T) {
	if err := checkSource(t, `enum Node { End, Next(Array<Node>), }
enum State { Ready, Busy, }
let status = match State::Busy { State::Ready => 1, _ => 2, };`); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsDirectRecursiveEnumLayout(t *testing.T) {
	err := checkSource(t, `enum Node { End, Next(Node), }`)
	if err == nil || !strings.Contains(err.Error(), "TYP077") {
		t.Fatalf("expected TYP077 recursive enum layout diagnostic, got %v", err)
	}
}

func TestRejectsRepeatedEnumPayloadBinding(t *testing.T) {
	err := checkSource(t, `enum Pair { Values(i64, i64), }
let result = match Pair::Values(1, 2) { Pair::Values(x, x) => 1, };`)
	if err == nil || !strings.Contains(err.Error(), "TYP076") {
		t.Fatalf("expected TYP076 duplicate payload-binding diagnostic, got %v", err)
	}
}
