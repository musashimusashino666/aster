package diagnostic

import (
	"context"
	"errors"
	"go/ast"
	goparser "go/parser"
	gotoken "go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/token"
)

func TestRuntimeCodeAssignsStableFailureClasses(t *testing.T) {
	tests := []struct {
		message string
		want    string
	}{
		{"assertion failed", RuntimeAssertionFailed},
		{"i64 overflow in addition", RuntimeArithmeticOverflow},
		{"division by zero", RuntimeDivisionByZero},
		{"remainder by zero", RuntimeRemainderByZero},
		{"non-finite floating-point result", RuntimeNonFiniteFloat},
		{"array index 4 out of bounds for length 2", RuntimeArrayIndexBounds},
		{"slice bounds [2:1] out of bounds for length 3", RuntimeSliceBounds},
		{"f64 value out of range for i64 conversion", RuntimeNumericConversionRange},
		{"string length limit exceeded (maximum 16 bytes)", RuntimeStringLimit},
		{"printed value exceeds 16-byte output limit", RuntimeRenderedOutputLimit},
		{"stdout byte limit exceeded (maximum 64 bytes)", RuntimeStdoutLimit},
		{"call depth limit exceeded (maximum 4)", RuntimeCallDepthLimit},
		{"instruction budget exceeded (100 steps)", RuntimeInstructionBudget},
		{"operand stack limit exceeded", RuntimeOperandStackLimit},
		{"active local slot limit exceeded", RuntimeLocalSlotLimit},
		{"write output: broken pipe", RuntimeOutputWrite},
		{"unexpected runtime condition", RuntimeGeneric},
	}
	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			if got := RuntimeCode(tt.message); got != tt.want {
				t.Fatalf("RuntimeCode(%q) = %q, want %q", tt.message, got, tt.want)
			}
		})
	}
}

func TestNativeRuntimeDiagnosticClassifierMatchesVM(t *testing.T) {
	messages := []string{
		"assertion failed",
		"i64 overflow in addition",
		"division by zero",
		"remainder by zero",
		"non-finite floating-point result",
		"array index 4 out of bounds for length 2",
		"slice bounds [2:1] out of bounds for length 3",
		"f64 value out of range for i64 conversion",
		"string length limit exceeded (maximum 16 bytes)",
		"printed value exceeds 16-byte output limit",
		"printed array nesting exceeds limit",
		"stdout byte limit exceeded (maximum 64 bytes)",
		"call depth limit exceeded (maximum 4)",
		"instruction budget exceeded (100 steps)",
		"operand stack limit exceeded",
		"active local slot limit exceeded",
		"write output: broken pipe",
		"invalid array element count",
		"array length limit exceeded",
		"unexpected runtime condition",
	}
	for _, message := range messages {
		t.Run(message, func(t *testing.T) {
			gotNative := nativeRuntimeDiagnosticCodeForTest(t, message)
			gotVM := RuntimeCode(message)
			if gotNative != gotVM {
				t.Fatalf("native runtime code = %q, VM runtime code = %q", gotNative, gotVM)
			}
		})
	}
}

func nativeRuntimeDiagnosticCodeForTest(t *testing.T, message string) string {
	t.Helper()
	source, err := os.ReadFile("../codegen/preamble/runtime.go.tmpl")
	if err != nil {
		t.Fatalf("read native runtime template: %v", err)
	}
	sourceText := string(source)
	start := strings.Index(sourceText, "func asterRuntimeDiagnosticCode")
	if start < 0 {
		t.Fatal("native runtime classifier function was not found")
	}
	relativeEnd := strings.Index(sourceText[start:], "\nfunc asterRuntimeError")
	if relativeEnd < 0 {
		t.Fatal("native runtime error handler was not found after classifier")
	}
	end := start + relativeEnd
	file, err := goparser.ParseFile(gotoken.NewFileSet(),
		"runtime_classifier.go",
		"package runtimeclassifier\nimport \"strings\"\n"+sourceText[start:end],
		0)
	if err != nil {
		t.Fatalf("parse native runtime classifier: %v", err)
	}
	var classifier *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "asterRuntimeDiagnosticCode" {
			classifier = fn
			break
		}
	}
	if classifier == nil {
		t.Fatal("parsed native runtime classifier was not found")
	}
	for _, statement := range classifier.Body.List {
		sw, ok := statement.(*ast.SwitchStmt)
		if !ok {
			continue
		}
		for _, item := range sw.Body.List {
			clause, ok := item.(*ast.CaseClause)
			if !ok {
				continue
			}
			matched := len(clause.List) == 0
			for _, condition := range clause.List {
				if nativeClassifierConditionMatches(condition, message) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			for _, body := range clause.Body {
				ret, ok := body.(*ast.ReturnStmt)
				if !ok || len(ret.Results) != 1 {
					continue
				}
				literal, ok := ret.Results[0].(*ast.BasicLit)
				if !ok || literal.Kind != gotoken.STRING {
					t.Fatalf("unexpected return in native runtime classifier: %#v", body)
				}
				code, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatalf("decode native diagnostic code: %v", err)
				}
				return code
			}
		}
	}
	t.Fatalf("no classifier case matched %q", message)
	return ""
}

