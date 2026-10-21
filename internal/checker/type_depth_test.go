package checker

import (
	"strings"
	"testing"

	"github.com/musashimusashino666/aster/internal/ast"
	"github.com/musashimusashino666/aster/internal/langtypes"
)

func nestedArrayTypeRef(depth int) ast.TypeRef {
	ref := ast.TypeRef{Name: "i64"}
	for i := 0; i < depth; i++ {
		element := ref
		ref = ast.TypeRef{Name: "Array", Element: &element}
	}
	return ref
}

func TestTypeFromRefEnforcesSharedNestingLimit(t *testing.T) {
	if _, err := typeFromRef(nestedArrayTypeRef(langtypes.MaxNestingDepth)); err != nil {
		t.Fatalf("maximum supported nesting should type-check: %v", err)
	}
	if _, err := typeFromRef(nestedArrayTypeRef(langtypes.MaxNestingDepth + 1)); err == nil || !strings.Contains(err.Error(), "type nesting exceeds limit") {
		t.Fatalf("expected type nesting limit error, got %v", err)
	}
}

func TestCheckRejectsExcessiveInferredArrayTypeNesting(t *testing.T) {
	// Construct the AST directly to isolate the checker's type-depth guard.
	var expr ast.Expr = &ast.LiteralExpr{Kind: "i64", Int: 1}
	for i := 0; i < langtypes.MaxNestingDepth+1; i++ {
		expr = &ast.ArrayExpr{Elements: []ast.Expr{expr}}
	}
	tree := &ast.Program{Decls: []ast.Decl{
		&ast.StmtDecl{Statement: &ast.LetStmt{Name: "value", Value: expr}},
	}}
	if err := Check(tree); err == nil || !strings.Contains(err.Error(), "type nesting exceeds limit") {
		t.Fatalf("expected inferred array type nesting limit error, got %v", err)
	}
}
