package bytecode

import (
	"strings"
	"testing"
)

func TestRejectsUnsupportedFormatVersion(t *testing.T) {
	p := &Program{Format: "aster-bytecode", Version: 999, Entry: 0, Functions: []Function{{Name: "main", Locals: 0, Return: "unit", Code: []Instruction{{Op: OpHalt}}}}}
	if err := p.Validate(); err == nil {
		t.Fatal("expected version rejection")
	}
}
func TestRejectsInvalidJump(t *testing.T) {
	p := &Program{Format: "aster-bytecode", Version: FormatVersion, Entry: 0, Functions: []Function{{Name: "main", Return: "unit", Code: []Instruction{{Op: OpJump, A: 99}, {Op: OpHalt}}}}}
	if err := p.Validate(); err == nil {
		t.Fatal("expected invalid jump rejection")
	}
}
func TestRejectsUnknownOpcode(t *testing.T) {
	p := &Program{Format: "aster-bytecode", Version: FormatVersion, Entry: 0, Functions: []Function{{Name: "main", Return: "unit", Code: []Instruction{{Op: "EXEC_SHELL"}, {Op: OpHalt}}}}}
	if err := p.Validate(); err == nil {
		t.Fatal("expected unknown opcode rejection")
	}
}

func TestReadRejectsTrailingJSON(t *testing.T) {
	input := `{"format":"aster-bytecode","version":1,"constants":[],"functions":[{"name":"main","params":0,"locals":0,"return":"unit","code":[{"op":"HALT"}]}],"entry":0} {}`
	if _, err := Read(strings.NewReader(input)); err == nil {
		t.Fatal("expected trailing JSON rejection")
	}
}


func stackVerifierProgram(entry int, constants []Value, functions ...Function) *Program {
	return &Program{
		Format:    "aster-bytecode",
		Version:   FormatVersion,
		Constants: constants,
		Functions: functions,
		Entry:     entry,
	}
}

func TestValidateAcceptsBalancedControlFlowMerge(t *testing.T) {
	p := stackVerifierProgram(0, []Value{Bool(true), I64(10), I64(20)},
		Function{
			Name:   "main",
			Return: "unit",
			Code: []Instruction{
				{Op: OpConst, A: 0},
				{Op: OpJumpIfFalse, A: 4},
				{Op: OpConst, A: 1},
				{Op: OpJump, A: 5},
				{Op: OpConst, A: 2},
				{Op: OpPop},
				{Op: OpHalt},
			},
		},
	)
	if err := p.Validate(); err != nil {
		t.Fatalf("valid balanced branch should pass verification: %v", err)
	}
}

func TestValidateRejectsOperandStackUnderflow(t *testing.T) {
	p := stackVerifierProgram(0, nil,
		Function{
			Name:   "main",
			Return: "unit",
			Code:   []Instruction{{Op: OpPop}, {Op: OpHalt}},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "operand stack underflow") {
		t.Fatalf("expected operand stack underflow error, got %v", err)
	}
}

func TestValidateRejectsInconsistentStackAtControlFlowMerge(t *testing.T) {
	p := stackVerifierProgram(0, []Value{Bool(true), I64(1)},
		Function{
			Name:   "main",
			Return: "unit",
			Code: []Instruction{
				{Op: OpConst, A: 0},
				{Op: OpJumpIfFalse, A: 4},
				{Op: OpConst, A: 1},
				{Op: OpJump, A: 5},
				{Op: OpJump, A: 5},
				{Op: OpHalt},
			},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "inconsistent operand stack heights") {
		t.Fatalf("expected inconsistent stack merge error, got %v", err)
	}
}

func TestValidateRejectsReturnWithExtraStackValues(t *testing.T) {
	p := stackVerifierProgram(1, []Value{I64(1), I64(2)},
		Function{
			Name:   "value",
			Return: "i64",
			Code: []Instruction{
				{Op: OpConst, A: 0},
				{Op: OpConst, A: 1},
				{Op: OpReturn},
			},
		},
		Function{
			Name:   "main",
			Return: "unit",
			Code:   []Instruction{{Op: OpHalt}},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "RETURN requires exactly one stack value") {
		t.Fatalf("expected invalid RETURN stack height error, got %v", err)
	}
}

func TestValidateRejectsHaltWithNonEmptyStack(t *testing.T) {
	p := stackVerifierProgram(0, []Value{I64(1)},
		Function{
			Name:   "main",
			Return: "unit",
			Code:   []Instruction{{Op: OpConst, A: 0}, {Op: OpHalt}},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "HALT requires an empty operand stack") {
		t.Fatalf("expected invalid HALT stack height error, got %v", err)
	}
}

func TestValidateChecksUnreachableCodeForUnderflow(t *testing.T) {
	p := stackVerifierProgram(0, nil,
		Function{
			Name:   "main",
			Return: "unit",
			Code: []Instruction{
				{Op: OpJump, A: 2},
				{Op: OpPop},
				{Op: OpHalt},
			},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "operand stack underflow") {
		t.Fatalf("expected unreachable code underflow error, got %v", err)
	}
}

func TestValidateRejectsFallthroughPastFunctionEnd(t *testing.T) {
	p := stackVerifierProgram(0, []Value{I64(1)},
		Function{
			Name:   "main",
			Return: "unit",
			Code:   []Instruction{{Op: OpConst, A: 0}},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "control flow falls past the end") {
		t.Fatalf("expected fallthrough error, got %v", err)
	}
}
