package bytecode

import "testing"

func FuzzValidateStructuredProgramsNeverPanics(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11})
	f.Add([]byte{255, 128, 64, 32, 16, 8, 4, 2, 1})
	f.Add([]byte("aster-bytecode verifier seed"))

	f.Fuzz(func(t *testing.T, input []byte) {
		at := func(index int) byte {
			if len(input) == 0 {
				return 0
			}
			return input[index%len(input)]
		}

		ops := []Op{
			OpConst, OpLoadLocal, OpBuildArray, OpIndex, OpSlice, OpSetIndexPath,
			OpToI64, OpToF64, OpStoreLocal, OpPop, OpNeg, OpNot, OpAdd, OpSub,
			OpMul, OpDiv, OpMod, OpEqual, OpNotEqual, OpLess, OpLessEqual,
			OpGreater, OpGreaterEqual, OpJump, OpJumpIfFalse, OpJumpIfTrue,
			OpCall, OpBuiltin, OpReturn, OpHalt, "UNKNOWN",
		}
		typeNames := []string{"i64", "f64", "bool", "string", "unit", "Array<i64>", "Array<unit>", "invalid"}

		constantCount := int(at(0) % 6)
		constants := make([]Value, constantCount)
		for i := range constants {
			switch at(10 + i)%5 {
			case 0:
				constants[i] = I64(int64(int8(at(20 + i))))
			case 1:
				constants[i] = F64(float64(int8(at(30 + i))))
			case 2:
				constants[i] = Bool(at(40+i)%2 == 0)
			case 3:
				constants[i] = Str(string([]byte{at(50 + i), at(60 + i)}))
			default:
				constants[i] = Array([]Value{I64(int64(int8(at(70 + i))))})
			}
		}

		functionCount := 1 + int(at(1)%3)
		functions := make([]Function, functionCount)
		for fi := range functions {
			params := int(at(80+fi) % 4)
			paramTypes := make([]string, params)
			for pi := range paramTypes {
				paramTypes[pi] = typeNames[int(at(90+fi*4+pi))%len(typeNames)]
			}
			locals := params + int(at(110+fi)%5)
			instructionCount := 1 + int(at(120+fi)%12)
			code := make([]Instruction, instructionCount)
			for pc := range code {
				code[pc] = Instruction{
					Op:     ops[int(at(140+fi*12+pc))%len(ops)],
					A:      int(int8(at(180 + fi*12 + pc))),
					B:      int(int8(at(220 + fi*12 + pc))),
					Line:   int(at(260 + fi*12 + pc) % 4),
					Column: int(at(300 + fi*12 + pc) % 8),
				}
			}
			if at(340+fi)%16 == 0 {
				code[0].Line = -1
			}
			returnType := typeNames[int(at(350+fi))%len(typeNames)]
			name := "function"
			if at(360+fi)%4 == 0 {
				name = ""
			}
			functions[fi] = Function{
				Name:       name,
				Params:     params,
				ParamTypes: paramTypes,
				Locals:     locals,
				Return:     returnType,
				Code:       code,
			}
		}

		format := "aster-bytecode"
		if at(2)%4 == 0 {
			format = "not-bytecode"
		}
		version := FormatVersion
		if at(3)%4 == 0 {
			version = int(at(4))
		}
		program := &Program{
			Format:    format,
			Version:   version,
			Constants: constants,
			Functions: functions,
			Entry:     int(at(5)) % functionCount,
		}
		_ = program.Validate()
	})
}
