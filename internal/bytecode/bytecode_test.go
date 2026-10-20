package bytecode

import (
	"bytes"
	"encoding/json"
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
	input := `{"format":"aster-bytecode","version":5,"constants":[],"functions":[{"name":"main","params":0,"locals":0,"return":"unit","code":[{"op":"HALT"}]}],"entry":0} {}`
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

func FuzzReadBytecodeNeverPanics(f *testing.F) {
	f.Add(`{"format":"aster-bytecode","version":5,"constants":[],"functions":[{"name":"main","params":0,"locals":0,"return":"unit","code":[{"op":"HALT"}]}],"entry":0}`)
	f.Add("")
	f.Add("{}")
	f.Add(`{"format":"aster-bytecode","version":5,"constants":[{"kind":"array","array":[]}],"functions":[{"name":"main","params":0,"locals":0,"return":"unit","code":[{"op":"HALT"}]}],"entry":0}`)
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = Read(bytes.NewReader([]byte(input)))
	})
}

func FuzzValidateInstructionOperandsNeverPanics(f *testing.F) {
	f.Add(uint8(0), int16(0), int16(0))
	f.Add(uint8(1), int16(-1), int16(3))
	f.Add(uint8(255), int16(32767), int16(-32768))
	ops := []Op{
		OpConst, OpLoadLocal, OpBuildArray, OpIndex, OpSlice, OpSetIndexPath, OpToI64, OpToF64, OpStoreLocal, OpPop,
		OpNeg, OpNot, OpAdd, OpSub, OpMul, OpDiv, OpMod, OpEqual,
		OpNotEqual, OpLess, OpLessEqual, OpGreater, OpGreaterEqual,
		OpJump, OpJumpIfFalse, OpJumpIfTrue, OpCall, OpBuiltin, OpReturn,
		OpHalt, "UNKNOWN",
	}
	f.Fuzz(func(t *testing.T, opByte uint8, a, b int16) {
		op := ops[int(opByte)%len(ops)]
		program := &Program{
			Format:    "aster-bytecode",
			Version:   FormatVersion,
			Constants: []Value{Unit(), I64(1), Bool(true), Str("seed")},
			Functions: []Function{{
				Name:   "main",
				Params: 0,
				Locals: 2,
				Return: "unit",
				Code: []Instruction{
					{Op: op, A: int(a), B: int(b)},
					{Op: OpHalt},
				},
			}},
			Entry: 0,
		}
		_ = program.Validate()
	})
}

