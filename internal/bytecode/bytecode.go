// Package bytecode defines Aster's versioned virtual-machine instruction format.
package bytecode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/aster-lang/aster/internal/diagnostic"
	"github.com/aster-lang/aster/internal/langtypes"
	"github.com/aster-lang/aster/internal/limits"
)

const FormatVersion = 6

// Per-value allocation limits enforced by bytecode validation and the VM.
// String limits are measured in UTF-8 bytes, not Unicode code points.
const (
	MaxStringBytes              = limits.MaxStringBytes
	MaxArrayLength              = limits.MaxArrayLength
	maxBytecodeFileBytes        = 64 << 20
	maxConstantPoolEntries      = 1 << 18
	maxTotalInstructions        = 1 << 20
	maxFunctionCount            = 100000
	maxJSONNestingDepth         = 1024
	maxArrayNestingDepth        = limits.MaxArrayNestingDepth
	maxTypeNestingDepth         = langtypes.MaxNestingDepth
	maxConstantValueNodes       = 2 << 20
	maxTotalConstantStringBytes = 32 << 20
	maxTotalConstantArrayItems  = 1 << 18
	maxTypedVerifierStateCells  = 1 << 20
	maxTotalParameterTypes      = 1 << 20
	maxStructTypeCount           = 1 << 16
	maxStructFieldCount          = 1 << 16
)

const maxTypeNameBytes = langtypes.MaxTypeNameBytes

type ValueKind string

const (
	ValueUnit   ValueKind = "unit"
	ValueI64    ValueKind = "i64"
	ValueF64    ValueKind = "f64"
	ValueBool   ValueKind = "bool"
	ValueString ValueKind = "string"
	ValueArray  ValueKind = "array"
	ValueStruct ValueKind = "struct"
)

// arrayStorage tracks whether the backing storage is shared by more than one
// independently mutable Aster array value. It is runtime metadata only and is
// deliberately excluded from the versioned bytecode JSON representation.
type arrayStorage struct {
	shared              bool
	childrenInitialized bool
}

type Value struct {
	Kind   ValueKind `json:"kind"`
	I64    int64     `json:"i64,omitempty"`
	F64    float64   `json:"f64,omitempty"`
	Bool   bool      `json:"bool,omitempty"`
	String     string  `json:"string,omitempty"`
	Array      []Value `json:"array,omitempty"`
	StructName string  `json:"struct_name,omitempty"`
	Fields     []Value `json:"fields,omitempty"`

	arrayStorage  *arrayStorage `json:"-"`
	arrayBorrowed bool          `json:"-"`
}

// MarshalJSON preserves the sign bit of floating-point zero. The default
// omitempty handling treats -0.0 as empty and would silently serialize it as
// +0.0 when bytecode is written and read back.
func (v Value) MarshalJSON() ([]byte, error) {
	type valueJSON struct {
		Kind   ValueKind `json:"kind"`
		I64    int64     `json:"i64,omitempty"`
		F64    *float64  `json:"f64,omitempty"`
		Bool   bool      `json:"bool,omitempty"`
		String     string  `json:"string,omitempty"`
		Array      []Value `json:"array,omitempty"`
		StructName string  `json:"struct_name,omitempty"`
		Fields     []Value `json:"fields,omitempty"`
	}
	var f64 *float64
	if v.Kind == ValueF64 {
		f64 = &v.F64
	}
	return json.Marshal(valueJSON{
		Kind:   v.Kind,
		I64:    v.I64,
		F64:    f64,
		Bool:   v.Bool,
		String:     v.String,
		Array:      v.Array,
		StructName: v.StructName,
		Fields:     v.Fields,
	})
}

// EnsureArrayStorage initializes runtime metadata on an array and its nested
// elements. The per-storage initialization flag keeps repeated local reads
// from traversing the entire array on every indexed update.
func (v *Value) EnsureArrayStorage() {
	if v == nil || v.Kind != ValueArray {
		return
	}
	if v.arrayStorage == nil {
		v.arrayStorage = &arrayStorage{}
	}
	if v.arrayStorage.childrenInitialized {
		return
	}
	v.arrayStorage.childrenInitialized = true
	for i := range v.Array {
		v.Array[i].EnsureAggregateStorage()
	}
}

// EnsureAggregateStorage initializes array sharing metadata throughout an
// aggregate, including arrays nested in struct fields.
func (v *Value) EnsureAggregateStorage() {
	if v == nil {
		return
	}
	switch v.Kind {
	case ValueArray:
		v.EnsureArrayStorage()
	case ValueStruct:
		for i := range v.Fields {
			v.Fields[i].EnsureAggregateStorage()
		}
	}
}

