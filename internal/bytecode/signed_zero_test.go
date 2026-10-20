package bytecode

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func TestReadPreservesNegativeZeroF64Constant(t *testing.T) {
	negativeZero := math.Copysign(0, -1)
	program := &Program{
		Format:    "aster-bytecode",
		Version:   FormatVersion,
		Constants: []Value{{Kind: ValueF64, F64: negativeZero}},
		Functions: []Function{{
			Name:   "main",
			Return: "unit",
			Code: []Instruction{
				{Op: OpConst, A: 0},
				{Op: OpPop},
				{Op: OpHalt},
			},
		}},
		Entry: 0,
	}

	var encoded bytes.Buffer
	if err := Write(&encoded, program); err != nil {
		t.Fatalf("write bytecode: %v", err)
	}
	if !strings.Contains(encoded.String(), `"f64": -0`) {
		t.Fatalf("serialized bytecode does not preserve negative zero: %s", encoded.String())
	}

	decoded, err := Read(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("read bytecode: %v", err)
	}
	if !math.Signbit(decoded.Constants[0].F64) {
		t.Fatalf("negative-zero sign bit was lost during bytecode round trip: %#v", decoded.Constants[0].F64)
	}
}
