package diagnostic

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	goparser "go/parser"
	gotoken "go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/musashimusashino666/aster/internal/token"
)

func TestRuntimeFailureCarriesExplicitCodeAndMessage(t *testing.T) {
	tests := []struct {
		code    string
		message string
	}{
		{RuntimeAssertionFailed, "assertion failed"},
		{RuntimeArithmeticOverflow, "i64 overflow in addition"},
		{RuntimeDivisionByZero, "division by zero"},
		{RuntimeRemainderByZero, "remainder by zero"},
		{RuntimeNonFiniteFloat, "non-finite floating-point result"},
		{RuntimeArrayIndexBounds, "array index 4 out of bounds for length 2"},
		{RuntimeSliceBounds, "slice bounds [2:1] out of bounds for length 3"},
		{RuntimeNumericConversionRange, "f64 value out of range for i64 conversion"},
		{RuntimeStringLimit, "string length limit exceeded (maximum 16 bytes)"},
		{RuntimeRenderedOutputLimit, "printed value exceeds 16-byte output limit"},
		{RuntimeStdoutLimit, "stdout byte limit exceeded (maximum 64 bytes)"},
		{RuntimeCallDepthLimit, "call depth limit exceeded (maximum 4)"},
		{RuntimeInstructionBudget, "instruction budget exceeded (100 steps)"},
		{RuntimeProcessMemoryLimit, "process memory limit exceeded"},
		{RuntimeCPUTimeLimit, "cumulative CPU-time budget exceeded"},
		{RuntimeOperandStackLimit, "operand stack limit exceeded"},
		{RuntimeLocalSlotLimit, "active local slot limit exceeded"},
		{RuntimeOutputWrite, "write output: broken pipe"},
		{RuntimeGeneric, "unexpected runtime condition"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			failure := NewRuntimeFailure(tt.code, tt.message)
			if failure.Code != tt.code {
				t.Fatalf("runtime failure code = %q, want %q", failure.Code, tt.code)
			}
			if got := failure.Error(); got != tt.message {
				t.Fatalf("runtime failure message = %q, want %q", got, tt.message)
			}
		})
	}
}

func TestNativeRuntimeErrorSitesSupplyExplicitCodes(t *testing.T) {
	source, err := os.ReadFile("../codegen/preamble/runtime.go.tmpl")
	if err != nil {
		t.Fatalf("read native runtime template: %v", err)
	}
	if strings.Contains(string(source), "asterRuntimeDiagnosticCode") {
		t.Fatal("native runtime must not classify diagnostics by matching message text")
	}
	file, err := goparser.ParseFile(gotoken.NewFileSet(), "runtime.go.tmpl", source, 0)
	if err != nil {
		t.Fatalf("parse native runtime template: %v", err)
	}

	count := 0
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name, ok := call.Fun.(*ast.Ident)
		if !ok || name.Name != "asterRuntimeError" {
			return true
		}
		if len(call.Args) != 4 {
			t.Errorf("asterRuntimeError call has %d arguments, want explicit code plus line, column, and message", len(call.Args))
			return true
		}
		literal, ok := call.Args[0].(*ast.BasicLit)
		if !ok || literal.Kind != gotoken.STRING {
			t.Errorf("asterRuntimeError call does not pass a literal diagnostic code: %#v", call.Args[0])
			return true
		}
		code, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Errorf("decode native runtime diagnostic code: %v", err)
			return true
		}
		if !strings.HasPrefix(code, "RUN") {
			t.Errorf("native runtime diagnostic code = %q, want RUNxxx", code)
		}
		count++
		return true
	})
	if count == 0 {
		t.Fatal("no explicit native runtime error sites were found")
	}
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

