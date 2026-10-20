// Package ir defines the normalized, typed intermediate representation shared by
// Aster compiler backends. It preserves source positions, resolved local and
// function identities, and explicit runtime-checking operations.
package ir

import (
	"fmt"

	"github.com/aster-lang/aster/internal/ast"
	"github.com/aster-lang/aster/internal/token"
)

type Type string

type FunctionID int
type SymbolID int

type FunctionSignature struct {
	Params []Type
	Return Type
}

type TypeResolver func(ast.Expr) (Type, bool)
type FunctionResolver func(name string) (FunctionSignature, bool)

type Program struct {
	SourceName string
	Functions  []*Function
	Body       []Stmt
}

type Node interface {
	Pos() token.Position
}

type Function struct {
	At         token.Position
	ID         FunctionID
	Name       string
	Params     []Local
	ReturnType Type
	Body       *BlockStmt
}

func (n *Function) Pos() token.Position { return n.At }

type Local struct {
	At      token.Position
	ID      SymbolID
	Name    string
	Type    Type
	Mutable bool
}

type Stmt interface {
	Node
	stmtNode()
}

type BlockStmt struct {
	At         token.Position
	Statements []Stmt
}

func (n *BlockStmt) Pos() token.Position { return n.At }
func (*BlockStmt) stmtNode()             {}

type LetStmt struct {
	At    token.Position
	Local Local
	Value Expr
}

func (n *LetStmt) Pos() token.Position { return n.At }
func (*LetStmt) stmtNode()             {}

type AssignStmt struct {
	At     token.Position
	Target SymbolID
	Name   string
	Value  Expr
}

func (n *AssignStmt) Pos() token.Position { return n.At }
func (*AssignStmt) stmtNode()             {}

type ExprStmt struct {
	At   token.Position
	Expr Expr
}

func (n *ExprStmt) Pos() token.Position { return n.At }
func (*ExprStmt) stmtNode()             {}

type IfStmt struct {
	At        token.Position
	Condition Expr
	Then      *BlockStmt
	Else      Stmt
}

func (n *IfStmt) Pos() token.Position { return n.At }
func (*IfStmt) stmtNode()             {}

type WhileStmt struct {
	At        token.Position
	Condition Expr
	Body      *BlockStmt
}

func (n *WhileStmt) Pos() token.Position { return n.At }
func (*WhileStmt) stmtNode()             {}

type ReturnStmt struct {
	At    token.Position
	Value Expr
}

func (n *ReturnStmt) Pos() token.Position { return n.At }
func (*ReturnStmt) stmtNode()             {}

type Expr interface {
	Node
	exprNode()
	ExprType() Type
}

type LiteralExpr struct {
	At           token.Position
	ResolvedType Type
	Kind         string
	Int          int64
	Float        float64
	String       string
	Bool         bool
}

func (n *LiteralExpr) Pos() token.Position { return n.At }
func (*LiteralExpr) exprNode()             {}
func (n *LiteralExpr) ExprType() Type      { return n.ResolvedType }

type LocalExpr struct {
	At           token.Position
	ResolvedType Type
	Symbol       SymbolID
	Name         string
}

func (n *LocalExpr) Pos() token.Position { return n.At }
func (*LocalExpr) exprNode()             {}
func (n *LocalExpr) ExprType() Type      { return n.ResolvedType }

type UnaryExpr struct {
	At           token.Position
	ResolvedType Type
	Op           RuntimeOp
	OperandType  Type
	Right        Expr
}

func (n *UnaryExpr) Pos() token.Position { return n.At }
func (*UnaryExpr) exprNode()             {}
func (n *UnaryExpr) ExprType() Type      { return n.ResolvedType }

type BinaryExpr struct {
	At           token.Position
	ResolvedType Type
	OperandType  Type
	Op           RuntimeOp
	Left         Expr
	Right        Expr
}

func (n *BinaryExpr) Pos() token.Position { return n.At }
func (*BinaryExpr) exprNode()             {}
func (n *BinaryExpr) ExprType() Type      { return n.ResolvedType }

type ArrayExpr struct {
	At           token.Position
	ResolvedType Type
	Elements     []Expr
}