func nativeClassifierConditionMatches(expr ast.Expr, message string) bool {
	switch condition := expr.(type) {
	case *ast.BinaryExpr:
		switch condition.Op {
		case gotoken.LAND:
			return nativeClassifierConditionMatches(condition.X, message) &&
				nativeClassifierConditionMatches(condition.Y, message)
		case gotoken.LOR:
			return nativeClassifierConditionMatches(condition.X, message) ||
				nativeClassifierConditionMatches(condition.Y, message)
		}
	case *ast.CallExpr:
		selector, ok := condition.Fun.(*ast.SelectorExpr)
		if !ok || len(condition.Args) != 2 {
			return false
		}
		value, ok := condition.Args[1].(*ast.BasicLit)
		if !ok || value.Kind != gotoken.STRING {
			return false
		}
		fragment, err := strconv.Unquote(value.Value)
		if err != nil {
			return false
		}
		switch selector.Sel.Name {
		case "Contains":
			return strings.Contains(message, fragment)
		case "HasPrefix":
			return strings.HasPrefix(message, fragment)
		}
	}
	return false
}

func TestBytecodeCodeAssignsStableFailureClasses(t *testing.T) {
	tests := []struct {
		message string
		want    string
	}{
		{"not an Aster bytecode file", BytecodeFormat},
		{"unsupported bytecode version 99 (supported: 5)", BytecodeVersion},
		{"constant pool exceeds safety limit", BytecodeResourceLimit},
		{"invalid JSON trailing JSON value", BytecodeDecode},
		{"function \"main\" instruction 2 has invalid jump target", BytecodeStructure},
		{"abstract operand stack types do not agree", BytecodeTypeValidation},
		{"unrecognized bytecode reader error", BytecodeGeneric},
	}
	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			if got := BytecodeCode(tt.message); got != tt.want {
				t.Fatalf("BytecodeCode(%q) = %q, want %q", tt.message, got, tt.want)
			}
		})
	}
}

func TestDiagnosticIncludesSourceAndStartPosition(t *testing.T) {
	err := NewAtSource(RuntimeDivisionByZero, "runtime", "examples/division.ast", token.Position{Line: 4, Column: 9}, "division by zero")
	const want = "examples/division.ast:4:9: runtime error: division by zero [RUN003]"
	if got := err.Error(); got != want {
		t.Fatalf("diagnostic = %q, want %q", got, want)
	}
}

func TestWrapToolchainPreservesCancellation(t *testing.T) {
	err := WrapToolchain(errors.New("native program timed out: context deadline exceeded"))
	if got := ToolchainCode(err.Error()); got != ToolchainTimeout {
		t.Fatalf("wrapped toolchain code = %q, want %q (%v)", got, ToolchainTimeout, err)
	}

	cancelled := WrapToolchain(context.Canceled)
	if !errors.Is(cancelled, context.Canceled) {
		t.Fatalf("wrapped diagnostic does not preserve context.Canceled: %v", cancelled)
	}
	if !strings.Contains(cancelled.Error(), "[TOOL001]") {
		t.Fatalf("wrapped cancellation is missing stable code: %v", cancelled)
	}
}


func TestNewSpanRetainsExclusiveEndWithoutChangingErrorFormat(t *testing.T) {
	start := token.Position{Offset: 2, Line: 1, Column: 3}
	end := token.Position{Offset: 5, Line: 1, Column: 6}
	got := NewSpan(ParserGeneric, "parser", start, end, "unexpected token")
	if !got.HasEndPosition || got.EndPosition != end {
		t.Fatalf("diagnostic span = (%+v, has=%v), want end %+v", got.EndPosition, got.HasEndPosition, end)
	}
	if got.Position != start {
		t.Fatalf("diagnostic start = %+v, want %+v", got.Position, start)
	}
	const want = "1:3: parser error: unexpected token [PAR000]"
	if got.Error() != want {
		t.Fatalf("Error() = %q, want %q", got.Error(), want)
	}
}

func TestNewAtSourceSpanRetainsSourcePathAndEnd(t *testing.T) {
	start := token.Position{Offset: 10, Line: 2, Column: 4}
	end := token.Position{Offset: 13, Line: 2, Column: 7}
	got := NewAtSourceSpan(RuntimeGeneric, "runtime", "example.ast", start, end, "runtime failed")
	if !got.HasEndPosition || got.EndPosition != end {
		t.Fatalf("diagnostic span = (%+v, has=%v), want end %+v", got.EndPosition, got.HasEndPosition, end)
	}
	const want = "example.ast:2:4: runtime error: runtime failed [RUN000]"
	if got.Error() != want {
		t.Fatalf("Error() = %q, want %q", got.Error(), want)
	}
}