// MarkArrayShared conservatively records that another independently mutable
// value may still refer to this array's backing storage.
func (v *Value) MarkArrayShared() {
	v.EnsureAggregateStorage()
	switch v.Kind {
	case ValueArray:
		v.arrayStorage.shared = true
	case ValueStruct:
		for i := range v.Fields {
			v.Fields[i].MarkArrayShared()
		}
	}
}

// PrepareArrayWrite detaches shared storage before a mutation. Child arrays
// become shared because both the old and the new outer array retain them.
func (v *Value) PrepareArrayWrite() {
	v.EnsureArrayStorage()
	if v.Kind != ValueArray || !v.arrayStorage.shared {
		return
	}
	items := append([]Value(nil), v.Array...)
	for i := range items {
		items[i].MarkArrayShared()
	}
	v.Array = items
	v.arrayStorage = &arrayStorage{childrenInitialized: true}
}

func (v Value) IsArrayBorrowed() bool {
	return v.Kind == ValueArray && v.arrayBorrowed
}

// SetArrayBorrowed marks a temporary array value as borrowed from a value that
// may remain live. Storing, returning, or passing it across a copy boundary
// must account for that additional owner.
func (v *Value) SetArrayBorrowed(borrowed bool) {
	if v != nil && v.Kind == ValueArray {
		v.arrayBorrowed = borrowed
	}
}

// ClearArrayBorrowed transfers a temporary array value to its next owner.
func (v *Value) ClearArrayBorrowed() {
	if v != nil {
		v.arrayBorrowed = false
	}
}

func Unit() Value         { return Value{Kind: ValueUnit} }
func I64(n int64) Value   { return Value{Kind: ValueI64, I64: n} }
func F64(n float64) Value { return Value{Kind: ValueF64, F64: n} }
func Bool(b bool) Value   { return Value{Kind: ValueBool, Bool: b} }
func Str(s string) Value  { return Value{Kind: ValueString, String: s} }
func Array(v []Value) Value {
	value := Value{Kind: ValueArray, Array: append([]Value(nil), v...)}
	value.EnsureArrayStorage()
	return value
}

func Struct(name string, fields []Value) Value {
	value := Value{Kind: ValueStruct, StructName: name, Fields: append([]Value(nil), fields...)}
	value.EnsureAggregateStorage()
	for i := range value.Fields {
		value.Fields[i].MarkArrayShared()
		value.Fields[i].ClearArrayBorrowed()
	}
	return value
}
func (v Value) StringValue() string {
	switch v.Kind {
	case ValueUnit:
		return "unit"
	case ValueI64:
		return fmt.Sprintf("%d", v.I64)
	case ValueF64:
		return fmt.Sprintf("%g", v.F64)
	case ValueBool:
		return fmt.Sprintf("%t", v.Bool)
	case ValueString:
		return v.String
	case ValueArray:
		parts := make([]string, len(v.Array))
		for i, item := range v.Array {
			parts[i] = item.StringValue()
		}
		return "[" + strings.Join(parts, " ") + "]"
	case ValueStruct:
		return "<struct " + v.StructName + ">"
	}
	return "<invalid>"
}

type Op string

const (
	OpConst        Op = "CONST"
	OpLoadLocal    Op = "LOAD_LOCAL"
	OpBuildArray   Op = "BUILD_ARRAY"
	OpBuildStruct  Op = "BUILD_STRUCT"
	OpGetField     Op = "GET_FIELD"
	OpIndex        Op = "INDEX"
	OpSlice        Op = "SLICE"
	OpToI64        Op = "TO_I64"
	OpToF64        Op = "TO_F64"
	OpSetIndexPath Op = "SET_INDEX_PATH"
	OpStoreLocal   Op = "STORE_LOCAL"
	OpPop          Op = "POP"
	OpNeg          Op = "NEG"
	OpNot          Op = "NOT"
	OpAdd          Op = "ADD"
	OpSub          Op = "SUB"
	OpMul          Op = "MUL"
	OpDiv          Op = "DIV"
	OpMod          Op = "MOD"
	OpEqual        Op = "EQ"
	OpNotEqual     Op = "NE"
	OpLess         Op = "LT"
	OpLessEqual    Op = "LE"
	OpGreater      Op = "GT"
	OpGreaterEqual Op = "GE"
	OpJump         Op = "JUMP"
	OpJumpIfFalse  Op = "JUMP_IF_FALSE"
	OpJumpIfTrue   Op = "JUMP_IF_TRUE"
	OpCall         Op = "CALL"
	OpBuiltin      Op = "BUILTIN"
	OpReturn       Op = "RETURN"
	OpHalt         Op = "HALT"
)