func (n *ArrayExpr) Pos() token.Position { return n.At }
func (*ArrayExpr) exprNode()             {}
func (n *ArrayExpr) ExprType() Type      { return n.ResolvedType }

type IndexExpr struct {
	At           token.Position
	ResolvedType Type
	Collection   Expr
	Index        Expr
	Op           RuntimeOp
}

func (n *IndexExpr) Pos() token.Position { return n.At }
func (*IndexExpr) exprNode()             {}
func (n *IndexExpr) ExprType() Type      { return n.ResolvedType }

type CallTarget string

const (
	CallUserFunction CallTarget = "user_function"
	CallPrint        CallTarget = "print"
	CallLen          CallTarget = "len"
	CallAssert       CallTarget = "assert"
)

type CallExpr struct {
	At           token.Position
	ResolvedType Type
	Target       CallTarget
	Function     FunctionID
	FunctionName string
	Args         []Expr
}

func (n *CallExpr) Pos() token.Position { return n.At }
func (*CallExpr) exprNode()             {}
func (n *CallExpr) ExprType() Type      { return n.ResolvedType }

// RuntimeOp records the language-level operation selected after type checking.
// Checked operations must trap rather than inherit unchecked Go arithmetic.
type RuntimeOp string

const (
	OpNegI64Checked RuntimeOp = "neg_i64_checked"
	OpNegF64        RuntimeOp = "neg_f64"
	OpNotBool       RuntimeOp = "not_bool"

	OpAddI64Checked       RuntimeOp = "add_i64_checked"
	OpSubI64Checked       RuntimeOp = "sub_i64_checked"
	OpMulI64Checked       RuntimeOp = "mul_i64_checked"
	OpDivI64Checked       RuntimeOp = "div_i64_checked"
	OpModI64Checked       RuntimeOp = "mod_i64_checked"
	OpAddF64Checked       RuntimeOp = "add_f64_checked"
	OpSubF64Checked       RuntimeOp = "sub_f64_checked"
	OpMulF64Checked       RuntimeOp = "mul_f64_checked"
	OpDivF64Checked       RuntimeOp = "div_f64_checked"
	OpConcatStringChecked RuntimeOp = "concat_string_checked"

	OpEqual                  RuntimeOp = "equal"
	OpNotEqual               RuntimeOp = "not_equal"
	OpLess                   RuntimeOp = "less"
	OpLessEqual              RuntimeOp = "less_equal"
	OpGreater                RuntimeOp = "greater"
	OpGreaterEqual           RuntimeOp = "greater_equal"
	OpLogicalAndShortCircuit RuntimeOp = "logical_and_short_circuit"
	OpLogicalOrShortCircuit  RuntimeOp = "logical_or_short_circuit"
	OpIndexChecked           RuntimeOp = "index_checked"
)

type scope struct {
	locals map[string]Local
	parent *scope
}

func newScope(parent *scope) *scope {
	return &scope{locals: make(map[string]Local), parent: parent}
}

func (s *scope) lookup(name string) (Local, bool) {
	for current := s; current != nil; current = current.parent {
		local, ok := current.locals[name]
		if ok {
			return local, true
		}
	}
	return Local{}, false
}

func (s *scope) define(local Local) bool {
	if _, exists := s.locals[local.Name]; exists {
		return false
	}
	s.locals[local.Name] = local
	return true
}

type lowerer struct {
	typeOf       TypeResolver
	functionOf   FunctionResolver
	functionIDs  map[string]FunctionID
	scope        *scope
	nextSymbolID SymbolID
}

