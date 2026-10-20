package codegen

import (
	"strconv"
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/ast"
	"github.com/aster-lang/aster/internal/checker"
	"github.com/aster-lang/aster/internal/langtypes"
	"github.com/aster-lang/aster/internal/limits"
	"github.com/aster-lang/aster/internal/parser"
)

func TestGenerateTypedUsesCheckerMetadata(t *testing.T) {
	tree, err := parser.Parse(`fn increment(x: i64) -> i64 { return x + 1; } print(increment(2));`, "typed-codegen.ast")
	if err != nil {
		t.Fatal(err)
	}
	typed, err := checker.Analyze(tree)
	if err != nil {
		t.Fatal(err)
	}
	source, err := GenerateTyped(typed)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "asterAddI64(v_0, int64(1), ") {
		t.Fatalf("generated source does not use expected typed arithmetic helper:\n%s", source)
	}
}
func TestGenerateRejectsUnknownExpressionType(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "initializer", source: "let value = missing;"},
		{name: "later array element", source: "let value = [1, missing];"},
		{name: "nested expression", source: "let value = 1 + missing;"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := parser.Parse(tt.source, "unknown-type.ast")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Generate(tree); err == nil {
				t.Fatal("expected code generation to reject unknown identifier")
			}
		})
	}
}

func TestGenerateRejectsExcessiveInferredArrayTypeNesting(t *testing.T) {
	// Construct the AST directly to isolate codegen's inferred-type guard.
	var expr ast.Expr = &ast.LiteralExpr{Kind: "i64", Int: 1}
	for i := 0; i < langtypes.MaxNestingDepth+1; i++ {
		expr = &ast.ArrayExpr{Elements: []ast.Expr{expr}}
	}
	tree := &ast.Program{Decls: []ast.Decl{
		&ast.StmtDecl{Statement: &ast.LetStmt{Name: "value", Value: expr}},
	}}
	if _, err := Generate(tree); err == nil || !strings.Contains(err.Error(), "type nesting exceeds limit") {
		t.Fatalf("expected code generation type nesting limit error, got %v", err)
	}
}

func TestGenerateStringConcatenationUsesRuntimeLimitHelper(t *testing.T) {
	tree, err := parser.Parse(`let left: string = "left"; let right: string = "right"; print(left + right);`, "string-limit.ast")
	if err != nil {
		t.Fatal(err)
	}
	source, err := Generate(tree)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "asterAddString(v_0, v_1, ") {
		t.Fatalf("generated source does not use bounded string concatenation:\n%s", source)
	}
	if !strings.Contains(source, "string length limit exceeded") {
		t.Fatalf("generated source is missing the string length diagnostic:\n%s", source)
	}
}

func TestGenerateRejectsOversizedStringLiteral(t *testing.T) {
	tree := &ast.Program{Decls: []ast.Decl{
		&ast.StmtDecl{Statement: &ast.LetStmt{
			Name:  "value",
			Value: &ast.LiteralExpr{Kind: "string", String: strings.Repeat("x", limits.MaxStringBytes+1)},
		}},
	}}
	if _, err := Generate(tree); err == nil || !strings.Contains(err.Error(), "string literal exceeds") {
		t.Fatalf("expected oversized string literal rejection, got %v", err)
	}
}

func TestGenerateRejectsOversizedArrayLiteral(t *testing.T) {
	elements := make([]ast.Expr, limits.MaxArrayLength+1)
	literal := &ast.LiteralExpr{Kind: "i64", Int: 1}
	for i := range elements {
		elements[i] = literal
	}
	tree := &ast.Program{Decls: []ast.Decl{
		&ast.StmtDecl{Statement: &ast.LetStmt{
			Name:  "values",
			Value: &ast.ArrayExpr{Elements: elements},
		}},
	}}
	if _, err := Generate(tree); err == nil || !strings.Contains(err.Error(), "array literal has") {
		t.Fatalf("expected oversized array literal rejection, got %v", err)
	}
}

func TestNativeRuntimePreambleLimitsMatchSharedDefinitions(t *testing.T) {
	limits := []struct {
		name  string
		value int
	}{
		{name: "asterMaxStringBytes", value: limits.MaxStringBytes},
		{name: "asterMaxArrayNestingDepth", value: limits.MaxArrayNestingDepth},
		{name: "asterMaxCallDepth", value: limits.MaxCallDepth},
	}
	for _, limit := range limits {
		expected := "const " + limit.name + " = " + strconv.Itoa(limit.value)
		if !strings.Contains(nativePreamble, expected) {
			t.Errorf("runtime preamble does not contain %q", expected)
		}
	}
}

func TestGenerateTypedZeroArgumentUserCallHasValidArguments(t *testing.T) {
	tree, err := parser.Parse(`fn value() -> i64 { return 1; } print(value());`, "zero-arg-call.ast")
	if err != nil {
		t.Fatal(err)
	}
	typed, err := checker.Analyze(tree)
	if err != nil {
		t.Fatal(err)
	}
	source, err := GenerateTyped(typed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(source, "asterFn_0(,") {
		t.Fatalf("generated zero-argument call has an empty argument before the location metadata:\n%s", source)
	}
	if !strings.Contains(source, "asterFn_0(1, ") {
		t.Fatalf("generated zero-argument call is missing location metadata:\n%s", source)
	}
}