type Instruction struct {
	Op     Op  `json:"op"`
	A      int `json:"a,omitempty"`
	B      int `json:"b,omitempty"`
	Line   int `json:"line,omitempty"`
	Column int `json:"column,omitempty"`
}
type Function struct {
	Name       string        `json:"name"`
	Params     int           `json:"params"`
	ParamTypes []string      `json:"param_types,omitempty"`
	Locals     int           `json:"locals"`
	Return     string        `json:"return"`
	Code       []Instruction `json:"code"`
}
type StructType struct {
	Name   string        `json:"name"`
	Fields []StructField `json:"fields"`
}

type StructField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Program struct {
	Format      string       `json:"format"`
	Version     int          `json:"version"`
	Source      string       `json:"source,omitempty"`
	StructTypes []StructType `json:"struct_types,omitempty"`
	Constants   []Value      `json:"constants"`
	Functions   []Function   `json:"functions"`
	Entry       int          `json:"entry"`
}


// validateStructTypes validates nominal metadata from untrusted bytecode,
// including duplicate names/fields, field types, and by-value layout cycles.
func (p *Program) validateStructTypes() error {
	if len(p.StructTypes) > maxStructTypeCount {
		return fmt.Errorf("struct type metadata exceeds safety limit")
	}
	types := make(map[string]struct{}, len(p.StructTypes))
	for i, st := range p.StructTypes {
		if st.Name == "" || len(st.Name) > maxTypeNameBytes || !langtypes.ValidName(st.Name) {
			return fmt.Errorf("struct type %d has invalid name %q", i, st.Name)
		}
		switch st.Name {
		case "i64", "f64", "bool", "string", "unit", "Array":
			return fmt.Errorf("struct type %q uses a reserved name", st.Name)
		}
		if _, exists := types[st.Name]; exists {
			return fmt.Errorf("duplicate struct type %q", st.Name)
		}
		types[st.Name] = struct{}{}
		if len(st.Fields) > maxStructFieldCount {
			return fmt.Errorf("struct %q exceeds field-count safety limit", st.Name)
		}
		seen := make(map[string]struct{}, len(st.Fields))
		for _, field := range st.Fields {
			if field.Name == "" || len(field.Name) > maxTypeNameBytes || (!langtypes.ValidName(field.Name) && field.Name != "Array") {
				return fmt.Errorf("struct %q has invalid field name %q", st.Name, field.Name)
			}
			if _, exists := seen[field.Name]; exists {
				return fmt.Errorf("struct %q has duplicate field %q", st.Name, field.Name)
			}
			seen[field.Name] = struct{}{}
		}
	}
	for _, st := range p.StructTypes {
		for _, field := range st.Fields {
			if field.Type == "unit" || !p.validProgramTypeName(field.Type) {
				return fmt.Errorf("struct %q field %q has invalid type %q", st.Name, field.Name, field.Type)
			}
		}
	}
	state := make(map[string]uint8)
	var visit func(string) error
	visit = func(name string) error {
		if state[name] == 1 {
			return fmt.Errorf("struct %q has a recursive by-value layout", name)
		}
		if state[name] == 2 {
			return nil
		}
		state[name] = 1
		st := p.structTypeByName(name)
		for _, field := range st.Fields {
			if _, arr := abstractArrayElementType(field.Type); arr {
				continue
			}
			if _, ok := types[field.Type]; ok {
				if err := visit(field.Type); err != nil {
					return err
				}
			}
		}
		state[name] = 2
		return nil
	}
	for _, st := range p.StructTypes {
		if err := visit(st.Name); err != nil {
			return err
		}
	}
	return nil
}

func (p *Program) structTypeByName(name string) StructType {
	for _, st := range p.StructTypes {
		if st.Name == name {
			return st
		}
	}
	return StructType{}
}

func (p *Program) validProgramTypeName(name string) bool {
	if !langtypes.ValidName(name) {
		return false
	}
	if strings.HasPrefix(name, "Array<") && strings.HasSuffix(name, ">") {
		element := name[len("Array<") : len(name)-1]
		return element != "unit" && p.validProgramTypeName(element)
	}
	switch name {
	case "i64", "f64", "bool", "string", "unit":
		return true
	default:
		for _, st := range p.StructTypes {
			if st.Name == name {
				return true
			}
		}
		return false
	}
}

