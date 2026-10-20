package tests

import (
	"bytes"
	"math"
	"testing"

	"github.com/aster-lang/aster/internal/bytecode"
	"github.com/aster-lang/aster/internal/vm"
)

func TestNegativeZeroSurvivesBytecodeRoundTripAndExecution(t *testing.T) {
	program := &bytecode.Program{
		Format:    "aster-bytecode",
		Version:   bytecode.FormatVersion,
		Constants: []bytecode.Value{{Kind: bytecode.ValueF64, F64: math.Copysign(0, -1)}},
		Functions: []bytecode.Function{{
			Name:   "main",
			Return: "unit",
			Code: []bytecode.Instruction{
				{Op: bytecode.OpConst, A: 0},
				{Op: bytecode.OpBuiltin, A: 0, B: 1},
				{Op: bytecode.OpPop},
				{Op: bytecode.OpHalt},
			},
		}},
		Entry: 0,
	}
	var encoded bytes.Buffer
	if err := bytecode.Write(&encoded, program); err != nil {
		t.Fatalf("write bytecode: %v", err)
	}
	decoded, err := bytecode.Read(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("read bytecode: %v", err)
	}
	var output bytes.Buffer
	if err := vm.Run(decoded, &output); err != nil {
		t.Fatalf("execute decoded bytecode: %v", err)
	}
	if got, want := output.String(), "-0\n"; got != want {
		t.Fatalf("negative-zero output after bytecode round trip = %q, want %q", got, want)
	}
}
