package bytecode

import (
	"fmt"
	"strings"
)

// abstractTypeForValue returns the source-level type represented by a bytecode
// constant. Empty and heterogeneous arrays are rejected because Aster arrays
// must be homogeneous and empty literals have no inferable element type.
func abstractTypeForValue(v Value) (string, error) {
	switch v.Kind {
	case ValueUnit:
		return "unit", nil
	case ValueI64:
		return "i64", nil
	case ValueF64:
		return "f64", nil
	case ValueBool:
		return "bool", nil
	case ValueString:
		return "string", nil
	case ValueArray:
		if len(v.Array) == 0 {
			return "", fmt.Errorf("empty arrays do not have an inferable element type")
		}
		elementType, err := abstractTypeForValue(v.Array[0])
		if err != nil {
			return "", err
		}
		if elementType == "unit" {
			return "", fmt.Errorf("arrays cannot contain unit values")
		}
		for i := 1; i < len(v.Array); i++ {
			got, err := abstractTypeForValue(v.Array[i])
			if err != nil {
				return "", err
			}
			if got != elementType {
				return "", fmt.Errorf("array elements have inconsistent types %s and %s", elementType, got)
			}
		}
		return "Array<" + elementType + ">", nil
	default:
		return "", fmt.Errorf("unknown value kind %q", v.Kind)
	}
}

type abstractLocal struct {
	typ         string
	initialized bool
}

type abstractStackNode struct {
	typ    string
	parent *abstractStackNode
	depth  int
}

type abstractState struct {
	stack  *abstractStackNode
	locals []abstractLocal
}

func abstractStackDepth(stack *abstractStackNode) int {
	if stack == nil {
		return 0
	}
	return stack.depth
}

func pushAbstractType(s *abstractState, typ string) {
	depth := 1
	if s.stack != nil {
		depth = s.stack.depth + 1
	}
	s.stack = &abstractStackNode{typ: typ, parent: s.stack, depth: depth}
}

func cloneAbstractState(s *abstractState) *abstractState {
	return &abstractState{
		stack:  s.stack,
		locals: append([]abstractLocal(nil), s.locals...),
	}
}

// mergeAbstractState merges two incoming abstract states at a control-flow join.
// Stack types must agree exactly. A local is definitely initialized only when
// it is initialized on every incoming path; its inferred type remains a
// constraint even when initialization is not definite.
func mergeAbstractState(dst, src *abstractState, fn string, pc int) (bool, error) {
	dstDepth, srcDepth := abstractStackDepth(dst.stack), abstractStackDepth(src.stack)
	if dstDepth != srcDepth {
		return false, fmt.Errorf("function %q instruction %d: inconsistent operand stack heights %d and %d", fn, pc, dstDepth, srcDepth)
	}
	changed := false
	left, right := dst.stack, src.stack
	for depth := 0; left != nil && right != nil; depth++ {
		if left == right {
			break
		}
		if left.typ != right.typ {
			return false, fmt.Errorf("function %q instruction %d: operand stack type mismatch at depth %d: %s versus %s", fn, pc, dstDepth-depth-1, left.typ, right.typ)
		}
		left, right = left.parent, right.parent
	}
	for i := range dst.locals {
		left, right := dst.locals[i], src.locals[i]
		if left.typ != "" && right.typ != "" && left.typ != right.typ {
			return false, fmt.Errorf("function %q instruction %d: local %d has inconsistent types %s and %s", fn, pc, i, left.typ, right.typ)
		}
		if dst.locals[i].typ == "" && right.typ != "" {
			dst.locals[i].typ = right.typ
			changed = true
		}
		initialized := left.initialized && right.initialized
		if initialized != dst.locals[i].initialized {
			dst.locals[i].initialized = initialized
			changed = true
		}
	}
	return changed, nil
}