func (p *Program) Validate() error {
	if p.Format != "aster-bytecode" {
		return fmt.Errorf("not an Aster bytecode file")
	}
	if p.Version != FormatVersion {
		return fmt.Errorf("unsupported bytecode version %d (supported: %d)", p.Version, FormatVersion)
	}
	if p.Entry < 0 || p.Entry >= len(p.Functions) {
		return fmt.Errorf("invalid entry function index %d", p.Entry)
	}
	if len(p.Functions) == 0 || len(p.Functions) > maxFunctionCount {
		return fmt.Errorf("invalid function count %d", len(p.Functions))
	}
	if len(p.Constants) > maxConstantPoolEntries {
		return fmt.Errorf("constant pool exceeds safety limit")
	}
	if err := p.validateStructTypes(); err != nil {
		return err
	}
	totalConstantValues := 0
	totalConstantStringBytes := 0
	totalConstantArrayItems := 0
	constantTypes := make([]string, len(p.Constants))
	for i, c := range p.Constants {
		if err := validateConstantValueLimits(c, i, &totalConstantValues, &totalConstantStringBytes, &totalConstantArrayItems); err != nil {
			return err
		}
		typ, err := abstractTypeForValue(c)
		if err != nil {
			return fmt.Errorf("constant %d has invalid static type: %w", i, err)
		}
		constantTypes[i] = typ
	}
	if p.Functions[p.Entry].Params != 0 {
		return fmt.Errorf("entry function must not require parameters")
	}
	if p.Functions[p.Entry].Return != "unit" {
		return fmt.Errorf("entry function must return unit")
	}
	totalInstructions := 0
	for fi, f := range p.Functions {
		if f.Name == "" {
			return fmt.Errorf("function %d has an empty name", fi)
		}
		if f.Params < 0 || f.Locals < f.Params || f.Locals > (1<<20) {
			return fmt.Errorf("function %q has invalid local count", f.Name)
		}
		if len(f.ParamTypes) != f.Params {
			return fmt.Errorf("function %q has %d parameters but %d parameter types", f.Name, f.Params, len(f.ParamTypes))
		}
		for pi, typ := range f.ParamTypes {
			if !p.validProgramTypeName(typ) || typ == "unit" {
				return fmt.Errorf("function %q parameter %d has invalid type %q", f.Name, pi, typ)
			}
		}
		if !p.validProgramTypeName(f.Return) {
			return fmt.Errorf("function %q has invalid return type %q", f.Name, f.Return)
		}
		if len(f.Code) == 0 {
			return fmt.Errorf("function %q has no instructions", f.Name)
		}
		totalInstructions += len(f.Code)
		if totalInstructions > maxTotalInstructions {
			return fmt.Errorf("program exceeds instruction safety limit")
		}
		for pc, ins := range f.Code {
			switch ins.Op {
			case OpConst:
				if ins.A < 0 || ins.A >= len(p.Constants) {
					return fmt.Errorf("function %q instruction %d references missing constant %d", f.Name, pc, ins.A)
				}
			case OpBuildArray:
				if ins.A < 0 || ins.A > MaxArrayLength {
					return fmt.Errorf("function %q instruction %d has invalid array element count %d", f.Name, pc, ins.A)
				}
			case OpBuildStruct:
				if ins.A < 0 || ins.A >= len(p.StructTypes) {
					return fmt.Errorf("function %q instruction %d references missing struct type %d", f.Name, pc, ins.A)
				}
				if ins.B != len(p.StructTypes[ins.A].Fields) {
					return fmt.Errorf("function %q instruction %d builds struct %q with %d fields; expected %d", f.Name, pc, p.StructTypes[ins.A].Name, ins.B, len(p.StructTypes[ins.A].Fields))
				}
			case OpGetField:
				if ins.A < 0 || ins.B != 0 {
					return fmt.Errorf("function %q instruction %d has invalid field access operands", f.Name, pc)
				}
			case OpIndex:
			case OpSlice:
				if ins.A != 0 || ins.B != 0 {
					return fmt.Errorf("function %q instruction %d has invalid slice operands", f.Name, pc)
				}
			case OpSetIndexPath:
				if ins.A < 1 || ins.A > maxArrayNestingDepth || ins.B != 0 {
					return fmt.Errorf("function %q instruction %d has invalid indexed-assignment depth %d", f.Name, pc, ins.A)
				}
			case OpLoadLocal, OpStoreLocal:
				if ins.A < 0 || ins.A >= f.Locals {
					return fmt.Errorf("function %q instruction %d references invalid local %d", f.Name, pc, ins.A)
				}
			case OpCall:
				if ins.A < 0 || ins.A >= len(p.Functions) {
					return fmt.Errorf("function %q instruction %d references missing function %d", f.Name, pc, ins.A)
				}
				if ins.B != p.Functions[ins.A].Params {
					return fmt.Errorf("function %q instruction %d passes %d arguments to %q, which requires %d", f.Name, pc, ins.B, p.Functions[ins.A].Name, p.Functions[ins.A].Params)
				}
			case OpJump, OpJumpIfFalse, OpJumpIfTrue:
				if ins.A < 0 || ins.A >= len(f.Code) {
					return fmt.Errorf("function %q instruction %d has invalid jump target %d", f.Name, pc, ins.A)
				}
			case OpBuiltin:
				if ins.A < 0 || ins.A > 2 {
					return fmt.Errorf("function %q instruction %d has unknown builtin %d", f.Name, pc, ins.A)
				}
				if ins.B != 1 {
					return fmt.Errorf("function %q instruction %d passes invalid argument count %d to builtin", f.Name, pc, ins.B)
				}
			case OpToI64, OpToF64:
				if ins.A != 0 || ins.B != 0 {
					return fmt.Errorf("function %q instruction %d has invalid numeric cast operands", f.Name, pc)
				}
			case OpPop, OpNeg, OpNot, OpAdd, OpSub, OpMul, OpDiv, OpMod, OpEqual, OpNotEqual, OpLess, OpLessEqual, OpGreater, OpGreaterEqual:
			case OpReturn:
				if fi == p.Entry {
					return fmt.Errorf("RETURN is invalid in the entry function")
				}
			case OpHalt:
				if fi != p.Entry {
					return fmt.Errorf("HALT is only valid in the entry function (function %q)", f.Name)
				}
			default:
				return fmt.Errorf("function %q instruction %d has unknown opcode %q", f.Name, pc, ins.Op)
			}
			if ins.Line < 0 || ins.Column < 0 {
				return fmt.Errorf("function %q instruction %d has invalid source location", f.Name, pc)
			}
		}
	}
	for _, f := range p.Functions {
		if err := validateFunctionStack(f); err != nil {
			return err
		}
	}
	typedStateCells := 0
	for _, f := range p.Functions {
		if f.Locals > 0 && len(f.Code) > (maxTypedVerifierStateCells-typedStateCells)/f.Locals {
			return fmt.Errorf("program exceeds typed verifier state safety limit")
		}
		typedStateCells += len(f.Code) * f.Locals
	}
	for fi := range p.Functions {
		if err := validateFunctionTypes(p, fi, constantTypes); err != nil {
			return err
		}
	}
	return nil
}

