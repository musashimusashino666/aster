package codegen

import (
	"strings"
	"testing"

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
	depth := langtypes.MaxNestingDepth + 1
	source := "let value = " + strings.Repeat("[", depth) + "1" + strings.Repeat("]", depth) + ";"
	tree, err := parser.Parse(source, "nested-array.ast")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(tree); err == nil || !strings.Contains(err.Error(), "type nesting exceeds limit") {
		t.Fatalf("expected code generation type nesting limit error, got %v", err)
	}
}
