// Package vm executes validated Aster bytecode with explicit resource limits and runtime checks.
package vm

import (
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/musashimusashino666/aster/internal/bytecode"
	"github.com/musashimusashino666/aster/internal/diagnostic"
	"github.com/musashimusashino666/aster/internal/limits"
	"github.com/musashimusashino666/aster/internal/token"
)

const maxStackDepth = 1 << 20
const maxCallDepth = limits.MaxCallDepth

// maxActiveLocalSlots bounds the sum of local slots across all live call frames.
// It is a deterministic allocation budget, not a complete process memory quota.
const maxActiveLocalSlots = 1 << 18
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
	program          *bytecode.Program
	stack            []bytecode.Value
	frames           []frame
	activeLocalSlots int
	stdoutBytes      int64
	out              io.Writer
}

func Run(program *bytecode.Program, out io.Writer) error {
	if err := program.Validate(); err != nil {
		return diagnostic.WrapBytecode(err, program.Source)
	}
	if out == nil {
		out = io.Discard
	}
	v := &VM{program: program, out: out, stack: make([]bytecode.Value, 0, 128)}
	entry := program.Functions[program.Entry]
	if entry.Locals > maxActiveLocalSlots {
		return v.faultCode(bytecode.Instruction{}, diagnostic.RuntimeLocalSlotLimit, "active local slot limit exceeded (entry requires %d slots, maximum %d)", entry.Locals, maxActiveLocalSlots)
	}
	v.activeLocalSlots = entry.Locals
	v.frames = append(v.frames, frame{function: program.Entry, locals: make([]bytecode.Value, entry.Locals)})
	steps := uint64(0)
	for len(v.frames) > 0 {
		steps++
		if steps > maxInstructionSteps {
			return v.faultCode(bytecode.Instruction{}, diagnostic.RuntimeInstructionBudget, "instruction budget exceeded (%d steps)", maxInstructionSteps)
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
			return v.faultCode(ins, diagnostic.RuntimeOperandStackLimit, "operand stack limit exceeded")
		}
		if len(v.frames) > maxCallDepth {
			return v.faultCode(ins, diagnostic.RuntimeCallDepthLimit, "call depth limit exceeded")
		}
	}
	return nil
}
func setIndexPath(collection bytecode.Value, indices []int64, replacement bytecode.Value) (bytecode.Value, error) {
	if len(indices) == 0 {
		return bytecode.Value{}, fmt.Errorf("indexed assignment requires at least one index")
	}
	if collection.Kind != bytecode.ValueArray {
		return bytecode.Value{}, fmt.Errorf("indexing requires an array, got %s", collection.Kind)
	}
	collection.EnsureArrayStorage()
	index := indices[0]
	if index < 0 || index >= int64(len(collection.Array)) {
		return bytecode.Value{}, diagnostic.NewRuntimeFailure(diagnostic.RuntimeArrayIndexBounds, fmt.Sprintf("array index %d out of bounds for length %d", index, len(collection.Array)))
	}

	// Only detach shared containers. An unshared local can be mutated in place,
	// avoiding a full slice copy for every element assignment in a tight loop.
	collection.PrepareArrayWrite()
	if len(indices) == 1 {
		if replacement.Kind == bytecode.ValueArray {
			replacement.EnsureArrayStorage()
			if replacement.IsArrayBorrowed() {
				replacement.MarkArrayShared()
			}
			replacement.ClearArrayBorrowed()
		}
		collection.Array[int(index)] = replacement
	} else {
		child, err := setIndexPath(collection.Array[int(index)], indices[1:], replacement)
		if err != nil {
			return bytecode.Value{}, err
		}
		collection.Array[int(index)] = child
	}
	// The updated root is being transferred back to the variable from which
	// the indexed-assignment instruction loaded it; this is not a new alias.
	collection.ClearArrayBorrowed()
	return collection, nil
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
		constant := &v.program.Constants[i.A]
		if constant.Kind == bytecode.ValueArray {
			// The constant pool remains a live owner for the lifetime of the
			// program, so any runtime value loaded from it must detach on write.
			constant.EnsureArrayStorage()
			constant.MarkArrayShared()
			value := *constant
			value.SetArrayBorrowed(true)
			push(value)
		} else {
			push(*constant)
		}
	case bytecode.OpBuildArray:
		if i.A < 0 || i.A > len(v.stack) {
			return v.fault(i, "array construction needs %d values, stack has %d", i.A, len(v.stack))
		}
		start := len(v.stack) - i.A
		items := append([]bytecode.Value(nil), v.stack[start:]...)
		for index := range items {
			items[index].EnsureAggregateStorage()
			if items[index].Kind == bytecode.ValueArray {
				if items[index].IsArrayBorrowed() {
					// The source array remains live outside this newly built container.
					items[index].MarkArrayShared()
				}
				items[index].ClearArrayBorrowed()
			} else if items[index].Kind == bytecode.ValueStruct || items[index].Kind == bytecode.ValueEnum {
				// The source aggregate remains live; payload/field arrays must
				// detach if either value later mutates an extracted array.
				items[index].MarkArrayShared()
			}
		}
		v.stack = v.stack[:start]
		push(bytecode.Array(items))
	case bytecode.OpBuildStruct:
		if i.A < 0 || i.A >= len(v.program.StructTypes) {
			return v.fault(i, "invalid struct type index %d", i.A)
		}
		structType := v.program.StructTypes[i.A]
		if i.B != len(structType.Fields) || i.B > len(v.stack) {
			return v.fault(i, "struct %s construction requires %d field values, stack has %d", structType.Name, len(structType.Fields), len(v.stack))
		}
		start := len(v.stack) - i.B
		fields := append([]bytecode.Value(nil), v.stack[start:]...)
		for index := range fields {
			fields[index].EnsureAggregateStorage()
		}
		v.stack = v.stack[:start]
		push(bytecode.Struct(structType.Name, fields))
	case bytecode.OpGetField:
		receiver, err := pop()
		if err != nil {
			return err
		}
		if receiver.Kind != bytecode.ValueStruct {
			return v.fault(i, "field selection requires a struct value, got %s", receiver.Kind)
		}
		if receiver.StructName == "" || i.A < 0 || i.A >= len(receiver.Fields) {
			return v.fault(i, "invalid field index %d for struct %q", i.A, receiver.StructName)
		}
		structType := -1
		for index, candidate := range v.program.StructTypes {
			if candidate.Name == receiver.StructName {
				structType = index
				break
			}
		}
		if structType < 0 || len(v.program.StructTypes[structType].Fields) != len(receiver.Fields) {
			return v.fault(i, "invalid runtime layout for struct %q", receiver.StructName)
		}
		value := receiver.Fields[i.A]
		value.EnsureAggregateStorage()
		if value.Kind == bytecode.ValueArray {
			value.MarkArrayShared()
			value.SetArrayBorrowed(true)
		} else if value.Kind == bytecode.ValueStruct || value.Kind == bytecode.ValueEnum {
			value.MarkArrayShared()
		}
		push(value)
	case bytecode.OpBuildEnum:
		if i.A < 0 || i.A >= len(v.program.EnumTypes) {
			return v.fault(i, "invalid enum type index %d", i.A)
		}
		enumType := v.program.EnumTypes[i.A]
		if i.B < 0 || i.B >= len(enumType.Variants) {
			return v.fault(i, "invalid variant index %d for enum %q", i.B, enumType.Name)
		}
		variant := enumType.Variants[i.B]
		if i.C != len(variant.PayloadTypes) || i.C > len(v.stack) {
			return v.fault(i, "enum %s::%s requires %d payload values, stack has %d", enumType.Name, variant.Name, len(variant.PayloadTypes), len(v.stack))
		}
		start := len(v.stack) - i.C
		payload := append([]bytecode.Value(nil), v.stack[start:]...)
		for index := range payload {
			payload[index].EnsureAggregateStorage()
		}
		v.stack = v.stack[:start]
		push(bytecode.EnumValue(i.A, i.B, payload))
	case bytecode.OpMatchEnum:
		fr := &v.frames[len(v.frames)-1]
		fn := v.program.Functions[fr.function]
		if i.A < 0 || i.A >= len(fr.locals) || i.B < 0 || i.B >= len(fn.MatchTables) {
			return v.fault(i, "invalid enum match local or table index")
		}
		value := fr.locals[i.A]
		table := fn.MatchTables[i.B]
		if value.Kind != bytecode.ValueEnum || value.EnumTypeID != table.EnumTypeID {
			return v.fault(i, "MATCH_ENUM subject does not match enum dispatch table")
		}
		if value.EnumTypeID < 0 || value.EnumTypeID >= len(v.program.EnumTypes) {
			return v.fault(i, "invalid runtime enum type index %d", value.EnumTypeID)
		}
		enumType := v.program.EnumTypes[value.EnumTypeID]
		if value.VariantIndex < 0 || value.VariantIndex >= len(enumType.Variants) || value.VariantIndex >= len(table.Targets) {
			return v.fault(i, "invalid runtime variant index %d for enum %q", value.VariantIndex, enumType.Name)
		}
		fr.ip = table.Targets[value.VariantIndex]
	case bytecode.OpGetEnumPayload:
		fr := &v.frames[len(v.frames)-1]
		if i.A < 0 || i.A >= len(fr.locals) {
			return v.fault(i, "invalid enum payload local %d", i.A)
		}
		value := fr.locals[i.A]
		if value.Kind != bytecode.ValueEnum {
			return v.fault(i, "GET_ENUM_PAYLOAD requires an enum value, got %s", value.Kind)
		}
		if value.VariantIndex != i.B {
			return v.fault(i, "enum payload variant mismatch: have %d, requested %d", value.VariantIndex, i.B)
		}
		if value.EnumTypeID < 0 || value.EnumTypeID >= len(v.program.EnumTypes) {
			return v.fault(i, "invalid runtime enum type index %d", value.EnumTypeID)
		}
		enumType := v.program.EnumTypes[value.EnumTypeID]
		if i.B < 0 || i.B >= len(enumType.Variants) {
			return v.fault(i, "invalid runtime variant index %d for enum %q", i.B, enumType.Name)
		}
		variant := enumType.Variants[i.B]
		if i.C < 0 || i.C >= len(variant.PayloadTypes) || i.C >= len(value.Payload) {
			return v.fault(i, "invalid payload index %d for enum %s::%s", i.C, enumType.Name, variant.Name)
		}
		payload := value.Payload[i.C]
		payload.EnsureAggregateStorage()
		if payload.Kind == bytecode.ValueArray {
			payload.MarkArrayShared()
			payload.SetArrayBorrowed(true)
		} else if payload.Kind == bytecode.ValueStruct || payload.Kind == bytecode.ValueEnum {
			payload.MarkArrayShared()
		}
		push(payload)
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
			return v.faultCode(i, diagnostic.RuntimeArrayIndexBounds, "array index %d out of bounds for length %d", index.I64, len(collection.Array))
		}
		value := collection.Array[int(index.I64)]
		if value.Kind == bytecode.ValueArray {
			value.EnsureArrayStorage()
			// INDEX returns a value borrowed from the parent array.
			value.SetArrayBorrowed(true)
		} else if value.Kind == bytecode.ValueStruct || value.Kind == bytecode.ValueEnum {
			value.MarkArrayShared()
		}
		push(value)
	case bytecode.OpSlice:
		end, err := pop()
		if err != nil {
			return err
		}
		start, err := pop()
		if err != nil {
			return err
		}
		collection, err := pop()
		if err != nil {
			return err
		}
		if collection.Kind != bytecode.ValueArray {
			return v.fault(i, "slicing requires an array, got %s", collection.Kind)
		}
		if start.Kind != bytecode.ValueI64 || end.Kind != bytecode.ValueI64 {
			return v.fault(i, "slice bounds must be i64")
		}
		if start.I64 < 0 || end.I64 < start.I64 || end.I64 > int64(len(collection.Array)) {
			return v.faultCode(i, diagnostic.RuntimeSliceBounds, "slice bounds [%d:%d] out of bounds for length %d", start.I64, end.I64, len(collection.Array))
		}
		items := make([]bytecode.Value, int(end.I64-start.I64))
		copy(items, collection.Array[int(start.I64):int(end.I64)])
		for index := range items {
			items[index].EnsureAggregateStorage()
			items[index].MarkArrayShared()
		}
		push(bytecode.Array(items))
	case bytecode.OpToI64:
		value, err := pop()
		if err != nil {
			return err
		}
		if value.Kind != bytecode.ValueF64 {
			return v.fault(i, "i64 conversion requires f64, got %s", value.Kind)
		}
		limit := math.Ldexp(1, 63)
		if math.IsNaN(value.F64) || math.IsInf(value.F64, 0) || value.F64 < -limit || value.F64 >= limit {
			return v.faultCode(i, diagnostic.RuntimeNumericConversionRange, "f64 value out of range for i64 conversion")
		}
		push(bytecode.Value{Kind: bytecode.ValueI64, I64: int64(value.F64)})
	case bytecode.OpToF64:
		value, err := pop()
		if err != nil {
			return err
		}
		if value.Kind != bytecode.ValueI64 {
			return v.fault(i, "f64 conversion requires i64, got %s", value.Kind)
		}
		push(bytecode.Value{Kind: bytecode.ValueF64, F64: float64(value.I64)})
	case bytecode.OpSetIndexPath:
		replacement, err := pop()
		if err != nil {
			return err
		}
		indices := make([]int64, i.A)
		for index := i.A - 1; index >= 0; index-- {
			value, err := pop()
			if err != nil {
				return err
			}
			if value.Kind != bytecode.ValueI64 {
				return v.fault(i, "array index must be i64, got %s", value.Kind)
			}
			indices[index] = value.I64
		}
		collection, err := pop()
		if err != nil {
			return err
		}
		if replacement.Kind == bytecode.ValueArray {
			replacement.EnsureArrayStorage()
		}
		updated, err := setIndexPath(collection, indices, replacement)
		if err != nil {
			return v.faultFromError(i, err)
		}
		push(updated)
	case bytecode.OpLoadLocal:
		fr := &v.frames[len(v.frames)-1]
		if i.A < 0 || i.A >= len(fr.locals) {
			return v.fault(i, "invalid local slot %d", i.A)
		}
		local := &fr.locals[i.A]
		if local.Kind == bytecode.ValueArray {
			local.EnsureArrayStorage()
			value := *local
			value.SetArrayBorrowed(true)
			push(value)
		} else {
			value := *local
			if value.Kind == bytecode.ValueStruct || value.Kind == bytecode.ValueEnum {
				value.MarkArrayShared()
			}
			push(value)
		}
	case bytecode.OpStoreLocal:
		x, err := pop()
		if err != nil {
			return err
		}
		fr := &v.frames[len(v.frames)-1]
		if i.A < 0 || i.A >= len(fr.locals) {
			return v.fault(i, "invalid local slot %d", i.A)
		}
		if x.Kind == bytecode.ValueArray {
			x.EnsureArrayStorage()
			if x.IsArrayBorrowed() {
				// A borrowed value stored in another slot creates an alias.
				x.MarkArrayShared()
			}
			x.ClearArrayBorrowed()
		} else if x.Kind == bytecode.ValueStruct || x.Kind == bytecode.ValueEnum {
			x.MarkArrayShared()
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
				return v.faultCode(i, diagnostic.RuntimeArithmeticOverflow, "integer overflow in unary '-'")
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
			return v.faultFromError(i, err)
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
		if len(v.frames) >= maxCallDepth {
			return v.faultCode(i, diagnostic.RuntimeCallDepthLimit, "call depth limit exceeded (maximum %d)", maxCallDepth)
		}
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
		if target.Locals > maxActiveLocalSlots-v.activeLocalSlots {
			return v.faultCode(i, diagnostic.RuntimeLocalSlotLimit, "active local slot limit exceeded (requested %d additional slots, %d of %d already active)", target.Locals, v.activeLocalSlots, maxActiveLocalSlots)
		}
		argsStart := len(v.stack) - i.B
		args := append([]bytecode.Value(nil), v.stack[argsStart:]...)
		for index := range args {
			if args[index].Kind == bytecode.ValueArray {
				args[index].EnsureArrayStorage()
				if args[index].IsArrayBorrowed() {
					// The caller may retain an owner after this argument is passed.
					args[index].MarkArrayShared()
				}
				args[index].ClearArrayBorrowed()
			} else if args[index].Kind == bytecode.ValueStruct || args[index].Kind == bytecode.ValueEnum {
				args[index].MarkArrayShared()
			}
		}
		v.stack = v.stack[:argsStart]
		locals := make([]bytecode.Value, target.Locals)
		copy(locals, args)
		v.activeLocalSlots += len(locals)
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
			text, err := display(args[0])
			if err != nil {
				return v.faultFromError(i, err)
			}
			printBytes := int64(len(text)) + 1 // Include the newline emitted by Fprintln.
			if printBytes > limits.MaxStdoutBytes-v.stdoutBytes {
				return v.faultCode(i, diagnostic.RuntimeStdoutLimit, "stdout byte limit exceeded (maximum %d bytes)", limits.MaxStdoutBytes)
			}
			n, writeErr := fmt.Fprintln(v.out, text)
			if writeErr != nil {
				return v.faultCode(i, diagnostic.RuntimeOutputWrite, "write output: %v", writeErr)
			}
			v.stdoutBytes += int64(n)
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
				return v.faultCode(i, diagnostic.RuntimeAssertionFailed, "assertion failed")
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
		if x.Kind == bytecode.ValueArray {
			x.EnsureArrayStorage()
			if x.IsArrayBorrowed() {
				// The source may be a nested element whose parent outlives this
				// frame. Sharing is conservative even when the frame owns the
				// only reference after return.
				x.MarkArrayShared()
			}
			x.ClearArrayBorrowed()
		} else if x.Kind == bytecode.ValueStruct || x.Kind == bytecode.ValueEnum {
			x.MarkArrayShared()
		}
		fr := v.frames[len(v.frames)-1]
		v.activeLocalSlots -= len(fr.locals)
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
func display(v bytecode.Value) (string, error) {
	var out strings.Builder
	if err := appendDisplay(&out, v, 0); err != nil {
		return "", err
	}
	return out.String(), nil
}

// appendDisplay caps rendered output as well as runtime string values. An array
// can repeat the same string value many times, so a per-string limit alone does
// not bound the size of its printable representation.
func appendDisplay(out *strings.Builder, value bytecode.Value, depth int) error {
	write := func(s string) error {
		if len(s) > bytecode.MaxStringBytes-out.Len() {
			return diagnostic.NewRuntimeFailure(diagnostic.RuntimeRenderedOutputLimit, fmt.Sprintf("printed value exceeds %d-byte output limit", bytecode.MaxStringBytes))
		}
		out.WriteString(s)
		return nil
	}

	if value.Kind == bytecode.ValueArray {
		if depth >= 256 && len(value.Array) > 0 {
			return diagnostic.NewRuntimeFailure(diagnostic.RuntimeRenderedOutputLimit, "printed array nesting exceeds depth limit")
		}
		if err := write("["); err != nil {
			return err
		}
		for i, item := range value.Array {
			if i > 0 {
				if err := write(" "); err != nil {
					return err
				}
			}
			if err := appendDisplay(out, item, depth+1); err != nil {
				return err
			}
		}
		return write("]")
	}

	if value.Kind == bytecode.ValueString {
		return write(value.String)
	}
	return write(value.StringValue())
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
			if len(b.String) > bytecode.MaxStringBytes || len(a.String) > bytecode.MaxStringBytes-len(b.String) {
				return bytecode.Value{}, diagnostic.NewRuntimeFailure(diagnostic.RuntimeStringLimit, fmt.Sprintf("string length limit exceeded (maximum %d UTF-8 bytes)", bytecode.MaxStringBytes))
			}
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
				return bytecode.Value{}, diagnostic.NewRuntimeFailure(diagnostic.RuntimeArithmeticOverflow, "i64 overflow in addition")
			}
			return bytecode.I64(x + y), nil
		case bytecode.OpSub:
			if (y < 0 && x > maxInt64+y) || (y > 0 && x < minInt64+y) {
				return bytecode.Value{}, diagnostic.NewRuntimeFailure(diagnostic.RuntimeArithmeticOverflow, "i64 overflow in subtraction")
			}
			return bytecode.I64(x - y), nil
		case bytecode.OpMul:
			if x != 0 && y != 0 {
				if (x == minInt64 && y == -1) || (y == minInt64 && x == -1) {
					return bytecode.Value{}, diagnostic.NewRuntimeFailure(diagnostic.RuntimeArithmeticOverflow, "i64 overflow in multiplication")
				}
				z := x * y
				if z/y != x {
					return bytecode.Value{}, diagnostic.NewRuntimeFailure(diagnostic.RuntimeArithmeticOverflow, "i64 overflow in multiplication")
				}
				return bytecode.I64(z), nil
			}
			return bytecode.I64(0), nil
		case bytecode.OpDiv:
			if y == 0 {
				return bytecode.Value{}, diagnostic.NewRuntimeFailure(diagnostic.RuntimeDivisionByZero, "division by zero")
			}
			if x == minInt64 && y == -1 {
				return bytecode.Value{}, diagnostic.NewRuntimeFailure(diagnostic.RuntimeArithmeticOverflow, "i64 overflow in division")
			}
			return bytecode.I64(x / y), nil
		case bytecode.OpMod:
			if y == 0 {
				return bytecode.Value{}, diagnostic.NewRuntimeFailure(diagnostic.RuntimeRemainderByZero, "remainder by zero")
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
				return bytecode.Value{}, diagnostic.NewRuntimeFailure(diagnostic.RuntimeNonFiniteFloat, "non-finite floating-point result")
			}
			return bytecode.F64(z), nil
		case bytecode.OpSub:
			z := x - y
			if math.IsInf(z, 0) || math.IsNaN(z) {
				return bytecode.Value{}, diagnostic.NewRuntimeFailure(diagnostic.RuntimeNonFiniteFloat, "non-finite floating-point result")
			}
			return bytecode.F64(z), nil
		case bytecode.OpMul:
			z := x * y
			if math.IsInf(z, 0) || math.IsNaN(z) {
				return bytecode.Value{}, diagnostic.NewRuntimeFailure(diagnostic.RuntimeNonFiniteFloat, "non-finite floating-point result")
			}
			return bytecode.F64(z), nil
		case bytecode.OpDiv:
			if y == 0 {
				return bytecode.Value{}, diagnostic.NewRuntimeFailure(diagnostic.RuntimeDivisionByZero, "division by zero")
			}
			z := x / y
			if math.IsInf(z, 0) || math.IsNaN(z) {
				return bytecode.Value{}, diagnostic.NewRuntimeFailure(diagnostic.RuntimeNonFiniteFloat, "non-finite floating-point result")
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
	return v.faultCode(i, diagnostic.RuntimeGeneric, format, args...)
}

// faultCode associates a runtime diagnostic class at the point the VM detects
// a failure. Human-readable text is not used to infer the stable public code.
func (v *VM) faultCode(i bytecode.Instruction, code, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	start := token.Position{Offset: i.Offset, Line: i.Line, Column: i.Column}
	if i.EndLine > 0 && i.EndColumn > 0 {
		rangeStart := token.Position{
			Offset: i.RangeStartOffset, Line: i.RangeStartLine, Column: i.RangeStartColumn,
		}
		end := token.Position{Offset: i.EndOffset, Line: i.EndLine, Column: i.EndColumn}
		return diagnostic.NewAtSourceRange(code, "runtime", v.program.Source, start, rangeStart, end, msg)
	}
	return diagnostic.NewAtSource(code, "runtime", v.program.Source, start, msg)
}

// faultFromError preserves a structured runtime failure's stable code and uses
// RUN000 only for helper failures without an explicit runtime classification.
func (v *VM) faultFromError(i bytecode.Instruction, err error) error {
	if failure, ok := err.(*diagnostic.RuntimeFailure); ok {
		return v.faultCode(i, failure.Code, "%s", failure.Message)
	}
	return v.faultCode(i, diagnostic.RuntimeGeneric, "%v", err)
}