const maxVerifiedStackDepth = 1 << 20

// validateFunctionStack performs a control-flow dataflow analysis over operand
// stack heights. Orphaned (unreachable) code regions are checked as independent
// blocks with an empty entry stack so malformed dead code cannot hide underflow.
func validateFunctionStack(fn Function) error {
	depths := make([]int, len(fn.Code))
	for i := range depths {
		depths[i] = -1
	}

	for root := 0; root < len(fn.Code); root++ {
		if depths[root] >= 0 {
			continue
		}
		depths[root] = 0
		queue := []int{root}

		for head := 0; head < len(queue); head++ {
			pc := queue[head]
			ins := fn.Code[pc]
			depth := depths[pc]
			required, delta := 0, 0

			switch ins.Op {
			case OpConst, OpLoadLocal:
				delta = 1
			case OpBuildArray:
				required, delta = ins.A, 1-ins.A
			case OpBuildStruct:
				required, delta = ins.B, 1-ins.B
			case OpGetField:
				required = 1
			case OpIndex:
				required, delta = 2, -1
			case OpSlice:
				required, delta = 3, -2
			case OpSetIndexPath:
				required, delta = ins.A+2, -ins.A-1
			case OpStoreLocal, OpPop:
				required, delta = 1, -1
			case OpNeg, OpNot, OpToI64, OpToF64:
				required = 1
			case OpAdd, OpSub, OpMul, OpDiv, OpMod, OpEqual, OpNotEqual,
				OpLess, OpLessEqual, OpGreater, OpGreaterEqual:
				required, delta = 2, -1
			case OpJump:
				// No stack effect.
			case OpJumpIfFalse, OpJumpIfTrue:
				required, delta = 1, -1
			case OpCall:
				required, delta = ins.B, 1-ins.B
			case OpBuiltin:
				required, delta = ins.B, 1-ins.B
			case OpReturn:
				if depth != 1 {
					return fmt.Errorf("function %q instruction %d: RETURN requires exactly one stack value, got %d", fn.Name, pc, depth)
				}
				continue
			case OpHalt:
				if depth != 0 {
					return fmt.Errorf("function %q instruction %d: HALT requires an empty operand stack, got %d", fn.Name, pc, depth)
				}
				continue
			default:
				// Unknown instructions are rejected by the structural pass in Validate.
				return fmt.Errorf("function %q instruction %d: cannot verify opcode %q", fn.Name, pc, ins.Op)
			}

			if depth < required {
				return fmt.Errorf("function %q instruction %d: operand stack underflow for %s (needs %d values, has %d)", fn.Name, pc, ins.Op, required, depth)
			}
			nextDepth := depth + delta
			if nextDepth < 0 {
				return fmt.Errorf("function %q instruction %d: invalid negative operand stack height %d", fn.Name, pc, nextDepth)
			}
			if nextDepth > maxVerifiedStackDepth {
				return fmt.Errorf("function %q instruction %d: operand stack height %d exceeds limit %d", fn.Name, pc, nextDepth, maxVerifiedStackDepth)
			}

			enqueue := func(target int) error {
				if target < 0 || target >= len(fn.Code) {
					return fmt.Errorf("function %q instruction %d: control flow falls past the end of the function", fn.Name, pc)
				}
				if depths[target] == -1 {
					depths[target] = nextDepth
					queue = append(queue, target)
					return nil
				}
				if depths[target] != nextDepth {
					return fmt.Errorf("function %q instruction %d: control-flow merge at instruction %d has inconsistent operand stack heights %d and %d", fn.Name, pc, target, depths[target], nextDepth)
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

func validateConstantValueLimits(root Value, constantIndex int, totalValues, totalStringBytes, totalArrayItems *int) error {
	type pendingValue struct {
		value Value
		depth int
	}
	stack := []pendingValue{{value: root}}
	for len(stack) > 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]

		*totalValues = *totalValues + 1
		if *totalValues > maxConstantValueNodes {
			return fmt.Errorf("constant pool values exceed safety limit")
		}

		switch current.value.Kind {
		case ValueUnit, ValueI64, ValueBool:
		case ValueF64:
			if math.IsNaN(current.value.F64) || math.IsInf(current.value.F64, 0) {
				return fmt.Errorf("constant %d contains a non-finite f64", constantIndex)
			}
		case ValueString:
			size := len(current.value.String)
			if size > MaxStringBytes {
				return fmt.Errorf("constant %d string exceeds %d-byte limit", constantIndex, MaxStringBytes)
			}
			*totalStringBytes = *totalStringBytes + size
			if *totalStringBytes > maxTotalConstantStringBytes {
				return fmt.Errorf("constant pool strings exceed total byte limit")
			}
		case ValueArray:
			items := len(current.value.Array)
			if items > MaxArrayLength {
				return fmt.Errorf("constant %d array has %d elements; maximum is %d", constantIndex, items, MaxArrayLength)
			}
			if items > 0 && current.depth >= maxArrayNestingDepth {
				return fmt.Errorf("constant %d array nesting exceeds depth limit %d", constantIndex, maxArrayNestingDepth)
			}
			*totalArrayItems = *totalArrayItems + items
			if *totalArrayItems > maxTotalConstantArrayItems {
				return fmt.Errorf("constant pool arrays exceed total element limit")
			}
			for i := len(current.value.Array) - 1; i >= 0; i-- {
				stack = append(stack, pendingValue{value: current.value.Array[i], depth: current.depth + 1})
			}
		default:
			return fmt.Errorf("constant %d has unknown value kind %q", constantIndex, current.value.Kind)
		}
	}
	return nil
}

func validTypeName(t string) bool {
	return langtypes.ValidName(t)
}

func Write(w io.Writer, p *Program) error {
	if err := p.Validate(); err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(p)
}

// bytecodePreflight tracks allocation-sensitive portions of the JSON before
// decoding them into slices of Value, Function, and Instruction structs.
type bytecodePreflight struct {
	constantEntries  int
	constantValues   int
	constantStrings  int
	arrayItems       int
	functionEntries  int
	instructionCount int
	parameterTypes   int
}

const preflightErrorPrefix = "bytecode preflight: "

func preflightBytecodeLimits(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	first, err := dec.Token()
	if err != nil {
		return nil // The regular decoder returns the canonical syntax error.
	}
	delim, ok := first.(json.Delim)
	if !ok || delim != '{' {
		return nil // Let decoding report a wrong top-level type.
	}

	limits := &bytecodePreflight{}
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return nil
		}
		key, ok := token.(string)
		if !ok {
			return nil
		}
		switch key {
		case "constants":
			err = preflightConstants(dec, limits)
		case "functions":
			err = preflightFunctions(dec, limits)
		default:
			err = skipPreflightJSONValue(dec, 0)
		}
		if err != nil {
			if strings.HasPrefix(err.Error(), preflightErrorPrefix) {
				return err
			}
			return nil // Leave malformed JSON diagnostics to json.Decoder.Decode.
		}
	}
	return nil
}

func preflightConstants(dec *json.Decoder, limits *bytecodePreflight) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, isArray := token.(json.Delim)
	if !isArray || delim != '[' {
		return skipPreflightJSONToken(dec, token, 0)
	}
	for dec.More() {
		limits.constantEntries++
		if limits.constantEntries > maxConstantPoolEntries {
			return fmt.Errorf("%sconstant pool exceeds %d-entry safety limit", preflightErrorPrefix, maxConstantPoolEntries)
		}
		if err := preflightConstantValue(dec, 0, limits); err != nil {
			return err
		}
	}
	_, err = dec.Token() // closing ']'
	return err
}

