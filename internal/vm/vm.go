// Package vm executes validated Aster bytecode with explicit resource limits and runtime checks.
package vm

import (
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/aster-lang/aster/internal/bytecode"
)

const maxStackDepth = 1 << 20
const maxCallDepth = 4096
const maxInstructionSteps = 100_000_000
const maxInt64 = int64(1<<63 - 1)
const minInt64 = -1 << 63

type frame struct {
	function  int
	ip        int
	locals    []bytecode.Value
	stackBase int
}
type VM struct {
	program *bytecode.Program
	stack   []bytecode.Value
	frames  []frame
	out     io.Writer
}

func Run(program *bytecode.Program, out io.Writer) error {
	if err := program.Validate(); err != nil {
		return err
	}
	if out == nil {
		out = io.Discard
	}
	v := &VM{program: program, out: out, stack: make([]bytecode.Value, 0, 128)}
	entry := program.Functions[program.Entry]
	v.frames = append(v.frames, frame{function: program.Entry, locals: make([]bytecode.Value, entry.Locals)})
	steps := uint64(0)
	for len(v.frames) > 0 {
		steps++
		if steps > maxInstructionSteps {
			return v.fault(bytecode.Instruction{}, "instruction budget exceeded (%d steps)", maxInstructionSteps)
		}
		fi := len(v.frames) - 1
		fr := &v.frames[fi]
		fn := v.program.Functions[fr.function]
		if fr.ip < 0 || fr.ip >= len(fn.Code) {
			return v.fault(bytecode.Instruction{}, "instruction pointer escaped function '%s'", fn.Name)
		}
		ins := fn.Code[fr.ip]
		fr.ip++
		if err := v.step(ins, fn.Name); err != nil {
			return err
		}
		if len(v.stack) > maxStackDepth {
			return v.fault(ins, "operand stack limit exceeded")
		}
		if len(v.frames) > maxCallDepth {
			return v.fault(ins, "call depth limit exceeded")
		}
	}
	return nil
}
func (v *VM) step(i bytecode.Instruction, fnName string) error {
	pop := func() (bytecode.Value, error) {
		if len(v.stack) == 0 {
			return bytecode.Value{}, v.fault(i, "operand stack underflow")
		}
		n := len(v.stack) - 1
		x := v.stack[n]
		v.stack = v.stack[:n]
		return x, nil
	}
	push := func(x bytecode.Value) { v.stack = append(v.stack, x) }
	switch i.Op {
	case bytecode.OpConst:
		if i.A < 0 || i.A >= len(v.program.Constants) {
			return v.fault(i, "invalid constant index %d", i.A)
		}
		push(v.program.Constants[i.A])
	case bytecode.OpBuildArray:
		if i.A < 0 || i.A > len(v.stack) {
			return v.fault(i, "array construction needs %d values, stack has %d", i.A, len(v.stack))
		}
		start := len(v.stack) - i.A
		items := append([]bytecode.Value(nil), v.stack[start:]...)
		v.stack = v.stack[:start]
		push(bytecode.Array(items))
	case bytecode.OpIndex:
		index, err := pop()
		if err != nil {
			return err
		}
		collection, err := pop()
		if err != nil {
			return err
		}
		if collection.Kind != bytecode.ValueArray {
			return v.fault(i, "indexing requires an array, got %s", collection.Kind)
		}
		if index.Kind != bytecode.ValueI64 {
			return v.fault(i, "array index must be i64, got %s", index.Kind)
		}
		if index.I64 < 0 || index.I64 >= int64(len(collection.Array)) {
			return v.fault(i, "array index %d out of bounds for length %d", index.I64, len(collection.Array))
		}
		push(collection.Array[int(index.I64)])
	case bytecode.OpLoadLocal:
		fr := &v.frames[len(v.frames)-1]
		if i.A < 0 || i.A >= len(fr.locals) {
			return v.fault(i, "invalid local slot %d", i.A)
		}
		push(fr.locals[i.A])
	case bytecode.OpStoreLocal:
		x, err := pop()
		if err != nil {
			return err
		}
		fr := &v.frames[len(v.frames)-1]
		if i.A < 0 || i.A >= len(fr.locals) {
			return v.fault(i, "invalid local slot %d", i.A)
		}
		fr.locals[i.A] = x
	case bytecode.OpPop:
		_, err := pop()
		return err
	case bytecode.OpNeg:
		x, err := pop()
		if err != nil {
			return err
		}
		switch x.Kind {
		case bytecode.ValueI64:
			if x.I64 == minInt64 {
				return v.fault(i, "integer overflow in unary '-'")
			}
			push(bytecode.I64(-x.I64))
		case bytecode.ValueF64:
			push(bytecode.F64(-x.F64))
		default:
			return v.fault(i, "unary '-' received %s", x.Kind)
		}
	case bytecode.OpNot:
		x, err := pop()
		if err != nil {
			return err
		}
		if x.Kind != bytecode.ValueBool {
			return v.fault(i, "logical not received %s", x.Kind)
		}
		push(bytecode.Bool(!x.Bool))
	case bytecode.OpAdd, bytecode.OpSub, bytecode.OpMul, bytecode.OpDiv, bytecode.OpMod, bytecode.OpEqual, bytecode.OpNotEqual, bytecode.OpLess, bytecode.OpLessEqual, bytecode.OpGreater, bytecode.OpGreaterEqual:
		r, err := pop()
		if err != nil {
			return err
		}
		l, err := pop()
		if err != nil {
			return err
		}
		res, err := applyBinary(i.Op, l, r)
		if err != nil {
			return v.fault(i, "%v", err)
		}
		push(res)
	case bytecode.OpJump:
		fr := &v.frames[len(v.frames)-1]
		if i.A < 0 || i.A > len(v.program.Functions[fr.function].Code) {
			return v.fault(i, "invalid jump target %d", i.A)
		}
		fr.ip = i.A
	case bytecode.OpJumpIfFalse, bytecode.OpJumpIfTrue:
		x, err := pop()
		if err != nil {
			return err
		}
		if x.Kind != bytecode.ValueBool {
			return v.fault(i, "conditional jump requires bool, got %s", x.Kind)
		}
		take := (i.Op == bytecode.OpJumpIfFalse && !x.Bool) || (i.Op == bytecode.OpJumpIfTrue && x.Bool)
		if take {
			fr := &v.frames[len(v.frames)-1]
			if i.A < 0 || i.A > len(v.program.Functions[fr.function].Code) {
				return v.fault(i, "invalid jump target %d", i.A)
			}
			fr.ip = i.A
		}
	case bytecode.OpCall:
		if i.A < 0 || i.A >= len(v.program.Functions) {
			return v.fault(i, "invalid function index %d", i.A)
		}
		target := v.program.Functions[i.A]
		if i.B != target.Params {
			return v.fault(i, "function '%s' expects %d arguments, bytecode passed %d", target.Name, target.Params, i.B)
		}
		if len(v.stack) < i.B {
			return v.fault(i, "not enough arguments on stack")
		}
		argsStart := len(v.stack) - i.B
		args := append([]bytecode.Value(nil), v.stack[argsStart:]...)
		v.stack = v.stack[:argsStart]
		locals := make([]bytecode.Value, target.Locals)
		copy(locals, args)
		v.frames = append(v.frames, frame{function: i.A, locals: locals, stackBase: len(v.stack)})
	case bytecode.OpBuiltin:
		if len(v.stack) < i.B {
			return v.fault(i, "not enough arguments for builtin")
		}
		argsStart := len(v.stack) - i.B
		args := append([]bytecode.Value(nil), v.stack[argsStart:]...)
		v.stack = v.stack[:argsStart]
		switch i.A {
		case 0:
			if len(args) != 1 {
				return v.fault(i, "print expects one argument")
			}
			if _, err := fmt.Fprintln(v.out, display(args[0])); err != nil {
				return v.fault(i, "write output: %v", err)
			}
			push(bytecode.Unit())
		case 1:
			if len(args) != 1 {
				return v.fault(i, "len expects one argument")
			}
			switch args[0].Kind {
			case bytecode.ValueString:
				push(bytecode.I64(int64(utf8.RuneCountInString(args[0].String))))
			case bytecode.ValueArray:
				push(bytecode.I64(int64(len(args[0].Array))))
			default:
				return v.fault(i, "len expects string or array, got %s", args[0].Kind)
			}
		case 2:
			if len(args) != 1 || args[0].Kind != bytecode.ValueBool {
				return v.fault(i, "assert expects one bool argument")
			}
			if !args[0].Bool {
				return v.fault(i, "assertion failed")
			}
			push(bytecode.Unit())
		default:
			return v.fault(i, "unknown builtin id %d", i.A)
		}
	case bytecode.OpReturn:
		x, err := pop()
		if err != nil {
			return err
		}
		fr := v.frames[len(v.frames)-1]
		v.frames = v.frames[:len(v.frames)-1]
		v.stack = v.stack[:fr.stackBase]
		push(x)
	case bytecode.OpHalt:
		if len(v.frames) != 1 {
			return v.fault(i, "HALT used outside entry function")
		}
		v.frames = nil
		v.stack = nil
	default:
		return v.fault(i, "unknown opcode %q", i.Op)
	}
	_ = fnName
	return nil
}
func display(v bytecode.Value) string {
	if v.Kind == bytecode.ValueString {
		return v.String
	}
	return v.StringValue()
}
func applyBinary(op bytecode.Op, a, b bytecode.Value) (bytecode.Value, error) {
	if op == bytecode.OpEqual || op == bytecode.OpNotEqual {
		eq := a.Kind == b.Kind && valueEqual(a, b)
		if op == bytecode.OpNotEqual {
			eq = !eq
		}
		return bytecode.Bool(eq), nil
	}
	if a.Kind != b.Kind {
		return bytecode.Value{}, fmt.Errorf("operand type mismatch: %s and %s", a.Kind, b.Kind)
	}
	if a.Kind == bytecode.ValueString {
		cmp := strings.Compare(a.String, b.String)
		switch op {
		case bytecode.OpAdd:
			return bytecode.Str(a.String + b.String), nil
		case bytecode.OpLess:
			return bytecode.Bool(cmp < 0), nil
		case bytecode.OpLessEqual:
			return bytecode.Bool(cmp <= 0), nil
		case bytecode.OpGreater:
			return bytecode.Bool(cmp > 0), nil
		case bytecode.OpGreaterEqual:
			return bytecode.Bool(cmp >= 0), nil
		default:
			return bytecode.Value{}, fmt.Errorf("operator %s does not support strings", op)
		}
	}
	if a.Kind == bytecode.ValueI64 {
		x, y := a.I64, b.I64
		switch op {
		case bytecode.OpAdd:
			if (y > 0 && x > maxInt64-y) || (y < 0 && x < minInt64-y) {
				return bytecode.Value{}, fmt.Errorf("i64 overflow in addition")
			}
			return bytecode.I64(x + y), nil
		case bytecode.OpSub:
			if (y < 0 && x > maxInt64+y) || (y > 0 && x < minInt64+y) {
				return bytecode.Value{}, fmt.Errorf("i64 overflow in subtraction")
			}
			return bytecode.I64(x - y), nil
		case bytecode.OpMul:
			if x != 0 && y != 0 {
				if (x == minInt64 && y == -1) || (y == minInt64 && x == -1) {
					return bytecode.Value{}, fmt.Errorf("i64 overflow in multiplication")
				}
				z := x * y
				if z/y != x {
					return bytecode.Value{}, fmt.Errorf("i64 overflow in multiplication")
				}
				return bytecode.I64(z), nil
			}
			return bytecode.I64(0), nil
		case bytecode.OpDiv:
			if y == 0 {
				return bytecode.Value{}, fmt.Errorf("division by zero")
			}
			if x == minInt64 && y == -1 {
				return bytecode.Value{}, fmt.Errorf("i64 overflow in division")
			}
			return bytecode.I64(x / y), nil
		case bytecode.OpMod:
			if y == 0 {
				return bytecode.Value{}, fmt.Errorf("remainder by zero")
			}
			if x == minInt64 && y == -1 {
				return bytecode.I64(0), nil
			}
			return bytecode.I64(x % y), nil
		case bytecode.OpLess:
			return bytecode.Bool(x < y), nil
		case bytecode.OpLessEqual:
			return bytecode.Bool(x <= y), nil
		case bytecode.OpGreater:
			return bytecode.Bool(x > y), nil
		case bytecode.OpGreaterEqual:
			return bytecode.Bool(x >= y), nil
		}
	}
	if a.Kind == bytecode.ValueF64 {
		x, y := a.F64, b.F64
		switch op {
		case bytecode.OpAdd:
			z := x + y
			if math.IsInf(z, 0) || math.IsNaN(z) {
				return bytecode.Value{}, fmt.Errorf("non-finite floating-point result")
			}
			return bytecode.F64(z), nil
		case bytecode.OpSub:
			z := x - y
			if math.IsInf(z, 0) || math.IsNaN(z) {
				return bytecode.Value{}, fmt.Errorf("non-finite floating-point result")
			}
			return bytecode.F64(z), nil
		case bytecode.OpMul:
			z := x * y
			if math.IsInf(z, 0) || math.IsNaN(z) {
				return bytecode.Value{}, fmt.Errorf("non-finite floating-point result")
			}
			return bytecode.F64(z), nil
		case bytecode.OpDiv:
			if y == 0 {
				return bytecode.Value{}, fmt.Errorf("division by zero")
			}
			z := x / y
			if math.IsInf(z, 0) || math.IsNaN(z) {
				return bytecode.Value{}, fmt.Errorf("non-finite floating-point result")
			}
			return bytecode.F64(z), nil
		case bytecode.OpLess:
			return bytecode.Bool(x < y), nil
		case bytecode.OpLessEqual:
			return bytecode.Bool(x <= y), nil
		case bytecode.OpGreater:
			return bytecode.Bool(x > y), nil
		case bytecode.OpGreaterEqual:
			return bytecode.Bool(x >= y), nil
		}
	}
	return bytecode.Value{}, fmt.Errorf("operator %s is not defined for %s", op, a.Kind)
}
func valueEqual(a, b bytecode.Value) bool {
	switch a.Kind {
	case bytecode.ValueUnit:
		return true
	case bytecode.ValueI64:
		return a.I64 == b.I64
	case bytecode.ValueF64:
		return a.F64 == b.F64
	case bytecode.ValueBool:
		return a.Bool == b.Bool
	case bytecode.ValueString:
		return a.String == b.String
	}
	return false
}
func (v *VM) fault(i bytecode.Instruction, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	if i.Line > 0 {
		return fmt.Errorf("%d:%d: runtime error: %s", i.Line, i.Column, msg)
	}
	return fmt.Errorf("runtime error: %s", msg)
}