func Lower(tree *ast.Program, typeOf TypeResolver, functionOf FunctionResolver) (*Program, error) {
	if tree == nil {
		return nil, fmt.Errorf("ir: nil AST program")
	}
	if typeOf == nil {
		return nil, fmt.Errorf("ir: nil expression type resolver")
	}
	if functionOf == nil {
		return nil, fmt.Errorf("ir: nil function resolver")
	}

	program := &Program{SourceName: tree.SourceName}
	l := &lowerer{
		typeOf:      typeOf,
		functionOf:  functionOf,
		functionIDs: make(map[string]FunctionID),
	}
	functionDecls := make([]*ast.Function, 0)
	for _, decl := range tree.Decls {
		if function, ok := decl.(*ast.Function); ok {
			id := FunctionID(len(functionDecls))
			l.functionIDs[function.Name] = id
			functionDecls = append(functionDecls, function)
		}
	}

	for _, decl := range functionDecls {
		signature, ok := functionOf(decl.Name)
		if !ok {
			return nil, fmt.Errorf("%d:%d: ir: missing checked signature for function %q", decl.Pos().Line, decl.Pos().Column, decl.Name)
		}
		if len(signature.Params) != len(decl.Params) {
			return nil, fmt.Errorf("%d:%d: ir: signature parameter count mismatch for function %q", decl.Pos().Line, decl.Pos().Column, decl.Name)
		}
		program.Functions = append(program.Functions, &Function{
			At:         decl.Pos(),
			ID:         l.functionIDs[decl.Name],
			Name:       decl.Name,
			ReturnType: signature.Return,
		})
	}

	for i, decl := range functionDecls {
		l.nextSymbolID = 0
		l.scope = newScope(nil)
		fn := program.Functions[i]
		signature, _ := functionOf(decl.Name)
		for j, param := range decl.Params {
			local := Local{
				At:   param.At,
				ID:   l.newSymbolID(),
				Name: param.Name,
				Type: signature.Params[j],
			}
			if !l.scope.define(local) {
				return nil, fmt.Errorf("%d:%d: ir: duplicate parameter %q", param.At.Line, param.At.Column, param.Name)
			}
			fn.Params = append(fn.Params, local)
		}
		body, err := l.lowerBlock(decl.Body, false)
		if err != nil {
			return nil, err
		}
		fn.Body = body
	}

	l.nextSymbolID = 0
	l.scope = newScope(nil)
	for _, decl := range tree.Decls {
		stmtDecl, ok := decl.(*ast.StmtDecl)
		if !ok {
			continue
		}
		stmt, err := l.lowerStmt(stmtDecl.Statement)
		if err != nil {
			return nil, err
		}
		program.Body = append(program.Body, stmt)
	}
	return program, nil
}

func (l *lowerer) newSymbolID() SymbolID {
	id := l.nextSymbolID
	l.nextSymbolID++
	return id
}

func (l *lowerer) lowerBlock(block *ast.BlockStmt, pushScope bool) (*BlockStmt, error) {
	old := l.scope
	if pushScope {
		l.scope = newScope(old)
		defer func() { l.scope = old }()
	}
	out := &BlockStmt{At: block.Pos()}
	for _, stmt := range block.Statements {
		lowered, err := l.lowerStmt(stmt)
		if err != nil {
			return nil, err
		}
		out.Statements = append(out.Statements, lowered)
	}
	return out, nil
}