func preflightConstantValue(dec *json.Decoder, depth int, limits *bytecodePreflight) error {
	limits.constantValues++
	if limits.constantValues > maxConstantValueNodes {
		return fmt.Errorf("%sconstant pool values exceed safety limit", preflightErrorPrefix)
	}
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, isObject := token.(json.Delim)
	if !isObject || delim != '{' {
		return skipPreflightJSONToken(dec, token, 0)
	}
	for dec.More() {
		fieldToken, err := dec.Token()
		if err != nil {
			return err
		}
		field, ok := fieldToken.(string)
		if !ok {
			return fmt.Errorf("invalid constant object key")
		}
		switch field {
		case "string":
			value, err := dec.Token()
			if err != nil {
				return err
			}
			if text, ok := value.(string); ok {
				size := len(text)
				if size > MaxStringBytes {
					return fmt.Errorf("%sconstant string exceeds %d-byte limit", preflightErrorPrefix, MaxStringBytes)
				}
				limits.constantStrings += size
				if limits.constantStrings > maxTotalConstantStringBytes {
					return fmt.Errorf("%sconstant pool strings exceed %d-byte total limit", preflightErrorPrefix, maxTotalConstantStringBytes)
				}
			} else if err := skipPreflightJSONToken(dec, value, 0); err != nil {
				return err
			}
		case "array":
			if err := preflightConstantArray(dec, depth, limits); err != nil {
				return err
			}
		default:
			if err := skipPreflightJSONValue(dec, 0); err != nil {
				return err
			}
		}
	}
	_, err = dec.Token() // closing '}'
	return err
}

