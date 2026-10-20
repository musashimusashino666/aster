// Package checker performs static type checking before any Aster program runs.
package checker

import (
	"fmt"
	"strings"

	"github.com/aster-lang/aster/internal/ast"
	"github.com/aster-lang/aster/internal/langtypes"
	"github.com/aster-lang/aster/internal/token"
)

type Type string

const (
	I64    Type = "i64"
	F64    Type = "f64"
	Bool   Type = "bool"
	String Type = "string"
	Unit   Type = "unit"
)

func (t Type) String() string { return string(t) }

type FunctionInfo struct {
	Name   string
	Params []Type
	Return Type
	Decl   *ast.Function
}

// TypedProgram is the checked view of an AST. Expression types and function
// signatures are produced by the checker and consumed by backend generators;
// backends must not independently infer these semantic facts.
type TypedProgram struct {
	Tree            *ast.Program
	expressionTypes map[ast.Expr]Type
	functions       map[string]FunctionInfo
}

// TypeOf returns the checker's resolved type for expr.
func (p *TypedProgram) TypeOf(expr ast.Expr) (Type, bool) {
	if p == nil {
		return "", false
	}
	t, ok := p.expressionTypes[expr]
	return t, ok
}

// Function returns the resolved signature for a declared function.
func (p *TypedProgram) Function(name string) (FunctionInfo, bool) {
	if p == nil {
		return FunctionInfo{}, false
	}
	info, ok := p.functions[name]
	if !ok {
		return FunctionInfo{}, false
	}
	info.Params = append([]Type(nil), info.Params...)
	return info, true
}
type Variable struct {
	Type    Type
	Mutable bool
}
type scope struct {
	vars   map[string]Variable
	parent *scope
}

func newScope(parent *scope) *scope { return &scope{vars: map[string]Variable{}, parent: parent} }
func (s *scope) lookup(name string) (Variable, bool) {
	for x := s; x != nil; x = x.parent {
		v, ok := x.vars[name]
		if ok {
			return v, true
		}
	}
	return Variable{}, false
}
func (s *scope) define(name string, v Variable) bool {
	if _, ok := s.vars[name]; ok {
		return false
	}
	s.vars[name] = v
	return true
}

type Checker struct {
	functions       map[string]FunctionInfo
	expressionTypes map[ast.Expr]Type
	currentReturn   Type
	inFunction      bool
	scopes          *scope
}

// Check validates program without retaining expression-type metadata.
func Check(program *ast.Program) error {
	c := &Checker{functions: map[string]FunctionInfo{}}
	return c.checkProgram(program)
}

// Analyze validates program and returns the semantic information produced by
// the checker for use by compiler backends.
func Analyze(program *ast.Program) (*TypedProgram, error) {
	c := &Checker{
		functions:       map[string]FunctionInfo{},
		expressionTypes: map[ast.Expr]Type{},
	}
	if err := c.checkProgram(program); err != nil {
		return nil, err
	}
	return &TypedProgram{
		Tree:            program,
		expressionTypes: c.expressionTypes,
		functions:       c.functions,
	}, nil
}

func (c *Checker) checkProgram(program *ast.Program) error {
	// First pass enables forward calls and recursion.
	for _, d := range program.Decls {
		f, ok := d.(*ast.Function)
		if !ok {
			continue
		}
		if _, exists := c.functions[f.Name]; exists {
			return c.err(f.Pos(), "function '%s' is already declared", f.Name)
		}
		params := make([]Type, len(f.Params))
		for i, p := range f.Params {
			if builtinName(p.Name) {
				return c.err(p.At, "parameter name '%s' is reserved", p.Name)
			}
			t, err := typeFromRef(p.Type)
			if err != nil {
				return c.err(p.At, "%v", err)
			}
			if t == Unit {
				return c.err(p.At, "parameter '%s' cannot have type unit", p.Name)
			}
			params[i] = t
		}
		ret := Unit
		if f.ReturnType != nil {
			t, err := typeFromRef(*f.ReturnType)
			if err != nil {
				return c.err(f.ReturnType.At, "%v", err)
			}
			ret = t
		}
		if builtinName(f.Name) {
			return c.err(f.Pos(), "function name '%s' is reserved", f.Name)
		}
		c.functions[f.Name] = FunctionInfo{Name: f.Name, Params: params, Return: ret, Decl: f}
	}
	// Validate duplicate parameter names and check function bodies.
	for _, d := range program.Decls {
		f, ok := d.(*ast.Function)
		if !ok {
			continue
		}
		info := c.functions[f.Name]
		c.inFunction = true
		c.currentReturn = info.Return
		c.scopes = newScope(nil)
		for i, p := range f.Params {
			if !c.scopes.define(p.Name, Variable{Type: info.Params[i]}) {
				return c.err(p.At, "duplicate parameter '%s'", p.Name)
			}
		}
		if _, err := c.checkBlock(f.Body, false); err != nil {
			return err
		}
		if info.Return != Unit && !AlwaysReturns(f.Body) {
			return c.err(f.Pos(), "function '%s' must return %s on every path", f.Name, info.Return)
		}
	}
	c.inFunction = false
	c.currentReturn = Unit
	c.scopes = newScope(nil)
	for _, d := range program.Decls {
		sd, ok := d.(*ast.StmtDecl)
		if !ok {
			continue
		}
		if _, err := c.checkStmt(sd.Statement); err != nil {
			return err
		}
	}
	return nil
}
func builtinName(n string) bool { return n == "print" || n == "len" || n == "assert" }
func typeFromName(s string) (Type, error) {
	switch s {
	case "i64":
		return I64, nil
	case "f64":
		return F64, nil
	case "bool":
		return Bool, nil
	case "string":
		return String, nil
	case "unit":
		return Unit, nil
	default:
		return "", fmt.Errorf("unknown type '%s'", s)
	}
}
func typeFromRef(r ast.TypeRef) (Type, error) {
	return typeFromRefDepth(r, 0)
}

