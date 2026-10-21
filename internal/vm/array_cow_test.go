package vm

import (
	"bytes"
	"testing"

	"github.com/musashimusashino666/aster/internal/bytecode"
)

func TestVMConstantPoolArraysDetachOnWrite(t *testing.T) {
	program := &bytecode.Program{
		Format:  "aster-bytecode",
		Version: bytecode.FormatVersion,
		Constants: []bytecode.Value{
			bytecode.Array([]bytecode.Value{
				bytecode.Array([]bytecode.Value{bytecode.I64(1), bytecode.I64(2)}),
				bytecode.Array([]bytecode.Value{bytecode.I64(3), bytecode.I64(4)}),
			}),
			bytecode.I64(0),
			bytecode.I64(1),
			bytecode.I64(9),
		},
		Functions: []bytecode.Function{{
			Name:   "__aster_main",
			Params: 0,
			Locals: 2,
			Return: "unit",
			Code: []bytecode.Instruction{
				{Op: bytecode.OpConst, A: 0},
				{Op: bytecode.OpStoreLocal, A: 0},
				{Op: bytecode.OpConst, A: 0},
				{Op: bytecode.OpStoreLocal, A: 1},

				{Op: bytecode.OpLoadLocal, A: 0},
				{Op: bytecode.OpConst, A: 1},
				{Op: bytecode.OpConst, A: 2},
				{Op: bytecode.OpConst, A: 3},
				{Op: bytecode.OpSetIndexPath, A: 2},
				{Op: bytecode.OpStoreLocal, A: 0},

				{Op: bytecode.OpLoadLocal, A: 0},
				{Op: bytecode.OpBuiltin, A: 0, B: 1},
				{Op: bytecode.OpPop},
				{Op: bytecode.OpLoadLocal, A: 1},
				{Op: bytecode.OpBuiltin, A: 0, B: 1},
				{Op: bytecode.OpPop},
				{Op: bytecode.OpConst, A: 0},
				{Op: bytecode.OpBuiltin, A: 0, B: 1},
				{Op: bytecode.OpPop},
				{Op: bytecode.OpHalt},
			},
		}},
		Entry: 0,
	}

	var output bytes.Buffer
	if err := Run(program, &output); err != nil {
		t.Fatalf("VM execution failed: %v; output=%q", err, output.String())
	}
	const want = "[[1 9] [3 4]]\n[[1 2] [3 4]]\n[[1 2] [3 4]]\n"
	if got := output.String(); got != want {
		t.Fatalf("unexpected output: got %q, want %q", got, want)
	}
	if got := program.Constants[0].StringValue(); got != "[[1 2] [3 4]]" {
		t.Fatalf("runtime mutation changed the constant pool: got %q", got)
	}
}