func preflightConstantArray(dec *json.Decoder, parentDepth int, limits *bytecodePreflight) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, isArray := token.(json.Delim)
	if !isArray || delim != '[' {
		return skipPreflightJSONToken(dec, token, 0)
	}
	items := 0
	for dec.More() {
		items++
		if items > MaxArrayLength {
			return fmt.Errorf("%sconstant array exceeds %d-element limit", preflightErrorPrefix, MaxArrayLength)
		}
		limits.arrayItems++
		if limits.arrayItems > maxTotalConstantArrayItems {
			return fmt.Errorf("%sconstant pool arrays exceed %d-element total limit", preflightErrorPrefix, maxTotalConstantArrayItems)
		}
		if parentDepth >= maxArrayNestingDepth {
			return fmt.Errorf("%sconstant array nesting exceeds depth limit %d", preflightErrorPrefix, maxArrayNestingDepth)
		}
		if err := preflightConstantValue(dec, parentDepth+1, limits); err != nil {
			return err
		}
	}
	_, err = dec.Token() // closing ']'
	return err
}

func preflightFunctions(dec *json.Decoder, limits *bytecodePreflight) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, isArray := token.(json.Delim)
	if !isArray || delim != '[' {
		return skipPreflightJSONToken(dec, token, 0)
	}
	for dec.More() {
		limits.functionEntries++
		if limits.functionEntries > maxFunctionCount {
			return fmt.Errorf("%sfunction list exceeds %d-entry safety limit", preflightErrorPrefix, maxFunctionCount)
		}
		if err := preflightFunction(dec, limits); err != nil {
			return err
		}
	}
	_, err = dec.Token() // closing ']'
	return err
}