func TestValidateRejectsOversizedStringConstant(t *testing.T) {
	p := stackVerifierProgram(0, []Value{Str(strings.Repeat("x", MaxStringBytes+1))},
		Function{
			Name:   "main",
			Return: "unit",
			Code:   []Instruction{{Op: OpHalt}},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "string exceeds") {
		t.Fatalf("expected string size limit error, got %v", err)
	}
}

func TestValidateRejectsOversizedArrayConstant(t *testing.T) {
	oversized := Value{Kind: ValueArray, Array: make([]Value, MaxArrayLength+1)}
	p := stackVerifierProgram(0, []Value{oversized},
		Function{
			Name:   "main",
			Return: "unit",
			Code:   []Instruction{{Op: OpHalt}},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "maximum is") {
		t.Fatalf("expected array size limit error, got %v", err)
	}
}

func TestReadPreflightsOversizedConstantArrayBeforeDecode(t *testing.T) {
	items := strings.Repeat("null,", MaxArrayLength)
	input := "{\"format\":\"aster-bytecode\",\"version\":5,\"constants\":[{\"kind\":\"array\",\"array\":[" + items + "null]}],\"functions\":[{\"name\":\"main\",\"params\":0,\"locals\":0,\"return\":\"unit\",\"code\":[{\"op\":\"HALT\"}]}],\"entry\":0}"
	_, err := Read(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "bytecode preflight: constant array exceeds") {
		t.Fatalf("expected pre-decode array limit error, got %v", err)
	}
}

func TestPreflightRejectsOversizedInstructionArray(t *testing.T) {
	raw := []byte("{\"functions\":[{\"code\":[{},{}]}]}")
	limits := &bytecodePreflight{instructionCount: maxTotalInstructions}
	dec := json.NewDecoder(bytes.NewReader(raw))
	_, _ = dec.Token() // root object
	_, _ = dec.Token() // "functions"
	_, _ = dec.Token() // functions array
	_, _ = dec.Token() // function object
	_, _ = dec.Token() // "code"
	if err := preflightInstructions(dec, limits); err == nil || !strings.Contains(err.Error(), "instruction safety limit") {
		t.Fatalf("expected instruction cap error, got %v", err)
	}
}

func TestValidateRejectsWrongArithmeticTypes(t *testing.T) {
	p := stackVerifierProgram(0, []Value{Bool(true), I64(1)},
		Function{
			Name:   "main",
			Return: "unit",
			Code: []Instruction{
				{Op: OpConst, A: 0},
				{Op: OpConst, A: 1},
				{Op: OpAdd},
				{Op: OpPop},
				{Op: OpHalt},
			},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "ADD requires") {
		t.Fatalf("expected arithmetic type error, got %v", err)
	}
}

func TestValidateRejectsUninitializedLocalRead(t *testing.T) {
	p := stackVerifierProgram(0, []Value{I64(1)},
		Function{
			Name:   "main",
			Locals: 1,
			Return: "unit",
			Code: []Instruction{
				{Op: OpLoadLocal, A: 0},
				{Op: OpPop},
				{Op: OpHalt},
			},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "before initialization") {
		t.Fatalf("expected uninitialized local rejection, got %v", err)
	}
}

func TestValidateRejectsLocalInitializedOnOnlyOneBranch(t *testing.T) {
	p := stackVerifierProgram(0, []Value{Bool(true), I64(1)},
		Function{
			Name:   "main",
			Locals: 1,
			Return: "unit",
			Code: []Instruction{
				{Op: OpConst, A: 0},
				{Op: OpJumpIfFalse, A: 4},
				{Op: OpConst, A: 1},
				{Op: OpStoreLocal, A: 0},
				{Op: OpJump, A: 5},
				{Op: OpLoadLocal, A: 0},
				{Op: OpPop},
				{Op: OpHalt},
			},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "before initialization") {
		t.Fatalf("expected local definite-initialization rejection, got %v", err)
	}
}

func TestValidateRejectsOperandStackTypeMismatchAtMerge(t *testing.T) {
	p := stackVerifierProgram(0, []Value{Bool(true), I64(1), Str("x")},
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
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "operand stack type mismatch") {
		t.Fatalf("expected stack type merge rejection, got %v", err)
	}
}

func TestValidateAcceptsTypedFunctionCall(t *testing.T) {
	p := stackVerifierProgram(1, []Value{I64(7)},
		Function{
			Name:       "identity",
			Params:     1,
			ParamTypes: []string{"i64"},
			Locals:     1,
			Return:     "i64",
			Code: []Instruction{
				{Op: OpLoadLocal, A: 0},
				{Op: OpReturn},
			},
		},
		Function{
			Name:   "main",
			Return: "unit",
			Code: []Instruction{
				{Op: OpConst, A: 0},
				{Op: OpCall, A: 0, B: 1},
				{Op: OpPop},
				{Op: OpHalt},
			},
		},
	)
	if err := p.Validate(); err != nil {
		t.Fatalf("expected typed function call to pass, got %v", err)
	}
}

func TestValidateRejectsTypedFunctionCallMismatch(t *testing.T) {
	p := stackVerifierProgram(1, []Value{Bool(true)},
		Function{
			Name:       "identity",
			Params:     1,
			ParamTypes: []string{"i64"},
			Locals:     1,
			Return:     "i64",
			Code: []Instruction{
				{Op: OpLoadLocal, A: 0},
				{Op: OpReturn},
			},
		},
		Function{
			Name:   "main",
			Return: "unit",
			Code: []Instruction{
				{Op: OpConst, A: 0},
				{Op: OpCall, A: 0, B: 1},
				{Op: OpPop},
				{Op: OpHalt},
			},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "expected i64") {
		t.Fatalf("expected function argument type rejection, got %v", err)
	}
}

func TestValidateRejectsReturnTypeMismatch(t *testing.T) {
	p := stackVerifierProgram(1, []Value{Bool(true)},
		Function{
			Name:   "value",
			Return: "i64",
			Code: []Instruction{
				{Op: OpConst, A: 0},
				{Op: OpReturn},
			},
		},
		Function{
			Name:   "main",
			Return: "unit",
			Code:   []Instruction{{Op: OpHalt}},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "RETURN type mismatch") {
		t.Fatalf("expected return type rejection, got %v", err)
	}
}

func TestValidateRejectsNonBoolConditional(t *testing.T) {
	p := stackVerifierProgram(0, []Value{I64(1)},
		Function{
			Name:   "main",
			Return: "unit",
			Code: []Instruction{
				{Op: OpConst, A: 0},
				{Op: OpJumpIfFalse, A: 2},
				{Op: OpHalt},
			},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "requires bool") {
		t.Fatalf("expected conditional type rejection, got %v", err)
	}
}

func TestValidateRejectsHeterogeneousArray(t *testing.T) {
	p := stackVerifierProgram(0, []Value{I64(1), Str("x")},
		Function{
			Name:   "main",
			Return: "unit",
			Code: []Instruction{
				{Op: OpConst, A: 0},
				{Op: OpConst, A: 1},
				{Op: OpBuildArray, A: 2},
				{Op: OpPop},
				{Op: OpHalt},
			},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "array elements have inconsistent types") {
		t.Fatalf("expected heterogeneous array rejection, got %v", err)
	}
}

func TestValidateRejectsMissingParameterTypeMetadata(t *testing.T) {
	p := stackVerifierProgram(1, []Value{I64(7)},
		Function{
			Name:   "identity",
			Params: 1,
			Locals: 1,
			Return: "i64",
			Code: []Instruction{
				{Op: OpLoadLocal, A: 0},
				{Op: OpReturn},
			},
		},
		Function{Name: "main", Return: "unit", Code: []Instruction{{Op: OpHalt}}},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "parameter types") {
		t.Fatalf("expected missing parameter metadata rejection, got %v", err)
	}
}

func TestReadRejectsLegacyBytecodeVersion(t *testing.T) {
	input := `{"format":"aster-bytecode","version":1,"constants":[],"functions":[{"name":"main","params":0,"locals":0,"return":"unit","code":[{"op":"HALT"}]}],"entry":0}`
	if _, err := Read(strings.NewReader(input)); err == nil || !strings.Contains(err.Error(), "unsupported bytecode version") {
		t.Fatalf("expected legacy format rejection, got %v", err)
	}
}

func TestValidateRejectsArrayOfUnitParameterType(t *testing.T) {
	p := stackVerifierProgram(1, nil,
		Function{
			Name:       "takesArray",
			Params:     1,
			ParamTypes: []string{"Array<unit>"},
			Locals:     1,
			Return:     "unit",
			Code: []Instruction{
				{Op: OpConst, A: 0},
				{Op: OpPop},
				{Op: OpReturn},
			},
		},
		Function{Name: "main", Return: "unit", Code: []Instruction{{Op: OpHalt}}},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "invalid type") {
		t.Fatalf("expected Array<unit> metadata rejection, got %v", err)
	}
}

func TestValidateRejectsInconsistentArrayConstant(t *testing.T) {
	p := stackVerifierProgram(0, []Value{Array([]Value{I64(1), Str("wrong")})},
		Function{Name: "main", Return: "unit", Code: []Instruction{{Op: OpHalt}}},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "inconsistent types") {
		t.Fatalf("expected heterogeneous array constant rejection, got %v", err)
	}
}

func TestValidateRejectsLocalTypeChange(t *testing.T) {
	p := stackVerifierProgram(0, []Value{I64(1), Str("wrong")},
		Function{
			Name:   "main",
			Locals: 1,
			Return: "unit",
			Code: []Instruction{
				{Op: OpConst, A: 0},
				{Op: OpStoreLocal, A: 0},
				{Op: OpConst, A: 1},
				{Op: OpStoreLocal, A: 0},
				{Op: OpHalt},
			},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "cannot store string") {
		t.Fatalf("expected local type change rejection, got %v", err)
	}
}

func TestValidateRejectsExcessivelyNestedParameterType(t *testing.T) {
	nested := strings.Repeat("Array<", maxTypeNestingDepth+1) + "i64" + strings.Repeat(">", maxTypeNestingDepth+1)
	p := stackVerifierProgram(1, nil,
		Function{
			Name:       "takesNestedArray",
			Params:     1,
			ParamTypes: []string{nested},
			Locals:     1,
			Return:     "unit",
			Code:       []Instruction{{Op: OpReturn}},
		},
		Function{Name: "main", Return: "unit", Code: []Instruction{{Op: OpHalt}}},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "invalid type") {
		t.Fatalf("expected type nesting limit rejection, got %v", err)
	}
}

func TestValidateRejectsUnitLocalStorage(t *testing.T) {
	p := stackVerifierProgram(0, []Value{Unit()},
		Function{
			Name:   "main",
			Locals: 1,
			Return: "unit",
			Code: []Instruction{
				{Op: OpConst, A: 0},
				{Op: OpStoreLocal, A: 0},
				{Op: OpHalt},
			},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "locals cannot hold unit") {
		t.Fatalf("expected unit local storage rejection, got %v", err)
	}
}

func TestValidateRejectsExcessiveTypedVerifierState(t *testing.T) {
	p := stackVerifierProgram(0, nil,
		Function{
			Name:   "main",
			Locals: maxTypedVerifierStateCells/2 + 1,
			Return: "unit",
			Code: []Instruction{
				{Op: OpHalt},
				{Op: OpHalt},
			},
		},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "typed verifier state safety limit") {
		t.Fatalf("expected typed verifier state limit rejection, got %v", err)
	}
}

func TestPreflightRejectsExcessiveParameterTypeMetadata(t *testing.T) {
	dec := json.NewDecoder(bytes.NewBufferString(`{"param_types":["i64"]}`))
	limits := &bytecodePreflight{parameterTypes: maxTotalParameterTypes}
	if err := preflightFunction(dec, limits); err == nil || !strings.Contains(err.Error(), "parameter type metadata") {
		t.Fatalf("expected parameter type metadata limit error, got %v", err)
	}
}

func TestPreflightRejectsOversizedTypeName(t *testing.T) {
	typ := strings.Repeat("x", maxTypeNameBytes+1)
	input := `{"functions":[{"return":"` + typ + `"}]}`
	if err := preflightBytecodeLimits([]byte(input)); err == nil || !strings.Contains(err.Error(), "type name exceeds") {
		t.Fatalf("expected oversized type name rejection, got %v", err)
	}
}

func TestValidateRejectsExcessivelyNestedBuiltArrayType(t *testing.T) {
	code := []Instruction{{Op: OpConst, A: 0}}
	for i := 0; i <= maxTypeNestingDepth; i++ {
		code = append(code, Instruction{Op: OpBuildArray, A: 1})
	}
	code = append(code, Instruction{Op: OpPop}, Instruction{Op: OpHalt})
	p := stackVerifierProgram(0, []Value{I64(1)},
		Function{Name: "main", Return: "unit", Code: code},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "array type nesting exceeds") {
		t.Fatalf("expected array type nesting limit rejection, got %v", err)
	}
}

func TestValidateRejectsInvalidIndexedAssignmentDepth(t *testing.T) {
	p := stackVerifierProgram(0, nil,
		Function{Name: "main", Locals: 1, Return: "unit", Code: []Instruction{
			{Op: OpSetIndexPath, A: 0},
			{Op: OpHalt},
		}},
	)
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "invalid indexed-assignment depth") {
		t.Fatalf("expected invalid indexed-assignment depth error, got %v", err)
	}
}

func TestValidateRejectsIndexedAssignmentValueTypeMismatch(t *testing.T) {
	p := stackVerifierProgram(0, []Value{
		Array([]Value{I64(1), I64(2)}),
		I64(0),
		Str("wrong"),
	}, Function{Name: "main", Locals: 1, Return: "unit", Code: []Instruction{
		{Op: OpConst, A: 0},
		{Op: OpStoreLocal, A: 0},
		{Op: OpLoadLocal, A: 0},
		{Op: OpConst, A: 1},
		{Op: OpConst, A: 2},
		{Op: OpSetIndexPath, A: 1},
		{Op: OpStoreLocal, A: 0},
		{Op: OpHalt},
	}})
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "indexed assignment value has type string; expected i64") {
		t.Fatalf("expected indexed-assignment type mismatch, got %v", err)
	}
}

func TestValidateAcceptsTypedSlice(t *testing.T) {
	p := stackVerifierProgram(0, []Value{
		Array([]Value{I64(1), I64(2), I64(3)}),
		I64(1),
		I64(3),
	}, Function{Name: "main", Locals: 1, Return: "unit", Code: []Instruction{
		{Op: OpConst, A: 0},
		{Op: OpConst, A: 1},
		{Op: OpConst, A: 2},
		{Op: OpSlice},
		{Op: OpStoreLocal, A: 0},
		{Op: OpHalt},
	}})
	if err := p.Validate(); err != nil {
		t.Fatalf("expected typed slice to validate: %v", err)
	}
}

func TestValidateRejectsInvalidSliceOperands(t *testing.T) {
	p := stackVerifierProgram(0, nil, Function{Name: "main", Return: "unit", Code: []Instruction{
		{Op: OpSlice, A: 1},
		{Op: OpHalt},
	}})
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "invalid slice operands") {
		t.Fatalf("expected invalid slice operand error, got %v", err)
	}
}

func TestValidateRejectsNonArraySlice(t *testing.T) {
	p := stackVerifierProgram(0, []Value{I64(3), I64(0), I64(1)}, Function{Name: "main", Locals: 1, Return: "unit", Code: []Instruction{
		{Op: OpConst, A: 0},
		{Op: OpConst, A: 1},
		{Op: OpConst, A: 2},
		{Op: OpSlice},
		{Op: OpStoreLocal, A: 0},
		{Op: OpHalt},
	}})
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "SLICE requires Array<T>") {
		t.Fatalf("expected non-array slice to be rejected, got %v", err)
	}
}

func TestValidateAcceptsTypedNumericCasts(t *testing.T) {
	p := stackVerifierProgram(0, []Value{
		I64(9007199254740993),
		Value{Kind: ValueF64, F64: 4.75},
	}, Function{Name: "main", Locals: 1, Return: "unit", Code: []Instruction{
		{Op: OpConst, A: 0},
		{Op: OpToF64},
		{Op: OpPop},
		{Op: OpConst, A: 1},
		{Op: OpToI64},
		{Op: OpStoreLocal, A: 0},
		{Op: OpHalt},
	}})
	if err := p.Validate(); err != nil {
		t.Fatalf("expected typed numeric casts to validate: %v", err)
	}
}

func TestValidateRejectsNumericCastTypeMismatch(t *testing.T) {
	p := stackVerifierProgram(0, []Value{I64(7)}, Function{Name: "main", Locals: 1, Return: "unit", Code: []Instruction{
		{Op: OpConst, A: 0},
		{Op: OpToI64},
		{Op: OpStoreLocal, A: 0},
		{Op: OpHalt},
	}})
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "TO_I64 requires f64, got i64") {
		t.Fatalf("expected invalid TO_I64 operand type to be rejected, got %v", err)
	}
}

func TestValidateRejectsNumericCastOperands(t *testing.T) {
	p := stackVerifierProgram(0, nil, Function{Name: "main", Return: "unit", Code: []Instruction{
		{Op: OpToF64, A: 1},
		{Op: OpHalt},
	}})
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "invalid numeric cast operands") {
		t.Fatalf("expected numeric cast operand fields to be rejected, got %v", err)
	}
}

func TestValidateTypedStructBytecode(t *testing.T) {
	program := &Program{
		Format:  "aster-bytecode",
		Version: FormatVersion,
		StructTypes: []StructType{{
			Name:   "Point",
			Fields: []StructField{{Name: "x", Type: "i64"}},
		}},
		Constants: []Value{I64(42)},
		Functions: []Function{{
			Name:   "main",
			Locals: 1,
			Return: "unit",
			Code: []Instruction{
				{Op: OpConst, A: 0},
				{Op: OpBuildStruct, A: 0, B: 1},
				{Op: OpStoreLocal, A: 0},
				{Op: OpLoadLocal, A: 0},
				{Op: OpGetField, A: 0},
				{Op: OpBuiltin, A: 0, B: 1},
				{Op: OpPop},
				{Op: OpHalt},
			},
		}},
		Entry: 0,
	}
	if err := program.Validate(); err != nil {
		t.Fatalf("valid struct bytecode was rejected: %v", err)
	}

	program.Functions[0].Code[4].A = 1
	if err := program.Validate(); err == nil || !strings.Contains(err.Error(), "field index 1 is out of bounds") {
		t.Fatalf("invalid field index should be rejected, got %v", err)
	}
}

func TestValidateRejectsInvalidStructMetadata(t *testing.T) {
	tests := []struct {
		name    string
		structs []StructType
		want    string
	}{
		{
			name:    "duplicate struct name",
			structs: []StructType{{Name: "Point"}, {Name: "Point"}},
			want:    "duplicate struct type",
		},
		{
			name: "duplicate field name",
			structs: []StructType{{Name: "Point", Fields: []StructField{
				{Name: "x", Type: "i64"},
				{Name: "x", Type: "i64"},
			}}},
			want: "duplicate field",
		},
		{
			name:    "unknown field type",
			structs: []StructType{{Name: "Point", Fields: []StructField{{Name: "x", Type: "Missing"}}}},
			want:    "has invalid type",
		},
		{
			name:    "unit field type",
			structs: []StructType{{Name: "Point", Fields: []StructField{{Name: "x", Type: "unit"}}}},
			want:    "has invalid type",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program := &Program{
				Format:      "aster-bytecode",
				Version:     FormatVersion,
				StructTypes: tt.structs,
				Functions: []Function{{
					Name:   "main",
					Return: "unit",
					Code:   []Instruction{{Op: OpHalt}},
				}},
				Entry: 0,
			}
			if err := program.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("invalid struct metadata should be rejected with %q, got %v", tt.want, err)
			}
		})
	}
}

func TestValidateRejectsStructConstructorFieldTypeMismatch(t *testing.T) {
	program := &Program{
		Format:      "aster-bytecode",
		Version:     FormatVersion,
		StructTypes: []StructType{{Name: "Point", Fields: []StructField{{Name: "x", Type: "i64"}}}},
		Constants:   []Value{Bool(true)},
		Functions: []Function{{
			Name:   "main",
			Return: "unit",
			Code: []Instruction{
				{Op: OpConst, A: 0},
				{Op: OpBuildStruct, A: 0, B: 1},
				{Op: OpPop},
				{Op: OpHalt},
			},
		}},
		Entry: 0,
	}
	if err := program.Validate(); err == nil || !strings.Contains(err.Error(), "constructor field") {
		t.Fatalf("struct constructor field type mismatch should be rejected, got %v", err)
	}
}

func TestValidateRejectsRecursiveStructLayout(t *testing.T) {
	program := &Program{
		Format:  "aster-bytecode",
		Version: FormatVersion,
		StructTypes: []StructType{
			{Name: "A", Fields: []StructField{{Name: "b", Type: "B"}}},
			{Name: "B", Fields: []StructField{{Name: "a", Type: "A"}}},
		},
		Functions: []Function{{
			Name:   "main",
			Return: "unit",
			Code:   []Instruction{{Op: OpHalt}},
		}},
		Entry: 0,
	}
	if err := program.Validate(); err == nil || !strings.Contains(err.Error(), "recursive by-value layout") {
		t.Fatalf("recursive struct layout should be rejected, got %v", err)
	}
}
