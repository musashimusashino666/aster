package bytecode

import (
	"fmt"

	"github.com/aster-lang/aster/internal/ast"
)

// Compile lowers a type-checked Aster AST into portable Aster bytecode.
func Compile(tree *ast.Program) (*Program, error) {
	p := &Program{Format: "aster-bytecode", Version: FormatVersion, Source: tree.SourceName, Constants: []Value{}}
	functionDecls := []*ast.Function{}
	for _, d := range tree.Decls {
		if f, ok := d.(*ast.Function); ok {
			functionDecls = append(functionDecls, f)
		}
	}
	ids := map[string]int{}
	for i, f := range functionDecls {
		ids[f.Name] = i
	}
	entry := len(functionDecls)
	ids["__aster_main"] = entry
	p.Entry = entry
	for _, f := range functionDecls {
		ret := "unit"
		if f.ReturnType != nil {
			ret = sourceTypeName(*f.ReturnType)
		}
		paramTypes := make([]string, len(f.Params))
		for i, param := range f.Params {
			paramTypes[i] = sourceTypeName(param.Type)
		}
		p.Functions = append(p.Functions, Function{Name: f.Name, Params: len(f.Params), ParamTypes: paramTypes, Return: ret})
	}
	p.Functions = append(p.Functions, Function{Name: "__aster_main", Return: "unit"})
	for i, f := range functionDecls {
		fc := newFunctionCompiler(p, ids, i, f.Name)
		fc.scope = newSlotScope(nil)
		for _, param := range f.Params {
			slot := fc.allocate(param.Name)
			if slot < 0 {
				return nil, fmt.Errorf("duplicate parameter %q", param.Name)
			}
		}
		if err := fc.compileBlock(f.Body, false); err != nil {
			return nil, err
		}
		if f.ReturnType == nil || sourceTypeName(*f.ReturnType) == "unit" {
			fc.emit(OpConst, fc.addConstant(Unit()), 0, f.Pos().Line, f.Pos().Column)
			fc.emit(OpReturn, 0, 0, f.Pos().Line, f.Pos().Column)
		}
		p.Functions[i].Locals = fc.nextSlot
		p.Functions[i].Code = fc.code
	}
	main := newFunctionCompiler(p, ids, entry, "__aster_main")
	main.scope = newSlotScope(nil)
	for _, d := range tree.Decls {
		sd, ok := d.(*ast.StmtDecl)
		if !ok {
			continue
		}
		if err := main.compileStmt(sd.Statement); err != nil {
			return nil, err
		}
	}
	main.emit(OpHalt, 0, 0, 1, 1)
	p.Functions[entry].Locals = main.nextSlot
	p.Functions[entry].Code = main.code
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

type slotScope struct {
	slots  map[string]int
	parent *slotScope
}

func newSlotScope(parent *slotScope) *slotScope {
	return &slotScope{slots: map[string]int{}, parent: parent}
}
func (s *slotScope) find(n string) (int, bool) {
	for x := s; x != nil; x = x.parent {
		v, ok := x.slots[n]
		if ok {
			return v, true
		}
	}
	return 0, false
}

type functionCompiler struct {
	program  *Program
	ids      map[string]int
	fnID     int
	name     string
	code     []Instruction
	scope    *slotScope
	nextSlot int
}

func newFunctionCompiler(p *Program, ids map[string]int, id int, name string) *functionCompiler {
	return &functionCompiler{program: p, ids: ids, fnID: id, name: name}
}
func (c *functionCompiler) emit(op Op, a, b, line, col int) int {
	c.code = append(c.code, Instruction{Op: op, A: a, B: b, Line: line, Column: col})
	return len(c.code) - 1
}
func (c *functionCompiler) patch(at, target int) { c.code[at].A = target }
func (c *functionCompiler) addConstant(v Value) int {
	c.program.Constants = append(c.program.Constants, v)
	return len(c.program.Constants) - 1
}
func (c *functionCompiler) allocate(name string) int {
	if _, exists := c.scope.slots[name]; exists {
		return -1
	}
	slot := c.nextSlot
	c.nextSlot++
	c.scope.slots[name] = slot
	return slot
}
func (c *functionCompiler) compileBlock(b *ast.BlockStmt, push bool) error {
	old := c.scope
	if push {
		c.scope = newSlotScope(old)
		defer func() { c.scope = old }()
	}
	for _, s := range b.Statements {
		if err := c.compileStmt(s); err != nil {
			return err
		}
	}
	return nil
}
func (c *functionCompiler) compileStmt(s ast.Stmt) error {
	pos := s.Pos()
	switch n := s.(type) {
	case *ast.BlockStmt:
		return c.compileBlock(n, true)
	case *ast.LetStmt:
		if err := c.compileExpr(n.Value); err != nil {
			return err
		}
		slot := c.allocate(n.Name)
		if slot < 0 {
			return fmt.Errorf("%d:%d: compiler: duplicate local %q", pos.Line, pos.Column, n.Name)
		}
		c.emit(OpStoreLocal, slot, 0, pos.Line, pos.Column)
		return nil
	case *ast.AssignStmt:
		slot, ok := c.scope.find(n.Name)
		if !ok {
			return fmt.Errorf("%d:%d: compiler: unknown local %q", pos.Line, pos.Column, n.Name)
		}
		if err := c.compileExpr(n.Value); err != nil {
			return err
		}
		c.emit(OpStoreLocal, slot, 0, pos.Line, pos.Column)
		return nil
	case *ast.ExprStmt:
		if err := c.compileExpr(n.Expr); err != nil {
			return err
		}
		c.emit(OpPop, 0, 0, pos.Line, pos.Column)
		return nil
	case *ast.IfStmt:
		if err := c.compileExpr(n.Condition); err != nil {
			return err
		}
		jf := c.emit(OpJumpIfFalse, -1, 0, pos.Line, pos.Column)
		if err := c.compileBlock(n.Then, true); err != nil {
			return err
		}
		if n.Else != nil {
			thenReturns := statementAlwaysReturns(n.Then)
			jend := -1
			if !thenReturns {
				jend = c.emit(OpJump, -1, 0, pos.Line, pos.Column)
			}
			c.patch(jf, len(c.code))
			if err := c.compileStmt(n.Else); err != nil {
				return err
			}
			if jend >= 0 {
				c.patch(jend, len(c.code))
			}
		} else {
			c.patch(jf, len(c.code))
		}
		return nil
	case *ast.WhileStmt:
		start := len(c.code)
		if err := c.compileExpr(n.Condition); err != nil {
			return err
		}
		exit := c.emit(OpJumpIfFalse, -1, 0, pos.Line, pos.Column)
		if err := c.compileBlock(n.Body, true); err != nil {
			return err
		}
		c.emit(OpJump, start, 0, pos.Line, pos.Column)
		c.patch(exit, len(c.code))
		return nil
	case *ast.ReturnStmt:
		if n.Value == nil {
			c.emit(OpConst, c.addConstant(Unit()), 0, pos.Line, pos.Column)
		} else if err := c.compileExpr(n.Value); err != nil {
			return err
		}
		c.emit(OpReturn, 0, 0, pos.Line, pos.Column)
		return nil
	default:
		return fmt.Errorf("%d:%d: compiler: unsupported statement %T", pos.Line, pos.Column, s)
	}
}
func (c *functionCompiler) compileExpr(e ast.Expr) error {
	pos := e.Pos()
	switch n := e.(type) {
	case *ast.LiteralExpr:
		var v Value
		switch n.Kind {
		case "i64":
			v = I64(n.Int)
		case "f64":
			v = F64(n.Float)
		case "bool":
			v = Bool(n.Bool)
		case "string":
			v = Str(n.String)
		default:
			return fmt.Errorf("%d:%d: compiler: unknown literal type %s", pos.Line, pos.Column, n.Kind)
		}
		c.emit(OpConst, c.addConstant(v), 0, pos.Line, pos.Column)
		return nil
	case *ast.ArrayExpr:
		for _, item := range n.Elements {
			if err := c.compileExpr(item); err != nil {
				return err
			}
		}
		c.emit(OpBuildArray, len(n.Elements), 0, pos.Line, pos.Column)
		return nil
	case *ast.IndexExpr:
		if err := c.compileExpr(n.Collection); err != nil {
			return err
		}
		if err := c.compileExpr(n.Index); err != nil {
			return err
		}
		c.emit(OpIndex, 0, 0, pos.Line, pos.Column)
		return nil
	case *ast.IdentifierExpr:
		slot, ok := c.scope.find(n.Name)
		if !ok {
			return fmt.Errorf("%d:%d: compiler: unknown local %q", pos.Line, pos.Column, n.Name)
		}
		c.emit(OpLoadLocal, slot, 0, pos.Line, pos.Column)
		return nil
	case *ast.UnaryExpr:
		if err := c.compileExpr(n.Right); err != nil {
			return err
		}
		if n.Op == "-" {
			c.emit(OpNeg, 0, 0, pos.Line, pos.Column)
		} else {
			c.emit(OpNot, 0, 0, pos.Line, pos.Column)
		}
		return nil
	case *ast.BinaryExpr:
		if n.Op == "&&" {
			if err := c.compileExpr(n.Left); err != nil {
				return err
			}
			jf := c.emit(OpJumpIfFalse, -1, 0, pos.Line, pos.Column)
			if err := c.compileExpr(n.Right); err != nil {
				return err
			}
			jend := c.emit(OpJump, -1, 0, pos.Line, pos.Column)
			c.patch(jf, len(c.code))
			c.emit(OpConst, c.addConstant(Bool(false)), 0, pos.Line, pos.Column)
			c.patch(jend, len(c.code))
			return nil
		}
		if n.Op == "||" {
			if err := c.compileExpr(n.Left); err != nil {
				return err
			}
			jt := c.emit(OpJumpIfTrue, -1, 0, pos.Line, pos.Column)
			if err := c.compileExpr(n.Right); err != nil {
				return err
			}
			jend := c.emit(OpJump, -1, 0, pos.Line, pos.Column)
			c.patch(jt, len(c.code))
			c.emit(OpConst, c.addConstant(Bool(true)), 0, pos.Line, pos.Column)
			c.patch(jend, len(c.code))
			return nil
		}
		if err := c.compileExpr(n.Left); err != nil {
			return err
		}
		if err := c.compileExpr(n.Right); err != nil {
			return err
		}
		ops := map[string]Op{"+": OpAdd, "-": OpSub, "*": OpMul, "/": OpDiv, "%": OpMod, "==": OpEqual, "!=": OpNotEqual, "<": OpLess, "<=": OpLessEqual, ">": OpGreater, ">=": OpGreaterEqual}
		op, ok := ops[n.Op]
		if !ok {
			return fmt.Errorf("%d:%d: compiler: unknown operator %q", pos.Line, pos.Column, n.Op)
		}
		c.emit(op, 0, 0, pos.Line, pos.Column)
		return nil
	case *ast.CallExpr:
		id, ok := n.Callee.(*ast.IdentifierExpr)
		if !ok {
			return fmt.Errorf("%d:%d: compiler: only named functions can be called", pos.Line, pos.Column)
		}
		for _, arg := range n.Args {
			if err := c.compileExpr(arg); err != nil {
				return err
			}
		}
		switch id.Name {
		case "print":
			c.emit(OpBuiltin, 0, len(n.Args), pos.Line, pos.Column)
		case "len":
			c.emit(OpBuiltin, 1, len(n.Args), pos.Line, pos.Column)
		case "assert":
			c.emit(OpBuiltin, 2, len(n.Args), pos.Line, pos.Column)
		default:
			fid, ok := c.ids[id.Name]
			if !ok {
				return fmt.Errorf("%d:%d: compiler: unknown function %q", pos.Line, pos.Column, id.Name)
			}
			c.emit(OpCall, fid, len(n.Args), pos.Line, pos.Column)
		}
		return nil
	default:
		return fmt.Errorf("%d:%d: compiler: unsupported expression %T", pos.Line, pos.Column, e)
	}
}

func sourceTypeName(t ast.TypeRef) string {
	if t.Element != nil {
		return t.Name + "<" + sourceTypeName(*t.Element) + ">"
	}
	return t.Name
}


// statementAlwaysReturns mirrors the checker's conservative return-flow rule.
// The compiler uses it to avoid emitting an unreachable jump after a branch
// that cannot fall through, which keeps generated bytecode verifiable.
func statementAlwaysReturns(s ast.Stmt) bool {
	switch n := s.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BlockStmt:
		for _, child := range n.Statements {
			if statementAlwaysReturns(child) {
				return true
			}
		}
	case *ast.IfStmt:
		return n.Else != nil && statementAlwaysReturns(n.Then) && statementAlwaysReturns(n.Else)
	}
	return false
}