func typeFromRefDepth(r ast.TypeRef, depth int) (Type, error) {
	if r.Name == "Array" {
		if r.Element == nil {
			return "", fmt.Errorf("type Array requires an element type, e.g. Array<i64>")
		}
		if depth >= langtypes.MaxNestingDepth {
			return "", fmt.Errorf("type nesting exceeds limit %d", langtypes.MaxNestingDepth)
		}
		elem, err := typeFromRefDepth(*r.Element, depth+1)
		if err != nil {
			return "", err
		}
		if elem == Unit {
			return "", fmt.Errorf("Array cannot contain unit values")
		}
		return Type("Array<" + elem.String() + ">"), nil
	}
	if r.Element != nil {
		return "", fmt.Errorf("type '%s' does not accept a generic type argument", r.Name)
	}
	return typeFromName(r.Name)
}
func arrayElement(t Type) (Type, bool) {
	s := string(t)
	if strings.HasPrefix(s, "Array<") && strings.HasSuffix(s, ">") {
		return Type(s[len("Array<") : len(s)-1]), true
	}
	return "", false
}
func comparableType(t Type) bool { return t == I64 || t == F64 || t == Bool || t == String }

func (c *Checker) checkBlock(b *ast.BlockStmt, push bool) (bool, error) {
	old := c.scopes
	if push {
		c.scopes = newScope(old)
		defer func() { c.scopes = old }()
	}
	returned := false
	for _, s := range b.Statements {
		if returned {
			return false, c.err(s.Pos(), "unreachable statement")
		}
		r, err := c.checkStmt(s)
		if err != nil {
			return false, err
		}
		returned = r
	}
	return returned, nil
}
func (c *Checker) checkStmt(s ast.Stmt) (bool, error) {
	switch n := s.(type) {
	case *ast.BlockStmt:
		return c.checkBlock(n, true)
	case *ast.LetStmt:
		if n.Name == "print" || n.Name == "len" || n.Name == "assert" {
			return false, c.err(n.Pos(), "'%s' is reserved", n.Name)
		}
		if _, exists := c.scopes.vars[n.Name]; exists {
			return false, c.err(n.Pos(), "variable '%s' is already declared in this scope", n.Name)
		}
		valueType, err := c.checkExpr(n.Value)
		if err != nil {
			return false, err
		}
		if n.Annotation != nil {
			annotated, e := typeFromRef(*n.Annotation)
			if e != nil {
				return false, c.err(n.Annotation.At, "%v", e)
			}
			if annotated == Unit {
				return false, c.err(n.Annotation.At, "variables cannot have type unit")
			}
			if annotated != valueType {
				return false, c.err(n.Pos(), "cannot assign %s to variable of type %s", valueType, annotated)
			}
			valueType = annotated
		}
		if valueType == Unit {
			return false, c.err(n.Value.Pos(), "variables cannot hold unit values; use the call as an expression statement")
		}
		c.scopes.define(n.Name, Variable{Type: valueType, Mutable: n.Mutable})
		return false, nil
	case *ast.AssignStmt:
		v, ok := c.scopes.lookup(n.Name)
		if !ok {
			return false, c.err(n.Pos(), "undefined variable '%s'", n.Name)
		}
		if !v.Mutable {
			return false, c.err(n.Pos(), "cannot assign to immutable variable '%s'; declare it with 'let mut'", n.Name)
		}
		got, err := c.checkExpr(n.Value)
		if err != nil {
			return false, err
		}
		if got != v.Type {
			return false, c.err(n.Pos(), "cannot assign %s to variable '%s' of type %s", got, n.Name, v.Type)
		}
		return false, nil
	case *ast.ExprStmt:
		_, err := c.checkExpr(n.Expr)
		return false, err
	case *ast.IfStmt:
		cond, err := c.checkExpr(n.Condition)
		if err != nil {
			return false, err
		}
		if cond != Bool {
			return false, c.err(n.Condition.Pos(), "if condition must be bool, got %s", cond)
		}
		thenRet, err := c.checkBlock(n.Then, true)
		if err != nil {
			return false, err
		}
		elseRet := false
		if n.Else != nil {
			elseRet, err = c.checkStmt(n.Else)
			if err != nil {
				return false, err
			}
		}
		return n.Else != nil && thenRet && elseRet, nil
	case *ast.WhileStmt:
		cond, err := c.checkExpr(n.Condition)
		if err != nil {
			return false, err
		}
		if cond != Bool {
			return false, c.err(n.Condition.Pos(), "while condition must be bool, got %s", cond)
		}
		_, err = c.checkBlock(n.Body, true)
		return false, err
	case *ast.ReturnStmt:
		if !c.inFunction {
			return false, c.err(n.Pos(), "return is only valid inside a function")
		}
		got := Unit
		var err error
		if n.Value != nil {
			got, err = c.checkExpr(n.Value)
			if err != nil {
				return false, err
			}
		}
		if got != c.currentReturn {
			return false, c.err(n.Pos(), "return type mismatch: function returns %s but expression has type %s", c.currentReturn, got)
		}
		return true, nil
	default:
		return false, c.err(s.Pos(), "unsupported statement node %T", s)
	}
}
func (c *Checker) checkExpr(e ast.Expr) (Type, error) {
	t, err := c.checkExprImpl(e)
	if err == nil && c.expressionTypes != nil {
		c.expressionTypes[e] = t
	}
	return t, err
}

