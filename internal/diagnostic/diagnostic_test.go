package diagnostic

import (
	"context"
	"errors"
	"testing"
	"strings"

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

