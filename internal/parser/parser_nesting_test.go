package parser

import (
	"strings"
	"testing"
)

func TestParenthesizedExpressionNestingLimit(t *testing.T) {
	validDepth := maxSyntaxNestingDepth - 1
	valid := "let value = " + strings.Repeat("(", validDepth) + "1" + strings.Repeat(")", validDepth) + ";"
	if _, err := Parse(valid, "nesting.ast"); err != nil {
		t.Fatalf("maximum supported parenthesis nesting should parse: %v", err)
	}

	tooDeep := "let value = " + strings.Repeat("(", maxSyntaxNestingDepth) + "1" + strings.Repeat(")", maxSyntaxNestingDepth) + ";"
	if _, err := Parse(tooDeep, "nesting.ast"); err == nil || !strings.Contains(err.Error(), "syntax nesting exceeds limit") {
		t.Fatalf("expected expression syntax nesting limit error, got %v", err)
	}
}


func TestNestedArrayExpressionNestingLimit(t *testing.T) {
	validDepth := maxSyntaxNestingDepth - 1
	valid := "let value = " + strings.Repeat("[", validDepth) + "1" + strings.Repeat("]", validDepth) + ";"
	if _, err := Parse(valid, "nesting.ast"); err != nil {
		t.Fatalf("maximum supported nested array expression should parse: %v", err)
	}

	tooDeep := "let value = " + strings.Repeat("[", maxSyntaxNestingDepth) + "1" + strings.Repeat("]", maxSyntaxNestingDepth) + ";"
	if _, err := Parse(tooDeep, "nesting.ast"); err == nil || !strings.Contains(err.Error(), "syntax nesting exceeds limit") {
		t.Fatalf("expected nested array syntax limit error, got %v", err)
	}
}

func TestExpressionASTDepthLimit(t *testing.T) {
	valid := "let value = " + strings.Repeat("1 + ", maxSyntaxNestingDepth-1) + "1;"
	if _, err := Parse(valid, "nesting.ast"); err != nil {
		t.Fatalf("maximum supported expression AST depth should parse: %v", err)
	}

	tooDeep := "let value = " + strings.Repeat("1 + ", maxSyntaxNestingDepth) + "1;"
	if _, err := Parse(tooDeep, "nesting.ast"); err == nil || !strings.Contains(err.Error(), "expression nesting exceeds limit") {
		t.Fatalf("expected expression AST depth limit error, got %v", err)
	}
}

func TestBlockNestingLimit(t *testing.T) {
	valid := strings.Repeat("{", maxSyntaxNestingDepth) + strings.Repeat("}", maxSyntaxNestingDepth)
	if _, err := Parse(valid, "nesting.ast"); err != nil {
		t.Fatalf("maximum supported block nesting should parse: %v", err)
	}

	tooDeep := strings.Repeat("{", maxSyntaxNestingDepth+1) + strings.Repeat("}", maxSyntaxNestingDepth+1)
	if _, err := Parse(tooDeep, "nesting.ast"); err == nil || !strings.Contains(err.Error(), "syntax nesting exceeds limit") {
		t.Fatalf("expected block nesting limit error, got %v", err)
	}
}

func TestPostfixExpressionASTDepthLimit(t *testing.T) {
	valid := "let value = " + "f()" + strings.Repeat("()", maxSyntaxNestingDepth-2) + ";"
	if _, err := Parse(valid, "nesting.ast"); err != nil {
		t.Fatalf("maximum supported call AST depth should parse: %v", err)
	}

	tooDeep := "let value = " + "f()" + strings.Repeat("()", maxSyntaxNestingDepth-1) + ";"
	if _, err := Parse(tooDeep, "nesting.ast"); err == nil || !strings.Contains(err.Error(), "expression nesting exceeds limit") {
		t.Fatalf("expected postfix expression AST depth limit error, got %v", err)
	}
}