func abstractPop(s *abstractState, fn string, pc int, op Op) (string, error) {
	if s.stack == nil {
		return "", fmt.Errorf("function %q instruction %d: abstract stack underflow for %s", fn, pc, op)
	}
	t := s.stack.typ
	s.stack = s.stack.parent
	return t, nil
}

func requireAbstractType(fn string, pc int, op Op, got, want string) error {
	if got != want {
		return fmt.Errorf("function %q instruction %d: %s requires %s, got %s", fn, pc, op, want, got)
	}
	return nil
}

// validateFunctionTypes performs forward dataflow over operand-stack types and
// local initialization. Disconnected code is verified as its own component so
// its abstract local state cannot weaken guarantees on reachable instructions.
func validateFunctionTypes(p *Program, fi int, constantTypes []string, structTypesByName map[string]StructType) error {
	fn := p.Functions[fi]
	states := make([]*abstractState, len(fn.Code))
	components := make([]int, len(fn.Code))
	for i := range components {
		components[i] = -1
	}
	nextComponent := 0

	for root := 0; root < len(fn.Code); root++ {
		if states[root] != nil {
			continue
		}
		component := nextComponent
		nextComponent++
		initial := &abstractState{
			stack:  nil,
			locals: make([]abstractLocal, fn.Locals),
		}
		for i, typ := range fn.ParamTypes {
			initial.locals[i] = abstractLocal{typ: typ, initialized: true}
		}
		states[root] = initial
		components[root] = component
		queue := []int{root}

		for head := 0; head < len(queue); head++ {
			pc := queue[head]
			state := cloneAbstractState(states[pc])
			ins := fn.Code[pc]
			terminal, err := applyAbstractInstruction(p, fi, pc, ins, state, constantTypes, structTypesByName)
			if err != nil {
				return err
			}
			if terminal {
				continue
			}

			enqueue := func(target int) error {
				if target < 0 || target >= len(fn.Code) {
					return fmt.Errorf("function %q instruction %d: control flow falls past the end of the function", fn.Name, pc)
				}
				if states[target] == nil {
					states[target] = cloneAbstractState(state)
					components[target] = component
					queue = append(queue, target)
					return nil
				}
				// Instructions unreachable from the entry point are checked for
				// malformed operations, but do not contribute facts to live code.
				if components[target] != component {
					return nil
				}
				changed, err := mergeAbstractState(states[target], state, fn.Name, target)
				if err != nil {
					return err
				}
				if changed {
					queue = append(queue, target)
				}
				return nil
			}

			switch ins.Op {
			case OpJump:
				if err := enqueue(ins.A); err != nil {
					return err
				}
			case OpJumpIfFalse, OpJumpIfTrue:
				if err := enqueue(ins.A); err != nil {
					return err
				}
				if err := enqueue(pc + 1); err != nil {
					return err
				}
			default:
				if err := enqueue(pc + 1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func applyAbstractInstruction(p *Program, fi, pc int, ins Instruction, s *abstractState, constantTypes []string, structTypesByName map[string]StructType) (bool, error) {
	fn := p.Functions[fi]
	push := func(t string) { pushAbstractType(s, t) }
	pop := func() (string, error) { return abstractPop(s, fn.Name, pc, ins.Op) }

	switch ins.Op {
	case OpConst:
		push(constantTypes[ins.A])
	case OpLoadLocal:
		local := s.locals[ins.A]
		if !local.initialized {
			return false, fmt.Errorf("function %q instruction %d: local %d may be read before initialization", fn.Name, pc, ins.A)
		}
		if local.typ == "" {
			return false, fmt.Errorf("function %q instruction %d: local %d has no inferred type", fn.Name, pc, ins.A)
		}
		push(local.typ)
	case OpStoreLocal:
		got, err := pop()
		if err != nil {
			return false, err
		}
		if got == "unit" {
			return false, fmt.Errorf("function %q instruction %d: locals cannot hold unit values", fn.Name, pc)
		}
		local := &s.locals[ins.A]
		if local.typ != "" && local.typ != got {
			return false, fmt.Errorf("function %q instruction %d: cannot store %s in local %d of type %s", fn.Name, pc, got, ins.A, local.typ)
		}
		local.typ = got
		local.initialized = true
	case OpBuildStruct:
		if ins.A < 0 || ins.A >= len(p.StructTypes) {
			return false, fmt.Errorf("function %q instruction %d: invalid struct metadata index %d", fn.Name, pc, ins.A)
		}
		structType := p.StructTypes[ins.A]
		cursor := s.stack
		for fieldIndex := len(structType.Fields) - 1; fieldIndex >= 0; fieldIndex-- {
			if cursor == nil {
				return false, fmt.Errorf("function %q instruction %d: abstract stack underflow while building struct %q", fn.Name, pc, structType.Name)
			}
			got, want := cursor.typ, structType.Fields[fieldIndex].Type
			if got != want {
				return false, fmt.Errorf("function %q instruction %d: constructor field %q of %s has type %s; expected %s", fn.Name, pc, structType.Fields[fieldIndex].Name, structType.Name, got, want)
			}
			cursor = cursor.parent
		}
		s.stack = cursor
		push(structType.Name)
	case OpGetField:
		receiverType, err := pop()
		if err != nil {
			return false, err
		}
		structType := structTypesByName[receiverType]
		if structType.Name == "" {
			return false, fmt.Errorf("function %q instruction %d: GET_FIELD requires a struct receiver, got %s", fn.Name, pc, receiverType)
		}
		if ins.A < 0 || ins.A >= len(structType.Fields) {
			return false, fmt.Errorf("function %q instruction %d: field index %d is out of bounds for struct %q", fn.Name, pc, ins.A, structType.Name)
		}
		push(structType.Fields[ins.A].Type)
	case OpBuildArray:
		if ins.A == 0 {
			return false, fmt.Errorf("function %q instruction %d: cannot infer the element type of an empty array", fn.Name, pc)
		}
		cursor := s.stack
		elementType := ""
		for i := 0; i < ins.A; i++ {
			if cursor == nil {
				return false, fmt.Errorf("function %q instruction %d: abstract stack underflow while building array", fn.Name, pc)
			}
			if i == 0 {
				elementType = cursor.typ
				if elementType == "unit" {
					return false, fmt.Errorf("function %q instruction %d: arrays cannot contain unit values", fn.Name, pc)
				}
			} else if cursor.typ != elementType {
				return false, fmt.Errorf("function %q instruction %d: array elements have inconsistent types %s and %s", fn.Name, pc, elementType, cursor.typ)
			}
			cursor = cursor.parent
		}
		s.stack = cursor
		arrayType := "Array<" + elementType + ">"
		if !validTypeName(arrayType) {
			return false, fmt.Errorf("function %q instruction %d: array type nesting exceeds limit %d", fn.Name, pc, maxTypeNestingDepth)
		}
		push(arrayType)
	case OpIndex:
		indexType, err := pop()
		if err != nil {
			return false, err
		}
		if err := requireAbstractType(fn.Name, pc, ins.Op, indexType, "i64"); err != nil {
			return false, err
		}
		collectionType, err := pop()
		if err != nil {
			return false, err
		}
		elementType, ok := abstractArrayElementType(collectionType)
		if !ok {
			return false, fmt.Errorf("function %q instruction %d: INDEX requires Array<T>, got %s", fn.Name, pc, collectionType)
		}
		push(elementType)
	case OpSlice:
		endType, err := pop()
		if err != nil {
			return false, err
		}
		if err := requireAbstractType(fn.Name, pc, ins.Op, endType, "i64"); err != nil {
			return false, err
		}
		startType, err := pop()
		if err != nil {
			return false, err
		}
		if err := requireAbstractType(fn.Name, pc, ins.Op, startType, "i64"); err != nil {
			return false, err
		}
		collectionType, err := pop()
		if err != nil {
			return false, err
		}
		if _, ok := abstractArrayElementType(collectionType); !ok {
			return false, fmt.Errorf("function %q instruction %d: SLICE requires Array<T>, got %s", fn.Name, pc, collectionType)
		}
		push(collectionType)
	case OpSetIndexPath:
		valueType, err := pop()
		if err != nil {
			return false, err
		}
		for index := 0; index < ins.A; index++ {
			indexType, err := pop()
			if err != nil {
				return false, err
			}
			if err := requireAbstractType(fn.Name, pc, ins.Op, indexType, "i64"); err != nil {
				return false, err
			}
		}
		collectionType, err := pop()
		if err != nil {
			return false, err
		}
		elementType := collectionType
		for depth := 0; depth < ins.A; depth++ {
			nextType, ok := abstractArrayElementType(elementType)
			if !ok {
				return false, fmt.Errorf("function %q instruction %d: indexed assignment path requires Array<T>, got %s", fn.Name, pc, elementType)
			}
			elementType = nextType
		}
		if valueType != elementType {
			return false, fmt.Errorf("function %q instruction %d: indexed assignment value has type %s; expected %s", fn.Name, pc, valueType, elementType)
		}
		push(collectionType)
	case OpPop:
		if _, err := pop(); err != nil {
			return false, err
		}
	case OpNeg:
		got, err := pop()
		if err != nil {
			return false, err
		}
		if got != "i64" && got != "f64" {
			return false, fmt.Errorf("function %q instruction %d: NEG requires i64 or f64, got %s", fn.Name, pc, got)
		}
		push(got)
	case OpNot:
		got, err := pop()
		if err != nil {
			return false, err
		}
		if err := requireAbstractType(fn.Name, pc, ins.Op, got, "bool"); err != nil {
			return false, err
		}
		push("bool")
	case OpToI64:
		got, err := pop()
		if err != nil {
			return false, err
		}
		if err := requireAbstractType(fn.Name, pc, ins.Op, got, "f64"); err != nil {
			return false, err
		}
		push("i64")
	case OpToF64:
		got, err := pop()
		if err != nil {
			return false, err
		}
		if err := requireAbstractType(fn.Name, pc, ins.Op, got, "i64"); err != nil {
			return false, err
		}
		push("f64")
	case OpAdd, OpSub, OpMul, OpDiv, OpMod, OpEqual, OpNotEqual, OpLess, OpLessEqual, OpGreater, OpGreaterEqual:
		right, err := pop()
		if err != nil {
			return false, err
		}
		left, err := pop()
		if err != nil {
			return false, err
		}
		result, err := abstractBinaryResult(fn.Name, pc, ins.Op, left, right)
		if err != nil {
			return false, err
		}
		push(result)
	case OpJump:
	case OpJumpIfFalse, OpJumpIfTrue:
		got, err := pop()
		if err != nil {
			return false, err
		}
		if err := requireAbstractType(fn.Name, pc, ins.Op, got, "bool"); err != nil {
			return false, err
		}
	case OpCall:
		target := p.Functions[ins.A]
		cursor := s.stack
		for i := len(target.ParamTypes) - 1; i >= 0; i-- {
			if cursor == nil {
				return false, fmt.Errorf("function %q instruction %d: abstract stack underflow while checking call to %q", fn.Name, pc, target.Name)
			}
			got, want := cursor.typ, target.ParamTypes[i]
			if got != want {
				return false, fmt.Errorf("function %q instruction %d: argument %d to %q has type %s; expected %s", fn.Name, pc, i+1, target.Name, got, want)
			}
			cursor = cursor.parent
		}
		s.stack = cursor
		push(target.Return)
	case OpBuiltin:
		if s.stack == nil {
			return false, fmt.Errorf("function %q instruction %d: abstract stack underflow for builtin", fn.Name, pc)
		}
		got := s.stack.typ
		s.stack = s.stack.parent
		switch ins.A {
		case 0:
			if got == "unit" {
				return false, fmt.Errorf("function %q instruction %d: print cannot display unit values", fn.Name, pc)
			}
			if !printableProgramType(p, got) {
				return false, fmt.Errorf("function %q instruction %d: print cannot display values of type %s", fn.Name, pc, got)
			}
			push("unit")
		case 1:
			if got != "string" {
				if _, ok := abstractArrayElementType(got); !ok {
					return false, fmt.Errorf("function %q instruction %d: len requires string or Array<T>, got %s", fn.Name, pc, got)
				}
			}
			push("i64")
		case 2:
			if got != "bool" {
				return false, fmt.Errorf("function %q instruction %d: assert requires bool, got %s", fn.Name, pc, got)
			}
			push("unit")
		default:
			return false, fmt.Errorf("function %q instruction %d: unknown builtin %d", fn.Name, pc, ins.A)
		}
	case OpReturn:
		if abstractStackDepth(s.stack) != 1 {
			return false, fmt.Errorf("function %q instruction %d: RETURN requires exactly one stack value, got %d", fn.Name, pc, abstractStackDepth(s.stack))
		}
		if s.stack.typ != fn.Return {
			return false, fmt.Errorf("function %q instruction %d: RETURN type mismatch: expected %s, got %s", fn.Name, pc, fn.Return, s.stack.typ)
		}
		return true, nil
	case OpHalt:
		if fi != p.Entry {
			return false, fmt.Errorf("function %q instruction %d: HALT outside entry function", fn.Name, pc)
		}
		if s.stack != nil {
			return false, fmt.Errorf("function %q instruction %d: HALT requires an empty operand stack, got %d", fn.Name, pc, abstractStackDepth(s.stack))
		}
		return true, nil
	default:
		return false, fmt.Errorf("function %q instruction %d: cannot type-verify opcode %q", fn.Name, pc, ins.Op)
	}
	return false, nil
}

func printableProgramType(p *Program, typ string) bool {
	switch typ {
	case "i64", "f64", "bool", "string":
		return true
	}
	if strings.HasPrefix(typ, "Array<") && strings.HasSuffix(typ, ">") {
		return printableProgramType(p, typ[len("Array<"):len(typ)-1])
	}
	// Struct printing is deliberately not part of the first struct slice.
	return false
}

func abstractBinaryResult(fn string, pc int, op Op, left, right string) (string, error) {
	typeError := func(requirement string) (string, error) {
		return "", fmt.Errorf("function %q instruction %d: %s requires %s, got %s and %s", fn, pc, op, requirement, left, right)
	}
	if left != right {
		return typeError("matching operand types")
	}
	switch op {
	case OpAdd:
		if left == "i64" || left == "f64" || left == "string" {
			return left, nil
		}
		return typeError("matching i64, f64, or string operands")
	case OpSub, OpMul, OpDiv:
		if left == "i64" || left == "f64" {
			return left, nil
		}
		return typeError("matching i64 or f64 operands")
	case OpMod:
		if left == "i64" {
			return "i64", nil
		}
		return typeError("i64 operands")
	case OpEqual, OpNotEqual:
		if left == "i64" || left == "f64" || left == "bool" || left == "string" {
			return "bool", nil
		}
		return typeError("matching comparable scalar operands")
	case OpLess, OpLessEqual, OpGreater, OpGreaterEqual:
		if left == "i64" || left == "f64" || left == "string" {
			return "bool", nil
		}
		return typeError("matching numeric or string operands")
	default:
		return "", fmt.Errorf("function %q instruction %d: unsupported binary opcode %s", fn, pc, op)
	}
}

func abstractArrayElementType(t string) (string, bool) {
	if !strings.HasPrefix(t, "Array<") || !strings.HasSuffix(t, ">") {
		return "", false
	}
	element := t[len("Array<") : len(t)-1]
	if !validTypeName(element) || element == "unit" {
		return "", false
	}
	return element, true
}