func TestRenderIncludesExclusiveRangeWithoutChangingError(t *testing.T) {
	start := token.Position{Offset: 10, Line: 2, Column: 4}
	end := token.Position{Offset: 13, Line: 2, Column: 7}
	err := NewAtSourceSpan(RuntimeGeneric, "runtime", "example.ast", start, end, "runtime failed")

	const legacy = "example.ast:2:4: runtime error: runtime failed [RUN000]"
	if got := err.Error(); got != legacy {
		t.Fatalf("Error() = %q, want legacy form %q", got, legacy)
	}
	const want = "example.ast:2:4-2:7: runtime error: runtime failed [RUN000]"
	if got := Render(err); got != want {
		t.Fatalf("Render() = %q, want %q", got, want)
	}
}

func TestRenderUsesSyntacticRangeStartInsteadOfDiagnosticAnchor(t *testing.T) {
	anchor := token.Position{Offset: 3, Line: 1, Column: 4}
	rangeStart := token.Position{Offset: 0, Line: 1, Column: 1}
	end := token.Position{Offset: 8, Line: 1, Column: 9}
	err := NewAtSourceRange(RuntimeDivisionByZero, "runtime", "expr.ast",
		anchor, rangeStart, end, "division by zero")

	const legacy = "expr.ast:1:4: runtime error: division by zero [RUN003]"
	if got := err.Error(); got != legacy {
		t.Fatalf("Error() = %q, want diagnostic-anchor form %q", got, legacy)
	}
	const want = "expr.ast:1:1-1:9: runtime error: division by zero [RUN003]"
	if got := Render(err); got != want {
		t.Fatalf("Render() = %q, want syntactic source range %q", got, want)
	}
}

func TestRenderPreservesWrappedPrefixesAndRejectsInvalidRanges(t *testing.T) {
	start := token.Position{Offset: 10, Line: 2, Column: 4}
	end := token.Position{Offset: 13, Line: 2, Column: 7}
	wrapped := fmt.Errorf("example.ast: %w", NewSpan(ParserGeneric, "parser", start, end, "unexpected token"))
	const want = "example.ast: 2:4-2:7: parser error: unexpected token [PAR000]"
	if got := Render(wrapped); got != want {
		t.Fatalf("Render(wrapped) = %q, want %q", got, want)
	}

	invalid := NewSpan(ParserGeneric, "parser", start,
		token.Position{Offset: 9, Line: 2, Column: 7}, "unexpected token")
	if got, legacy := Render(invalid), invalid.Error(); got != legacy {
		t.Fatalf("Render(invalid range) = %q, want start-only fallback %q", got, legacy)
	}
}

func TestToolchainCodeUsesSupportedGoMinimum(t *testing.T) {
	messages := []string{
		`native backend requires Go 1.26 or newer; executable "go" was not found in PATH`,
		"native backend requires gofmt (normally installed with Go 1.26 or newer)",
	}
	for _, message := range messages {
		if got := ToolchainCode(message); got != ToolchainUnavailable {
			t.Errorf("ToolchainCode(%q) = %q, want %q", message, got, ToolchainUnavailable)
		}
	}
}

func TestResourceIsolationDiagnosticCodes(t *testing.T) {
	memory := NewRuntimeFailure(RuntimeProcessMemoryLimit, "process memory limit exceeded")
	if memory.Code != RuntimeProcessMemoryLimit {
		t.Fatalf("memory failure code = %q, want %q", memory.Code, RuntimeProcessMemoryLimit)
	}
	cpu := NewRuntimeFailure(RuntimeCPUTimeLimit, "cumulative CPU-time budget exceeded")
	if cpu.Code != RuntimeCPUTimeLimit {
		t.Fatalf("CPU-time failure code = %q, want %q", cpu.Code, RuntimeCPUTimeLimit)
	}
	if got := ToolchainCode("resource isolation unavailable: cgroup v2 is missing"); got != ToolchainResourceIsolationUnavailable {
		t.Fatalf("ToolchainCode(resource isolation) = %q, want %q", got, ToolchainResourceIsolationUnavailable)
	}
}
