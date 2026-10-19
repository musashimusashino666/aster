package checker

import (
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/ast"
	"github.com/aster-lang/aster/internal/langtypes"
	"github.com/aster-lang/aster/internal/parser"
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
	depth := langtypes.MaxNestingDepth + 1
	source := strings.Repeat("[", depth) + "1" + strings.Repeat("]", depth) + ";"
	tree, err := parser.Parse(source, "nested-array.ast")
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(tree); err == nil || !strings.Contains(err.Error(), "type nesting exceeds limit") {
		t.Fatalf("expected inferred array type nesting limit error, got %v", err)
	}
}