func (l *lowerer) lowerStmt(stmt ast.Stmt) (Stmt, error) {
	switch n := stmt.(type) {
	case *ast.BlockStmt:
		return l.lowerBlock(n, true)
	case *ast.LetStmt:
		value, err := l.lowerExpr(n.Value)
		if err != nil {
			return nil, err
		}
		local := Local{At: n.Pos(), ID: l.newSymbolID(), Name: n.Name, Type: value.ExprType(), Mutable: n.Mutable}
		if !l.scope.define(local) {
			return nil, fmt.Errorf("%d:%d: ir: duplicate local %q", n.Pos().Line, n.Pos().Column, n.Name)
		}
		return &LetStmt{At: n.Pos(), Local: local, Value: value}, nil
	case *ast.AssignStmt:
		target, ok := l.scope.lookup(n.Name)
		if !ok {
			return nil, fmt.Errorf("%d:%d: ir: unresolved assignment target %q", n.Pos().Line, n.Pos().Column, n.Name)
		}
		value, err := l.lowerExpr(n.Value)
		if err != nil {
			return nil, err
		}
		return &AssignStmt{At: n.Pos(), Target: target.ID, Name: target.Name, Value: value}, nil
	case *ast.ExprStmt:
		expr, err := l.lowerExpr(n.Expr)
		if err != nil {
			return nil, err
		}
		return &ExprStmt{At: n.Pos(), Expr: expr}, nil
	case *ast.IfStmt:
		condition, err := l.lowerExpr(n.Condition)
		if err != nil {
			return nil, err
		}
		thenBlock, err := l.lowerBlock(n.Then, true)
		if err != nil {
			return nil, err
		}
		var elseStmt Stmt
		if n.Else != nil {
			elseStmt, err = l.lowerStmt(n.Else)
			if err != nil {
				return nil, err
			}
		}
		return &IfStmt{At: n.Pos(), Condition: condition, Then: thenBlock, Else: elseStmt}, nil
	case *ast.WhileStmt:
		condition, err := l.lowerExpr(n.Condition)
		if err != nil {
			return nil, err
		}
		body, err := l.lowerBlock(n.Body, true)
		if err != nil {
			return nil, err
		}
		return &WhileStmt{At: n.Pos(), Condition: condition, Body: body}, nil
	case *ast.ReturnStmt:
		var value Expr
		var err error
		if n.Value != nil {
			value, err = l.lowerExpr(n.Value)
			if err != nil {
				return nil, err
			}
		}
		return &ReturnStmt{At: n.Pos(), Value: value}, nil
	default:
		return nil, fmt.Errorf("%d:%d: ir: unsupported statement %T", stmt.Pos().Line, stmt.Pos().Column, stmt)
	}
}

func (l *lowerer) lowerExpr(expr ast.Expr) (Expr, error) {
	resolvedType, ok := l.typeOf(expr)
	if !ok {
		pos := expr.Pos()
		return nil, fmt.Errorf("%d:%d: ir: missing checked type for expression %T", pos.Line, pos.Column, expr)
	}
	switch n := expr.(type) {
	case *ast.LiteralExpr:
		return &LiteralExpr{
			At:           n.Pos(),
			ResolvedType: resolvedType,
			Kind:         n.Kind,
			Int:          n.Int,
			Float:        n.Float,
			String:       n.String,
			Bool:         n.Bool,
		}, nil
	case *ast.IdentifierExpr:
		local, exists := l.scope.lookup(n.Name)
		if !exists {
			return nil, fmt.Errorf("%d:%d: ir: unresolved local %q", n.Pos().Line, n.Pos().Column, n.Name)
		}
		if local.Type != resolvedType {
			return nil, fmt.Errorf("%d:%d: ir: resolved local type mismatch for %q", n.Pos().Line, n.Pos().Column, n.Name)
		}
		return &LocalExpr{At: n.Pos(), ResolvedType: resolvedType, Symbol: local.ID, Name: local.Name}, nil
	case *ast.UnaryExpr:
		right, err := l.lowerExpr(n.Right)
		if err != nil {
			return nil, err
		}
		op, ok := unaryOperation(n.Op, right.ExprType())
		if !ok {
			return nil, fmt.Errorf("%d:%d: ir: unsupported unary operator %q for %s", n.Pos().Line, n.Pos().Column, n.Op, right.ExprType())
		}
		return &UnaryExpr{At: n.Pos(), ResolvedType: resolvedType, Op: op, OperandType: right.ExprType(), Right: right}, nil
	case *ast.BinaryExpr:
		left, err := l.lowerExpr(n.Left)
		if err != nil {
			return nil, err
		}
		right, err := l.lowerExpr(n.Right)
		if err != nil {
			return nil, err
		}
		op, ok := binaryOperation(n.Op, left.ExprType())
		if !ok {
			return nil, fmt.Errorf("%d:%d: ir: unsupported binary operator %q for %s", n.Pos().Line, n.Pos().Column, n.Op, left.ExprType())
		}
		return &BinaryExpr{
			At: n.Pos(), ResolvedType: resolvedType, OperandType: left.ExprType(),
			Op: op, Left: left, Right: right,
		}, nil
	case *ast.ArrayExpr:
		elements := make([]Expr, len(n.Elements))
		for i, element := range n.Elements {
			lowered, err := l.lowerExpr(element)
			if err != nil {
				return nil, err
			}
			elements[i] = lowered
		}
		return &ArrayExpr{At: n.Pos(), ResolvedType: resolvedType, Elements: elements}, nil
	case *ast.IndexExpr:
		collection, err := l.lowerExpr(n.Collection)
		if err != nil {
			return nil, err
		}
		index, err := l.lowerExpr(n.Index)
		if err != nil {
			return nil, err
		}
		return &IndexExpr{At: n.Pos(), ResolvedType: resolvedType, Collection: collection, Index: index, Op: OpIndexChecked}, nil
	case *ast.CallExpr:
		callee, ok := n.Callee.(*ast.IdentifierExpr)
		if !ok {
			return nil, fmt.Errorf("%d:%d: ir: only named functions can be called", n.Pos().Line, n.Pos().Column)
		}
		args := make([]Expr, len(n.Args))
		for i, arg := range n.Args {
			lowered, err := l.lowerExpr(arg)
			if err != nil {
				return nil, err
			}
			args[i] = lowered
		}
		target := CallTarget("")
		functionID := FunctionID(-1)
		switch callee.Name {
		case "print":
			target = CallPrint
		case "len":
			target = CallLen
		case "assert":
			target = CallAssert
		default:
			var exists bool
			functionID, exists = l.functionIDs[callee.Name]
			if !exists {
				return nil, fmt.Errorf("%d:%d: ir: unresolved function %q", n.Pos().Line, n.Pos().Column, callee.Name)
			}
			target = CallUserFunction
		}
		return &CallExpr{
			At: n.Pos(), ResolvedType: resolvedType, Target: target,
			Function: functionID, FunctionName: callee.Name, Args: args,
		}, nil
	default:
		return nil, fmt.Errorf("%d:%d: ir: unsupported expression %T", expr.Pos().Line, expr.Pos().Column, expr)
	}
}

