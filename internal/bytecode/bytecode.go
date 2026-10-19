// Package bytecode defines Aster's versioned virtual-machine instruction format.
package bytecode

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
)

const FormatVersion = 1

type ValueKind string

const (
	ValueUnit   ValueKind = "unit"
	ValueI64    ValueKind = "i64"
	ValueF64    ValueKind = "f64"
	ValueBool   ValueKind = "bool"
	ValueString ValueKind = "string"
	ValueArray  ValueKind = "array"
)

type Value struct {
	Kind   ValueKind `json:"kind"`
	I64    int64     `json:"i64,omitempty"`
	F64    float64   `json:"f64,omitempty"`
	Bool   bool      `json:"bool,omitempty"`
	String string    `json:"string,omitempty"`
	Array  []Value   `json:"array,omitempty"`
}

func Unit() Value           { return Value{Kind: ValueUnit} }
func I64(n int64) Value     { return Value{Kind: ValueI64, I64: n} }
func F64(n float64) Value   { return Value{Kind: ValueF64, F64: n} }
func Bool(b bool) Value     { return Value{Kind: ValueBool, Bool: b} }
func Str(s string) Value    { return Value{Kind: ValueString, String: s} }
func Array(v []Value) Value { return Value{Kind: ValueArray, Array: append([]Value(nil), v...)} }
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
	}
	return "<invalid>"
}

type Op string

const (
	OpConst        Op = "CONST"
	OpLoadLocal    Op = "LOAD_LOCAL"
	OpBuildArray   Op = "BUILD_ARRAY"
	OpIndex        Op = "INDEX"
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
	Name   string        `json:"name"`
	Params int           `json:"params"`
	Locals int           `json:"locals"`
	Return string        `json:"return"`
	Code   []Instruction `json:"code"`
}
type Program struct {
	Format    string     `json:"format"`
	Version   int        `json:"version"`
	Source    string     `json:"source,omitempty"`
	Constants []Value    `json:"constants"`
	Functions []Function `json:"functions"`
	Entry     int        `json:"entry"`
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
	if len(p.Functions) == 0 || len(p.Functions) > 100000 {
		return fmt.Errorf("invalid function count %d", len(p.Functions))
	}
	if len(p.Constants) > 1000000 {
		return fmt.Errorf("constant pool exceeds safety limit")
	}
	for i, c := range p.Constants {
		switch c.Kind {
		case ValueUnit, ValueI64, ValueF64, ValueBool, ValueString, ValueArray:
		default:
			return fmt.Errorf("constant %d has unknown value kind %q", i, c.Kind)
		}
		if c.Kind == ValueF64 && (math.IsNaN(c.F64) || math.IsInf(c.F64, 0)) {
			return fmt.Errorf("constant %d is a non-finite f64", i)
		}
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
		if !validTypeName(f.Return) {
			return fmt.Errorf("function %q has invalid return type %q", f.Name, f.Return)
		}
		if len(f.Code) == 0 {
			return fmt.Errorf("function %q has no instructions", f.Name)
		}
		totalInstructions += len(f.Code)
		if totalInstructions > 10000000 {
			return fmt.Errorf("program exceeds instruction safety limit")
		}
		for pc, ins := range f.Code {
			switch ins.Op {
			case OpConst:
				if ins.A < 0 || ins.A >= len(p.Constants) {
					return fmt.Errorf("function %q instruction %d references missing constant %d", f.Name, pc, ins.A)
				}
			case OpBuildArray:
				if ins.A < 0 || ins.A > (1<<20) {
					return fmt.Errorf("function %q instruction %d has invalid array element count %d", f.Name, pc, ins.A)
				}
			case OpIndex:
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
	return nil
}
func validTypeName(t string) bool {
	switch t {
	case "i64", "f64", "bool", "string", "unit":
		return true
	}
	if strings.HasPrefix(t, "Array<") && strings.HasSuffix(t, ">") {
		elem := t[len("Array<") : len(t)-1]
		return elem != "unit" && validTypeName(elem)
	}
	return false
}

func Write(w io.Writer, p *Program) error {
	if err := p.Validate(); err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(p)
}
func Read(r io.Reader) (*Program, error) {
	dec := json.NewDecoder(r)
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
