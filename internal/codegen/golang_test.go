package codegen

import (
	"testing"

	"github.com/aster-lang/aster/internal/parser"
)

func TestGenerateRejectsUnknownExpressionType(t *testing.T) {
	tree, err := parser.Parse("let value = missing;", "unknown-type.ast")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(tree); err == nil {
		t.Fatal("expected type inference error")
	}
}