func unaryOperation(op string, operand Type) (RuntimeOp, bool) {
	switch op {
	case "-":
		switch operand {
		case "i64":
			return OpNegI64Checked, true
		case "f64":
			return OpNegF64, true
		}
	case "!":
		if operand == "bool" {
			return OpNotBool, true
		}
	}
	return "", false
}

func binaryOperation(op string, operand Type) (RuntimeOp, bool) {
	switch op {
	case "+":
		switch operand {
		case "i64":
			return OpAddI64Checked, true
		case "f64":
			return OpAddF64Checked, true
		case "string":
			return OpConcatStringChecked, true
		}
	case "-":
		switch operand {
		case "i64":
			return OpSubI64Checked, true
		case "f64":
			return OpSubF64Checked, true
		}
	case "*":
		switch operand {
		case "i64":
			return OpMulI64Checked, true
		case "f64":
			return OpMulF64Checked, true
		}
	case "/":
		switch operand {
		case "i64":
			return OpDivI64Checked, true
		case "f64":
			return OpDivF64Checked, true
		}
	case "%":
		if operand == "i64" {
			return OpModI64Checked, true
		}
	case "==":
		return OpEqual, true
	case "!=":
		return OpNotEqual, true
	case "<":
		return OpLess, true
	case "<=":
		return OpLessEqual, true
	case ">":
		return OpGreater, true
	case ">=":
		return OpGreaterEqual, true
	case "&&":
		if operand == "bool" {
			return OpLogicalAndShortCircuit, true
		}
	case "||":
		if operand == "bool" {
			return OpLogicalOrShortCircuit, true
		}
	}
	return "", false
}


// AlwaysReturns reports whether the checked control-flow shape guarantees a
// return on all paths represented by stmt. Loops remain conservative because
// the language does not assume that a loop executes at least once.
func AlwaysReturns(stmt Stmt) bool {
	switch n := stmt.(type) {
	case *ReturnStmt:
		return true
	case *BlockStmt:
		for _, child := range n.Statements {
			if AlwaysReturns(child) {
				return true
			}
		}
	case *IfStmt:
		return n.Else != nil && AlwaysReturns(n.Then) && AlwaysReturns(n.Else)
	}
	return false
}
