package codegen

import (
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/ast"
	"github.com/aster-lang/aster/internal/langtypes"
	"github.com/aster-lang/aster/internal/parser"
)

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
