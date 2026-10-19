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