// checkExprImpl performs type inference/checking. Recursive children are routed
// through checkExpr so every successfully checked expression is recorded.
func (c *Checker) checkExprImpl(e ast.Expr) (Type, error) {
	switch n := e.(type) {
	case *ast.LiteralExpr:
		switch n.Kind {
		case "i64":
			return I64, nil
		case "f64":
			return F64, nil
		case "bool":
			return Bool, nil
		case "string":
			return String, nil
		}
	case *ast.ArrayExpr:
		if len(n.Elements) == 0 {
			return "", c.err(n.Pos(), "cannot infer the element type of an empty array; provide at least one element")
		}
		elemType, err := c.checkExpr(n.Elements[0])
		if err != nil {
			return "", err
		}
		if elemType == Unit {
			return "", c.err(n.Elements[0].Pos(), "arrays cannot contain unit values")
		}
		for _, item := range n.Elements[1:] {
			t, err := c.checkExpr(item)
			if err != nil {
				return "", err
			}
			if t != elemType {
				return "", c.err(item.Pos(), "array elements must have one type; expected %s, got %s", elemType, t)
			}
		}
		arrayType := Type("Array<" + elemType.String() + ">")
		if !langtypes.ValidName(arrayType.String()) {
			return "", c.err(n.Pos(), "type nesting exceeds limit %d", langtypes.MaxNestingDepth)
		}
		return arrayType, nil
	case *ast.IndexExpr:
		collectionType, err := c.checkExpr(n.Collection)
		if err != nil {
			return "", err
		}
		elemType, ok := arrayElement(collectionType)
		if !ok {
			return "", c.err(n.Collection.Pos(), "indexing requires an Array<T>, got %s", collectionType)
		}
		indexType, err := c.checkExpr(n.Index)
		if err != nil {
			return "", err
		}
		if indexType != I64 {
			return "", c.err(n.Index.Pos(), "array index must be i64, got %s", indexType)
		}
		return elemType, nil
	case *ast.IdentifierExpr:
		v, ok := c.scopes.lookup(n.Name)
		if !ok {
			return "", c.err(n.Pos(), "undefined variable '%s'", n.Name)
		}
		return v.Type, nil
	case *ast.UnaryExpr:
		t, err := c.checkExpr(n.Right)
		if err != nil {
			return "", err
		}
		switch n.Op {
		case "-":
			if t == I64 || t == F64 {
				return t, nil
			}
			return "", c.err(n.Pos(), "unary '-' requires a number, got %s", t)
		case "!":
			if t == Bool {
				return Bool, nil
			}
			return "", c.err(n.Pos(), "'!' requires bool, got %s", t)
		}
	case *ast.BinaryExpr:
		l, err := c.checkExpr(n.Left)
		if err != nil {
			return "", err
		}
		r, err := c.checkExpr(n.Right)
		if err != nil {
			return "", err
		}
		switch n.Op {
		case "+":
			if l == String && r == String {
				return String, nil
			}
			if l == r && (l == I64 || l == F64) {
				return l, nil
			}
			return "", c.err(n.Pos(), "operator '+' does not support %s and %s", l, r)
		case "-", "*", "/", "%":
			if l != r || (l != I64 && l != F64) {
				return "", c.err(n.Pos(), "operator '%s' requires matching numeric types, got %s and %s", n.Op, l, r)
			}
			if n.Op == "%" && l != I64 {
				return "", c.err(n.Pos(), "operator '%%' requires i64 operands")
			}
			return l, nil
		case "<", "<=", ">", ">=":
			if l != r || !(l == I64 || l == F64 || l == String) {
				return "", c.err(n.Pos(), "operator '%s' requires matching numbers or strings", n.Op)
			}
			return Bool, nil
		case "==", "!=":
			if l != r || !comparableType(l) {
				return "", c.err(n.Pos(), "cannot compare values of type %s and %s", l, r)
			}
			return Bool, nil
		case "&&", "||":
			if l != Bool || r != Bool {
				return "", c.err(n.Pos(), "operator '%s' requires bool operands", n.Op)
			}
			return Bool, nil
		}
	case *ast.CallExpr:
		id, ok := n.Callee.(*ast.IdentifierExpr)
		if !ok {
			return "", c.err(n.Pos(), "only named functions can be called")
		}
		args := n.Args
		if id.Name == "print" {
			if len(args) != 1 {
				return "", c.err(n.Pos(), "print expects exactly one argument")
			}
			argType, err := c.checkExpr(args[0])
			if err != nil {
				return "", err
			}
			if argType == Unit {
				return "", c.err(args[0].Pos(), "print cannot display a unit value")
			}
			return Unit, nil
		}
		if id.Name == "len" {
			if len(args) != 1 {
				return "", c.err(n.Pos(), "len expects exactly one argument")
			}
			t, err := c.checkExpr(args[0])
			if err != nil {
				return "", err
			}
			if t != String {
				if _, ok := arrayElement(t); !ok {
					return "", c.err(n.Pos(), "len accepts string or Array<T>, got %s", t)
				}
			}
			return I64, nil
		}
		if id.Name == "assert" {
			if len(args) != 1 {
				return "", c.err(n.Pos(), "assert expects exactly one argument")
			}
			t, err := c.checkExpr(args[0])
			if err != nil {
				return "", err
			}
			if t != Bool {
				return "", c.err(n.Pos(), "assert expects bool, got %s", t)
			}
			return Unit, nil
		}
		fn, ok := c.functions[id.Name]
		if !ok {
			return "", c.err(n.Pos(), "undefined function '%s'", id.Name)
		}
		if len(args) != len(fn.Params) {
			return "", c.err(n.Pos(), "function '%s' expects %d arguments, got %d", id.Name, len(fn.Params), len(args))
		}
		for i, arg := range args {
			t, err := c.checkExpr(arg)
			if err != nil {
				return "", err
			}
			if t != fn.Params[i] {
				return "", c.err(arg.Pos(), "argument %d to '%s' has type %s; expected %s", i+1, id.Name, t, fn.Params[i])
			}
		}
		return fn.Return, nil
	}
	return "", c.err(e.Pos(), "unsupported expression node %T", e)
}
// AlwaysReturns reports whether the conservative return-flow analysis guarantees a return.
func AlwaysReturns(s ast.Stmt) bool {
	switch n := s.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BlockStmt:
		for _, child := range n.Statements {
			if AlwaysReturns(child) {
				return true
			}
		}
	case *ast.IfStmt:
		if n.Else != nil && AlwaysReturns(n.Then) && AlwaysReturns(n.Else) {
			return true
		}
	}
	return false
}
func (c *Checker) err(pos token.Position, format string, args ...any) error {
	return fmt.Errorf("%d:%d: type error: %s", pos.Line, pos.Column, fmt.Sprintf(format, args...))
}
