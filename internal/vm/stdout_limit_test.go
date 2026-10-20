package vm

import (
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/bytecode"
)

// stdoutCountWriter counts bytes without retaining the output.
type stdoutCountWriter struct {
	written int64
}

func (w *stdoutCountWriter) Write(p []byte) (int, error) {
	w.written += int64(len(p))
	return len(p), nil
}

func TestVMCumulativeStdoutLimitRejectsPrintBeforeWriting(t *testing.T) {
	large := strings.Repeat("x", bytecode.MaxStringBytes)
	code := make([]bytecode.Instruction, 0, 13)
	for i := 0; i < 4; i++ {
		code = append(code,
			bytecode.Instruction{Op: bytecode.OpConst, A: 0},
			bytecode.Instruction{Op: bytecode.OpBuiltin, A: 0, B: 1},
			bytecode.Instruction{Op: bytecode.OpPop},
		)
	}
	code = append(code, bytecode.Instruction{Op: bytecode.OpHalt})
	program := &bytecode.Program{
		Format:    "aster-bytecode",
		Version:   bytecode.FormatVersion,
		Constants: []bytecode.Value{bytecode.Str(large)},
		Functions: []bytecode.Function{{
			Name:   "main",
			Return: "unit",
			Code:   code,
		}},
		Entry: 0,
	}
	out := &stdoutCountWriter{}
	err := Run(program, out)
	if err == nil || !strings.Contains(err.Error(), "stdout byte limit exceeded") {
		t.Fatalf("expected cumulative stdout limit error, got %v", err)
	}
	// The first three 16 MiB prints fit. The fourth must be rejected before
	// even one byte of that print is written.
	want := int64(3) * (int64(bytecode.MaxStringBytes) + 1)
	if out.written != want {
		t.Fatalf("writer received %d bytes, want %d; rejected print may have written partially", out.written, want)
	}
}
