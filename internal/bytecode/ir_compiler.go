package bytecode

import (
	"fmt"

	"github.com/aster-lang/aster/internal/ir"
)

func compileIR(tree *ir.Program) (*Program, error) {
	if tree == nil {
		return nil, fmt.Errorf("bytecode: nil normalized IR")
	}
	p := &Program{
		Format:      "aster-bytecode",
		Version:     FormatVersion,
		Source:      tree.SourceName,
		Constants:   []Value{},
		StructTypes: make([]StructType, len(tree.StructTypes)),
		EnumTypes:   make([]EnumType, len(tree.EnumTypes)),
		Entry:       len(tree.Functions),
	}
	for i, structType := range tree.StructTypes {
		if structType.ID != i {
			return nil, fmt.Errorf("bytecode: non-canonical IR struct id %d at index %d", structType.ID, i)
		}
		fields := make([]StructField, len(structType.Fields))
		for j, field := range structType.Fields {
			fields[j] = StructField{Name: field.Name, Type: string(field.Type)}
		}
		p.StructTypes[i] = StructType{Name: structType.Name, Fields: fields}
	}
	for i, enumType := range tree.EnumTypes {
		if enumType.ID != i {
			return nil, fmt.Errorf("bytecode: non-canonical IR enum id %d at index %d", enumType.ID, i)
		}
		variants := make([]EnumVariant, len(enumType.Variants))
		for j, variant := range enumType.Variants {
			payloadTypes := make([]string, len(variant.Payload))
			for k, typ := range variant.Payload {
				payloadTypes[k] = string(typ)
			}
			variants[j] = EnumVariant{Name: variant.Name, PayloadTypes: payloadTypes}
		}
		p.EnumTypes[i] = EnumType{Name: enumType.Name, Variants: variants}
	}
	for i, fn := range tree.Functions {
		if fn == nil {
			return nil, fmt.Errorf("bytecode: nil IR function at index %d", i)
		}
		if int(fn.ID) != i {
			return nil, fmt.Errorf("%d:%d: bytecode: non-canonical IR function id %d at index %d", fn.Pos().Line, fn.Pos().Column, fn.ID, i)
		}
		paramTypes := make([]string, len(fn.Params))
		for j, param := range fn.Params {
			paramTypes[j] = string(param.Type)
		}
		p.Functions = append(p.Functions, Function{
			Name:       fn.Name,
			Params:     len(fn.Params),
			ParamTypes: paramTypes,
			Return:     string(fn.ReturnType),
		})
	}
	p.Functions = append(p.Functions, Function{Name: "__aster_main", Return: "unit"})

	for i, fn := range tree.Functions {
		c := newIRFunctionCompiler(p, i, fn.Name)
		for _, param := range fn.Params {
			if _, err := c.allocate(param.ID, param.Name); err != nil {
				return nil, err
			}
		}
		if fn.Body != nil {
			if err := c.compileBlock(fn.Body); err != nil {
				return nil, err
			}
		}
		if fn.ReturnType == "unit" {
			c.emit(OpConst, c.addConstant(Unit()), 0, fn.Pos().Line, fn.Pos().Column)
			c.emit(OpReturn, 0, 0, fn.Pos().Line, fn.Pos().Column)
		}
		p.Functions[i].Locals = c.nextSlot
		p.Functions[i].Code = c.code
	}

	main := newIRFunctionCompiler(p, p.Entry, "__aster_main")
	for _, stmt := range tree.Body {
		if err := main.compileStmt(stmt); err != nil {
			return nil, err
		}
	}
	main.emit(OpHalt, 0, 0, 1, 1)
	p.Functions[p.Entry].Locals = main.nextSlot
	p.Functions[p.Entry].Code = main.code

	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

type irFunctionCompiler struct {
	program  *Program
	fnID     int
	name     string
	code     []Instruction
	symbols  map[ir.SymbolID]int
	nextSlot int
}

func newIRFunctionCompiler(p *Program, id int, name string) *irFunctionCompiler {
	return &irFunctionCompiler{
		program: p,
		fnID:    id,
		name:    name,
		symbols: make(map[ir.SymbolID]int),
	}
}

func (c *irFunctionCompiler) emit(op Op, a, b, line, column int) int {
	c.code = append(c.code, Instruction{Op: op, A: a, B: b, Line: line, Column: column})
	return len(c.code) - 1
}

func (c *irFunctionCompiler) emitABC(op Op, a, b, operandC, line, column int) int {
	c.code = append(c.code, Instruction{Op: op, A: a, B: b, C: operandC, Line: line, Column: column})
	return len(c.code) - 1
}

func (c *irFunctionCompiler) patch(at, target int) {
	c.code[at].A = target
}

func (c *irFunctionCompiler) addConstant(value Value) int {
	c.program.Constants = append(c.program.Constants, value)
	return len(c.program.Constants) - 1
}

func (c *irFunctionCompiler) allocate(id ir.SymbolID, name string) (int, error) {
	if _, exists := c.symbols[id]; exists {
		return 0, fmt.Errorf("compiler: duplicate IR local identity %d for %q in function %q", id, name, c.name)
	}
	slot := c.nextSlot
	c.nextSlot++
	c.symbols[id] = slot
	return slot, nil
}

func (c *irFunctionCompiler) slot(local ir.SymbolID, name string, pos ir.Node) (int, error) {
	slot, exists := c.symbols[local]
	if !exists {
		p := pos.Pos()
		return 0, fmt.Errorf("%d:%d: compiler: unresolved IR local %q (symbol %d)", p.Line, p.Column, name, local)
	}
	return slot, nil
}

func (c *irFunctionCompiler) compileBlock(block *ir.BlockStmt) error {
	for _, stmt := range block.Statements {
		if err := c.compileStmt(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (c *irFunctionCompiler) compileStmt(stmt ir.Stmt) error {
	pos := stmt.Pos()
	switch n := stmt.(type) {
	case *ir.BlockStmt:
		return c.compileBlock(n)
	case *ir.LetStmt:
		if err := c.compileExpr(n.Value); err != nil {
			return err
		}
		slot, err := c.allocate(n.Local.ID, n.Local.Name)
		if err != nil {
			return fmt.Errorf("%d:%d: %w", pos.Line, pos.Column, err)
		}
		c.emit(OpStoreLocal, slot, 0, pos.Line, pos.Column)
		return nil
	case *ir.IndexAssignStmt:
		slot, err := c.slot(n.Target, n.Name, n)
		if err != nil {
			return err
		}
		c.emit(OpLoadLocal, slot, 0, pos.Line, pos.Column)
		for _, index := range n.Indices {
			if err := c.compileExpr(index); err != nil {
				return err
			}
		}
		if err := c.compileExpr(n.Value); err != nil {
			return err
		}
		c.emit(OpSetIndexPath, len(n.Indices), 0, pos.Line, pos.Column)
		c.emit(OpStoreLocal, slot, 0, pos.Line, pos.Column)
		return nil
	case *ir.AssignStmt:
		slot, err := c.slot(n.Target, n.Name, n)
		if err != nil {
			return err
		}
		if err := c.compileExpr(n.Value); err != nil {
			return err
		}
		c.emit(OpStoreLocal, slot, 0, pos.Line, pos.Column)
		return nil
	case *ir.ExprStmt:
		if err := c.compileExpr(n.Expr); err != nil {
			return err
		}
		c.emit(OpPop, 0, 0, pos.Line, pos.Column)
		return nil
	case *ir.IfStmt:
		if condition, constant := ir.BoolLiteral(n.Condition); constant {
			if condition {
				return c.compileBlock(n.Then)
			}
			if n.Else != nil {
				return c.compileStmt(n.Else)
			}
			return nil
		}
		if err := c.compileExpr(n.Condition); err != nil {
			return err
		}
		jumpFalse := c.emit(OpJumpIfFalse, -1, 0, pos.Line, pos.Column)
		if err := c.compileBlock(n.Then); err != nil {
			return err
		}
		if n.Else == nil {
			c.patch(jumpFalse, len(c.code))
			return nil
		}
		jumpEnd := -1
		if !ir.CannotFallThrough(n.Then) {
			jumpEnd = c.emit(OpJump, -1, 0, pos.Line, pos.Column)
		}
		c.patch(jumpFalse, len(c.code))
		if err := c.compileStmt(n.Else); err != nil {
			return err
		}
		if jumpEnd >= 0 {
			c.patch(jumpEnd, len(c.code))
		}
		return nil
	case *ir.WhileStmt:
		if condition, constant := ir.BoolLiteral(n.Condition); constant {
			if !condition {
				return nil
			}
			start := len(c.code)
			if err := c.compileBlock(n.Body); err != nil {
				return err
			}
			c.emit(OpJump, start, 0, pos.Line, pos.Column)
			return nil
		}
		start := len(c.code)
		if err := c.compileExpr(n.Condition); err != nil {
			return err
		}
		jumpExit := c.emit(OpJumpIfFalse, -1, 0, pos.Line, pos.Column)
		if err := c.compileBlock(n.Body); err != nil {
			return err
		}
		c.emit(OpJump, start, 0, pos.Line, pos.Column)
		c.patch(jumpExit, len(c.code))
		return nil
	case *ir.ReturnStmt:
		if n.Value == nil {
			c.emit(OpConst, c.addConstant(Unit()), 0, pos.Line, pos.Column)
		} else if err := c.compileExpr(n.Value); err != nil {
			return err
		}
		c.emit(OpReturn, 0, 0, pos.Line, pos.Column)
		return nil
	default:
		return fmt.Errorf("%d:%d: compiler: unsupported IR statement %T", pos.Line, pos.Column, stmt)
	}
}

func (c *irFunctionCompiler) compileExpr(expr ir.Expr) error {
	pos := expr.Pos()
	switch n := expr.(type) {
	case *ir.LiteralExpr:
		var value Value
		switch n.Kind {
		case "i64":
			value = I64(n.Int)
		case "f64":
			value = F64(n.Float)
		case "bool":
			value = Bool(n.Bool)
		case "string":
			value = Str(n.String)
		default:
			return fmt.Errorf("%d:%d: compiler: unknown IR literal type %s", pos.Line, pos.Column, n.Kind)
		}
		c.emit(OpConst, c.addConstant(value), 0, pos.Line, pos.Column)
		return nil
	case *ir.ArrayExpr:
		for _, item := range n.Elements {
			if err := c.compileExpr(item); err != nil {
				return err
			}
		}
		c.emit(OpBuildArray, len(n.Elements), 0, pos.Line, pos.Column)
		return nil
	case *ir.EnumExpr:
		if n.EnumID < 0 || n.EnumID >= len(c.program.EnumTypes) {
			return fmt.Errorf("%d:%d: compiler: invalid enum id %d", pos.Line, pos.Column, n.EnumID)
		}
		enumType := c.program.EnumTypes[n.EnumID]
		if n.VariantIndex < 0 || n.VariantIndex >= len(enumType.Variants) {
			return fmt.Errorf("%d:%d: compiler: invalid variant index %d for enum %q", pos.Line, pos.Column, n.VariantIndex, enumType.Name)
		}
		if len(n.Args) != len(enumType.Variants[n.VariantIndex].PayloadTypes) {
			return fmt.Errorf("%d:%d: compiler: enum %q variant %q has wrong payload arity", pos.Line, pos.Column, enumType.Name, enumType.Variants[n.VariantIndex].Name)
		}
		for _, arg := range n.Args {
			if err := c.compileExpr(arg); err != nil {
				return err
			}
		}
		c.emitABC(OpBuildEnum, n.EnumID, n.VariantIndex, len(n.Args), pos.Line, pos.Column)
		return nil
	case *ir.MatchExpr:
		if n.EnumID < 0 || n.EnumID >= len(c.program.EnumTypes) {
			return fmt.Errorf("%d:%d: compiler: invalid match enum id %d", pos.Line, pos.Column, n.EnumID)
		}
		if err := c.compileExpr(n.Subject); err != nil {
			return err
		}
		subjectSlot, err := c.allocate(n.SubjectLocal.ID, n.SubjectLocal.Name)
		if err != nil {
			return fmt.Errorf("%d:%d: %w", pos.Line, pos.Column, err)
		}
		c.emit(OpStoreLocal, subjectSlot, 0, pos.Line, pos.Column)
		tableIndex := len(c.program.Functions[c.fnID].MatchTables)
		targets := make([]int, len(c.program.EnumTypes[n.EnumID].Variants))
		for i := range targets {
			targets[i] = -1
		}
		c.program.Functions[c.fnID].MatchTables = append(c.program.Functions[c.fnID].MatchTables, MatchTable{EnumTypeID: n.EnumID, Targets: targets})
		c.emit(OpMatchEnum, subjectSlot, tableIndex, pos.Line, pos.Column)
		var endJumps []int
		for _, arm := range n.Arms {
			armStart := len(c.code)
			for _, variantIndex := range arm.VariantIndexes {
				if variantIndex < 0 || variantIndex >= len(targets) {
					return fmt.Errorf("%d:%d: compiler: invalid variant index %d in match arm", arm.At.Line, arm.At.Column, variantIndex)
				}
				if targets[variantIndex] != -1 {
					return fmt.Errorf("%d:%d: compiler: duplicate variant target %d in match", arm.At.Line, arm.At.Column, variantIndex)
				}
				targets[variantIndex] = armStart
			}
			variantIndex := -1
			if len(arm.VariantIndexes) == 1 {
				variantIndex = arm.VariantIndexes[0]
			}
			for payloadIndex, binding := range arm.Bindings {
				if variantIndex < 0 {
					return fmt.Errorf("%d:%d: compiler: wildcard arm cannot bind enum payloads", arm.At.Line, arm.At.Column)
				}
				c.emitABC(OpGetEnumPayload, subjectSlot, variantIndex, payloadIndex, binding.At.Line, binding.At.Column)
				bindingSlot, bindErr := c.allocate(binding.ID, binding.Name)
				if bindErr != nil {
					return fmt.Errorf("%d:%d: %w", binding.At.Line, binding.At.Column, bindErr)
				}
				c.emit(OpStoreLocal, bindingSlot, 0, binding.At.Line, binding.At.Column)
			}
			if err := c.compileExpr(arm.Expr); err != nil {
				return err
			}
			endJumps = append(endJumps, c.emit(OpJump, -1, 0, arm.At.Line, arm.At.Column))
		}
		for i, target := range targets {
			if target < 0 {
				return fmt.Errorf("%d:%d: compiler: enum match leaves variant %d uncovered", pos.Line, pos.Column, i)
			}
			c.program.Functions[c.fnID].MatchTables[tableIndex].Targets[i] = target
		}
		end := len(c.code)
		for _, jump := range endJumps {
			c.patch(jump, end)
		}
		return nil
	case *ir.FieldExpr:
		if err := c.compileExpr(n.Object); err != nil {
			return err
		}
		c.emit(OpGetField, n.FieldIndex, 0, pos.Line, pos.Column)
		return nil
	case *ir.IndexExpr:
		if err := c.compileExpr(n.Collection); err != nil {
			return err
		}
		if err := c.compileExpr(n.Index); err != nil {
			return err
		}
		if n.Op != ir.OpIndexChecked {
			return fmt.Errorf("%d:%d: compiler: unsupported IR index operation %q", pos.Line, pos.Column, n.Op)
		}
		c.emit(OpIndex, 0, 0, pos.Line, pos.Column)
		return nil
	case *ir.SliceExpr:
		if err := c.compileExpr(n.Collection); err != nil {
			return err
		}
		if err := c.compileExpr(n.Start); err != nil {
			return err
		}
		if err := c.compileExpr(n.End); err != nil {
			return err
		}
		if n.Op != ir.OpSliceChecked {
			return fmt.Errorf("%d:%d: compiler: unsupported IR slice operation %q", pos.Line, pos.Column, n.Op)
		}
		c.emit(OpSlice, 0, 0, pos.Line, pos.Column)
		return nil
	case *ir.LocalExpr:
		slot, err := c.slot(n.Symbol, n.Name, n)
		if err != nil {
			return err
		}
		c.emit(OpLoadLocal, slot, 0, pos.Line, pos.Column)
		return nil
	case *ir.UnaryExpr:
		if err := c.compileExpr(n.Right); err != nil {
			return err
		}
		switch n.Op {
		case ir.OpNegI64Checked, ir.OpNegF64:
			c.emit(OpNeg, 0, 0, pos.Line, pos.Column)
		case ir.OpNotBool:
			c.emit(OpNot, 0, 0, pos.Line, pos.Column)
		default:
			return fmt.Errorf("%d:%d: compiler: unsupported IR unary operation %q", pos.Line, pos.Column, n.Op)
		}
		return nil
	case *ir.BinaryExpr:
		switch n.Op {
		case ir.OpLogicalAndShortCircuit:
			if err := c.compileExpr(n.Left); err != nil {
				return err
			}
			jumpFalse := c.emit(OpJumpIfFalse, -1, 0, pos.Line, pos.Column)
			if err := c.compileExpr(n.Right); err != nil {
				return err
			}
			jumpEnd := c.emit(OpJump, -1, 0, pos.Line, pos.Column)
			c.patch(jumpFalse, len(c.code))
			c.emit(OpConst, c.addConstant(Bool(false)), 0, pos.Line, pos.Column)
			c.patch(jumpEnd, len(c.code))
			return nil
		case ir.OpLogicalOrShortCircuit:
			if err := c.compileExpr(n.Left); err != nil {
				return err
			}
			jumpTrue := c.emit(OpJumpIfTrue, -1, 0, pos.Line, pos.Column)
			if err := c.compileExpr(n.Right); err != nil {
				return err
			}
			jumpEnd := c.emit(OpJump, -1, 0, pos.Line, pos.Column)
			c.patch(jumpTrue, len(c.code))
			c.emit(OpConst, c.addConstant(Bool(true)), 0, pos.Line, pos.Column)
			c.patch(jumpEnd, len(c.code))
			return nil
		}
		if err := c.compileExpr(n.Left); err != nil {
			return err
		}
		if err := c.compileExpr(n.Right); err != nil {
			return err
		}
		op, ok := bytecodeOpForIR(n.Op)
		if !ok {
			return fmt.Errorf("%d:%d: compiler: unsupported IR binary operation %q", pos.Line, pos.Column, n.Op)
		}
		c.emit(op, 0, 0, pos.Line, pos.Column)
		return nil
	case *ir.CallExpr:
		for _, arg := range n.Args {
			if err := c.compileExpr(arg); err != nil {
				return err
			}
		}
		switch n.Target {
		case ir.CallStructConstructor:
			if n.StructID < 0 || n.StructID >= len(c.program.StructTypes) {
				return fmt.Errorf("%d:%d: compiler: invalid struct id %d", pos.Line, pos.Column, n.StructID)
			}
			if len(n.Args) != len(c.program.StructTypes[n.StructID].Fields) {
				return fmt.Errorf("%d:%d: compiler: struct %q has wrong constructor arity", pos.Line, pos.Column, n.FunctionName)
			}
			c.emit(OpBuildStruct, n.StructID, len(n.Args), pos.Line, pos.Column)
		case ir.CallPrint:
			c.emit(OpBuiltin, 0, len(n.Args), pos.Line, pos.Column)
		case ir.CallLen:
			c.emit(OpBuiltin, 1, len(n.Args), pos.Line, pos.Column)
		case ir.CallAssert:
			c.emit(OpBuiltin, 2, len(n.Args), pos.Line, pos.Column)
		case ir.CallI64Cast:
			if len(n.Args) != 1 {
				return fmt.Errorf("%d:%d: compiler: malformed i64 cast call", pos.Line, pos.Column)
			}
			if n.Args[0].ExprType() == "f64" {
				c.emit(OpToI64, 0, 0, pos.Line, pos.Column)
			} else if n.Args[0].ExprType() != "i64" {
				return fmt.Errorf("%d:%d: compiler: invalid i64 cast source type %q", pos.Line, pos.Column, n.Args[0].ExprType())
			}
		case ir.CallF64Cast:
			if len(n.Args) != 1 {
				return fmt.Errorf("%d:%d: compiler: malformed f64 cast call", pos.Line, pos.Column)
			}
			if n.Args[0].ExprType() == "i64" {
				c.emit(OpToF64, 0, 0, pos.Line, pos.Column)
			} else if n.Args[0].ExprType() != "f64" {
				return fmt.Errorf("%d:%d: compiler: invalid f64 cast source type %q", pos.Line, pos.Column, n.Args[0].ExprType())
			}
		case ir.CallUserFunction:
			if n.Function < 0 || int(n.Function) >= len(c.program.Functions)-1 {
				return fmt.Errorf("%d:%d: compiler: invalid IR function id %d", pos.Line, pos.Column, n.Function)
			}
			c.emit(OpCall, int(n.Function), len(n.Args), pos.Line, pos.Column)
		default:
			return fmt.Errorf("%d:%d: compiler: unsupported IR call target %q", pos.Line, pos.Column, n.Target)
		}
		return nil
	default:
		return fmt.Errorf("%d:%d: compiler: unsupported IR expression %T", pos.Line, pos.Column, expr)
	}
}

func bytecodeOpForIR(op ir.RuntimeOp) (Op, bool) {
	switch op {
	case ir.OpAddI64Checked, ir.OpSubI64Checked, ir.OpMulI64Checked, ir.OpDivI64Checked,
		ir.OpModI64Checked, ir.OpAddF64Checked, ir.OpSubF64Checked, ir.OpMulF64Checked,
		ir.OpDivF64Checked, ir.OpConcatStringChecked:
		switch op {
		case ir.OpAddI64Checked, ir.OpAddF64Checked, ir.OpConcatStringChecked:
			return OpAdd, true
		case ir.OpSubI64Checked, ir.OpSubF64Checked:
			return OpSub, true
		case ir.OpMulI64Checked, ir.OpMulF64Checked:
			return OpMul, true
		case ir.OpDivI64Checked, ir.OpDivF64Checked:
			return OpDiv, true
		case ir.OpModI64Checked:
			return OpMod, true
		}
	case ir.OpEqual:
		return OpEqual, true
	case ir.OpNotEqual:
		return OpNotEqual, true
	case ir.OpLess:
		return OpLess, true
	case ir.OpLessEqual:
		return OpLessEqual, true
	case ir.OpGreater:
		return OpGreater, true
	case ir.OpGreaterEqual:
		return OpGreaterEqual, true
	}
	return "", false
}