func preflightFunction(dec *json.Decoder, limits *bytecodePreflight) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, isObject := token.(json.Delim)
	if !isObject || delim != '{' {
		return skipPreflightJSONToken(dec, token, 0)
	}
	for dec.More() {
		fieldToken, err := dec.Token()
		if err != nil {
			return err
		}
		field, ok := fieldToken.(string)
		if !ok {
			return fmt.Errorf("invalid function object key")
		}
		switch field {
		case "code":
			if err := preflightInstructions(dec, limits); err != nil {
				return err
			}
		case "param_types":
			if err := preflightParameterTypes(dec, limits); err != nil {
				return err
			}
		case "return":
			if err := preflightTypeName(dec); err != nil {
				return err
			}
		default:
			if err := skipPreflightJSONValue(dec, 0); err != nil {
				return err
			}
		}
	}
	_, err = dec.Token() // closing '}'
	return err
}

func preflightParameterTypes(dec *json.Decoder, limits *bytecodePreflight) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, isArray := token.(json.Delim)
	if !isArray || delim != '[' {
		return skipPreflightJSONToken(dec, token, 0)
	}
	for dec.More() {
		limits.parameterTypes++
		if limits.parameterTypes > maxTotalParameterTypes {
			return fmt.Errorf("%sparameter type metadata exceeds %d-entry safety limit", preflightErrorPrefix, maxTotalParameterTypes)
		}
		value, err := dec.Token()
		if err != nil {
			return err
		}
		if typ, ok := value.(string); ok {
			if len(typ) > maxTypeNameBytes {
				return fmt.Errorf("%sparameter type name exceeds %d-byte safety limit", preflightErrorPrefix, maxTypeNameBytes)
			}
		} else if err := skipPreflightJSONToken(dec, value, 0); err != nil {
			return err
		}
	}
	_, err = dec.Token() // closing ']'
	return err
}

func preflightTypeName(dec *json.Decoder) error {
	value, err := dec.Token()
	if err != nil {
		return err
	}
	if typ, ok := value.(string); ok && len(typ) > maxTypeNameBytes {
		return fmt.Errorf("%stype name exceeds %d-byte safety limit", preflightErrorPrefix, maxTypeNameBytes)
	}
	return skipPreflightJSONToken(dec, value, 0)
}

func preflightInstructions(dec *json.Decoder, limits *bytecodePreflight) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, isArray := token.(json.Delim)
	if !isArray || delim != '[' {
		return skipPreflightJSONToken(dec, token, 0)
	}
	for dec.More() {
		limits.instructionCount++
		if limits.instructionCount > maxTotalInstructions {
			return fmt.Errorf("%sprogram exceeds %d-instruction safety limit", preflightErrorPrefix, maxTotalInstructions)
		}
		if err := skipPreflightJSONValue(dec, 0); err != nil {
			return err
		}
	}
	_, err = dec.Token() // closing ']'
	return err
}

func skipPreflightJSONValue(dec *json.Decoder, depth int) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	return skipPreflightJSONToken(dec, token, depth)
}

func skipPreflightJSONToken(dec *json.Decoder, token any, depth int) error {
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if depth >= maxJSONNestingDepth {
		return fmt.Errorf("%sJSON nesting exceeds safety limit %d", preflightErrorPrefix, maxJSONNestingDepth)
	}
	switch delim {
	case '{':
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return err
			}
			if _, ok := key.(string); !ok {
				return fmt.Errorf("invalid JSON object key")
			}
			if err := skipPreflightJSONValue(dec, depth+1); err != nil {
				return err
			}
		}
		_, err := dec.Token()
		return err
	case '[':
		for dec.More() {
			if err := skipPreflightJSONValue(dec, depth+1); err != nil {
				return err
			}
		}
		_, err := dec.Token()
		return err
	default:
		return nil
	}
}

func Read(r io.Reader) (program *Program, resultErr error) {
	defer func() { resultErr = diagnostic.WrapBytecode(resultErr, "") }()
	raw, err := io.ReadAll(io.LimitReader(r, maxBytecodeFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read bytecode: %w", err)
	}
	if len(raw) > maxBytecodeFileBytes {
		return nil, fmt.Errorf("bytecode file exceeds %d-byte safety limit", maxBytecodeFileBytes)
	}
	if err := preflightBytecodeLimits(raw); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var p Program
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("decode bytecode: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode bytecode: trailing JSON value")
		}
		return nil, fmt.Errorf("decode bytecode: trailing data: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}
