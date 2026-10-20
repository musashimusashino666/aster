// Package checker performs static type checking before any Aster program runs.
package checker

import (
	"fmt"
	"strings"

	"github.com/aster-lang/aster/internal/ast"
	"github.com/aster-lang/aster/internal/diagnostic"
	"github.com/aster-lang/aster/internal/ir"
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
	IR              *ir.Program
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
	typed := &TypedProgram{
		Tree:            program,
		expressionTypes: c.expressionTypes,
		functions:       c.functions,
	}
	normalized, err := ir.Lower(
		program,
		func(expr ast.Expr) (ir.Type, bool) {
			t, ok := typed.TypeOf(expr)
			return ir.Type(t), ok
		},
		func(name string) (ir.FunctionSignature, bool) {
			info, ok := typed.Function(name)
			if !ok {
				return ir.FunctionSignature{}, false
			}
			params := make([]ir.Type, len(info.Params))
			for i, param := range info.Params {
				params[i] = ir.Type(param)
			}
			return ir.FunctionSignature{Params: params, Return: ir.Type(info.Return)}, true
		},
	)
	if err != nil {
		return nil, err
	}
	typed.IR = normalized
	return typed, nil
}

func (c *Checker) checkProgram(program *ast.Program) error {
	// First pass enables forward calls and recursion.
	for _, d := range program.Decls {
		f, ok := d.(*ast.Function)
		if !ok {
			continue
		}
		if _, exists := c.functions[f.Name]; exists {
			return typeError(f.Pos(), diagnostic.TypeDuplicateFunction, "function '%s' is already declared", f.Name)
		}
		params := make([]Type, len(f.Params))
		for i, p := range f.Params {
			if builtinName(p.Name) {
				return typeError(p.At, diagnostic.TypeReservedParameter, "parameter name '%s' is reserved", p.Name)
			}
			t, err := typeFromRef(p.Type)
			if err != nil {
				return typeError(p.At, diagnostic.TypeInvalidTypeAnnotation, "%v", err)
			}
			if t == Unit {
				return typeError(p.At, diagnostic.TypeUnitParameter, "parameter '%s' cannot have type unit", p.Name)
			}
			params[i] = t
		}
		ret := Unit
		if f.ReturnType != nil {
			t, err := typeFromRef(*f.ReturnType)
			if err != nil {
				return typeError(f.ReturnType.At, diagnostic.TypeInvalidTypeAnnotation, "%v", err)
			}
			ret = t
		}
		if builtinName(f.Name) {
			return typeError(f.Pos(), diagnostic.TypeReservedFunction, "function name '%s' is reserved", f.Name)
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
				return typeError(p.At, diagnostic.TypeDuplicateParameter, "duplicate parameter '%s'", p.Name)
			}
		}
		cannotFallThrough, err := c.checkBlock(f.Body, false)
		if err != nil {
			return err
		}
		if info.Return != Unit && !cannotFallThrough {
			return typeError(f.Pos(), diagnostic.TypeMissingReturn, "function '%s' must return %s on every path that reaches its end", f.Name, info.Return)
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
func builtinName(n string) bool {
	return n == "print" || n == "len" || n == "assert" || n == "i64" || n == "f64"
}
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
	cannotFallThrough := false
	for _, s := range b.Statements {
		if cannotFallThrough {
			return false, typeError(s.Pos(), diagnostic.TypeUnreachableStatement, "unreachable statement")
		}
		terminates, err := c.checkStmt(s)
		if err != nil {
			return false, err
		}
		cannotFallThrough = terminates
	}
	return cannotFallThrough, nil
}
func (c *Checker) checkStmt(s ast.Stmt) (bool, error) {
	switch n := s.(type) {
	case *ast.BlockStmt:
		return c.checkBlock(n, true)
	case *ast.LetStmt:
		if n.Name == "print" || n.Name == "len" || n.Name == "assert" {
			return false, typeError(n.Pos(), diagnostic.TypeReservedBinding, "'%s' is reserved", n.Name)
		}
		if _, exists := c.scopes.vars[n.Name]; exists {
			return false, typeError(n.Pos(), diagnostic.TypeDuplicateBinding, "variable '%s' is already declared in this scope", n.Name)
		}
		valueType, err := c.checkExpr(n.Value)
		if err != nil {
			return false, err
		}
		if n.Annotation != nil {
			annotated, e := typeFromRef(*n.Annotation)
			if e != nil {
				return false, typeError(n.Annotation.At, diagnostic.TypeInvalidTypeAnnotation, "%v", e)
			}
			if annotated == Unit {
				return false, typeError(n.Annotation.At, diagnostic.TypeUnitBinding, "variables cannot have type unit")
			}
			if annotated != valueType {
				return false, typeError(n.Pos(), diagnostic.TypeBindingTypeMismatch, "cannot assign %s to variable of type %s", valueType, annotated)
			}
			valueType = annotated
		}
		if valueType == Unit {
			return false, typeError(n.Value.Pos(), diagnostic.TypeUnitValueBinding, "variables cannot hold unit values; use the call as an expression statement")
		}
		c.scopes.define(n.Name, Variable{Type: valueType, Mutable: n.Mutable})
		return false, nil
	case *ast.AssignStmt:
		v, ok := c.scopes.lookup(n.Name)
		if !ok {
			return false, typeError(n.Pos(), diagnostic.TypeUndefinedVariable, "undefined variable '%s'", n.Name)
		}
		if !v.Mutable {
			return false, typeError(n.Pos(), diagnostic.TypeImmutableAssignment, "cannot assign to immutable variable '%s'; declare it with 'let mut'", n.Name)
		}
		got, err := c.checkExpr(n.Value)
		if err != nil {
			return false, err
		}
		if got != v.Type {
			return false, typeError(n.Pos(), diagnostic.TypeAssignmentTypeMismatch, "cannot assign %s to variable '%s' of type %s", got, n.Name, v.Type)
		}
		return false, nil
	case *ast.IndexAssignStmt:
		root, ok := indexedAssignmentRoot(n.Target)
		if !ok {
			return false, typeError(n.Pos(), diagnostic.TypeGeneric, "array assignment target must be rooted in a variable")
		}
		variable, exists := c.scopes.lookup(root.Name)
		if !exists {
			return false, typeError(root.Pos(), diagnostic.TypeUndefinedVariable, "undefined variable '%s'", root.Name)
		}
		if !variable.Mutable {
			return false, typeError(n.Pos(), diagnostic.TypeImmutableAssignment, "cannot assign to immutable variable '%s'; declare it with 'let mut'", root.Name)
		}
		elementType, err := c.checkExpr(n.Target)
		if err != nil {
			return false, err
		}
		valueType, err := c.checkExpr(n.Value)
		if err != nil {
			return false, err
		}
		if valueType != elementType {
			return false, typeError(n.Pos(), diagnostic.TypeIndexAssignmentMismatch, "cannot assign %s to array element of type %s", valueType, elementType)
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
			return false, typeError(n.Condition.Pos(), diagnostic.TypeIfCondition, "if condition must be bool, got %s", cond)
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
		if condition, ok := astBoolLiteral(n.Condition); ok {
			if condition {
				return thenRet, nil
			}
			if n.Else != nil {
				return elseRet, nil
			}
			return false, nil
		}
		return n.Else != nil && thenRet && elseRet, nil
	case *ast.WhileStmt:
		cond, err := c.checkExpr(n.Condition)
		if err != nil {
			return false, err
		}
		if cond != Bool {
			return false, typeError(n.Condition.Pos(), diagnostic.TypeWhileCondition, "while condition must be bool, got %s", cond)
		}
		_, err = c.checkBlock(n.Body, true)
		if err != nil {
			return false, err
		}
		condition, constant := astBoolLiteral(n.Condition)
		return constant && condition, nil
	case *ast.ReturnStmt:
		if !c.inFunction {
			return false, typeError(n.Pos(), diagnostic.TypeReturnOutsideFunction, "return is only valid inside a function")
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
			return false, typeError(n.Pos(), diagnostic.TypeReturnMismatch, "return type mismatch: function returns %s but expression has type %s", c.currentReturn, got)
		}
		return true, nil
	default:
		return false, typeError(s.Pos(), diagnostic.TypeUnsupportedStatement, "unsupported statement node %T", s)
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
			return "", typeError(n.Pos(), diagnostic.TypeEmptyArrayInference, "cannot infer the element type of an empty array; provide at least one element")
		}
		elemType, err := c.checkExpr(n.Elements[0])
		if err != nil {
			return "", err
		}
		if elemType == Unit {
			return "", typeError(n.Elements[0].Pos(), diagnostic.TypeUnitArrayElement, "arrays cannot contain unit values")
		}
		for _, item := range n.Elements[1:] {
			t, err := c.checkExpr(item)
			if err != nil {
				return "", err
			}
			if t != elemType {
				return "", typeError(item.Pos(), diagnostic.TypeHeterogeneousArray, "array elements must have one type; expected %s, got %s", elemType, t)
			}
		}
		arrayType := Type("Array<" + elemType.String() + ">")
		if !langtypes.ValidName(arrayType.String()) {
			return "", typeError(n.Pos(), diagnostic.TypeNestingLimit, "type nesting exceeds limit %d", langtypes.MaxNestingDepth)
		}
		return arrayType, nil
	case *ast.IndexExpr:
		collectionType, err := c.checkExpr(n.Collection)
		if err != nil {
			return "", err
		}
		elemType, ok := arrayElement(collectionType)
		if !ok {
			return "", typeError(n.Collection.Pos(), diagnostic.TypeIndexNonArray, "indexing requires an Array<T>, got %s", collectionType)
		}
		indexType, err := c.checkExpr(n.Index)
		if err != nil {
			return "", err
		}
		if indexType != I64 {
			return "", typeError(n.Index.Pos(), diagnostic.TypeIndexType, "array index must be i64, got %s", indexType)
		}
		return elemType, nil
	case *ast.SliceExpr:
		collectionType, err := c.checkExpr(n.Collection)
		if err != nil {
			return "", err
		}
		if _, ok := arrayElement(collectionType); !ok {
			return "", typeError(n.Collection.Pos(), diagnostic.TypeSliceNonArray, "slicing requires an Array<T>, got %s", collectionType)
		}
		startType, err := c.checkExpr(n.Start)
		if err != nil {
			return "", err
		}
		if startType != I64 {
			return "", typeError(n.Start.Pos(), diagnostic.TypeSliceBoundType, "slice bounds must be i64, got %s", startType)
		}
		endType, err := c.checkExpr(n.End)
		if err != nil {
			return "", err
		}
		if endType != I64 {
			return "", typeError(n.End.Pos(), diagnostic.TypeSliceBoundType, "slice bounds must be i64, got %s", endType)
		}
		return collectionType, nil
	case *ast.IdentifierExpr:
		v, ok := c.scopes.lookup(n.Name)
		if !ok {
			return "", typeError(n.Pos(), diagnostic.TypeUndefinedVariable, "undefined variable '%s'", n.Name)
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
			return "", typeError(n.Pos(), diagnostic.TypeUnaryMinusOperand, "unary '-' requires a number, got %s", t)
		case "!":
			if t == Bool {
				return Bool, nil
			}
			return "", typeError(n.Pos(), diagnostic.TypeNotOperand, "'!' requires bool, got %s", t)
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
			return "", typeError(n.Pos(), diagnostic.TypePlusOperands, "operator '+' does not support %s and %s", l, r)
		case "-", "*", "/", "%":
			if l != r || (l != I64 && l != F64) {
				return "", typeError(n.Pos(), diagnostic.TypeNumericOperands, "operator '%s' requires matching numeric types, got %s and %s", n.Op, l, r)
			}
			if n.Op == "%" && l != I64 {
				return "", typeError(n.Pos(), diagnostic.TypeRemainderOperands, "operator '%%' requires i64 operands")
			}
			return l, nil
		case "<", "<=", ">", ">=":
			if l != r || !(l == I64 || l == F64 || l == String) {
				return "", typeError(n.Pos(), diagnostic.TypeOperatorOperands, "operator '%s' requires matching numbers or strings", n.Op)
			}
			return Bool, nil
		case "==", "!=":
			if l != r || !comparableType(l) {
				return "", typeError(n.Pos(), diagnostic.TypeComparisonOperands, "cannot compare values of type %s and %s", l, r)
			}
			return Bool, nil
		case "&&", "||":
			if l != Bool || r != Bool {
				return "", typeError(n.Pos(), diagnostic.TypeLogicalOperands, "operator '%s' requires bool operands", n.Op)
			}
			return Bool, nil
		}
	case *ast.CallExpr:
		id, ok := n.Callee.(*ast.IdentifierExpr)
		if !ok {
			return "", typeError(n.Pos(), diagnostic.TypeCallTarget, "only named functions can be called")
		}
		args := n.Args
		if id.Name == "i64" || id.Name == "f64" {
			if len(args) != 1 {
				return "", typeError(n.Pos(), diagnostic.TypeNumericCastArity, "%s conversion expects exactly one argument", id.Name)
			}
			argType, err := c.checkExpr(args[0])
			if err != nil {
				return "", err
			}
			if argType != I64 && argType != F64 {
				return "", typeError(args[0].Pos(), diagnostic.TypeNumericCastOperand, "%s conversion requires i64 or f64, got %s", id.Name, argType)
			}
			if id.Name == "i64" {
				return I64, nil
			}
			return F64, nil
		}
		if id.Name == "print" {
			if len(args) != 1 {
				return "", typeError(n.Pos(), diagnostic.TypePrintArity, "print expects exactly one argument")
			}
			argType, err := c.checkExpr(args[0])
			if err != nil {
				return "", err
			}
			if argType == Unit {
				return "", typeError(args[0].Pos(), diagnostic.TypePrintUnit, "print cannot display a unit value")
			}
			return Unit, nil
		}
		if id.Name == "len" {
			if len(args) != 1 {
				return "", typeError(n.Pos(), diagnostic.TypeLenArity, "len expects exactly one argument")
			}
			t, err := c.checkExpr(args[0])
			if err != nil {
				return "", err
			}
			if t != String {
				if _, ok := arrayElement(t); !ok {
					return "", typeError(n.Pos(), diagnostic.TypeLenArgument, "len accepts string or Array<T>, got %s", t)
				}
			}
			return I64, nil
		}
		if id.Name == "assert" {
			if len(args) != 1 {
				return "", typeError(n.Pos(), diagnostic.TypeAssertArity, "assert expects exactly one argument")
			}
			t, err := c.checkExpr(args[0])
			if err != nil {
				return "", err
			}
			if t != Bool {
				return "", typeError(n.Pos(), diagnostic.TypeAssertArgument, "assert expects bool, got %s", t)
			}
			return Unit, nil
		}
		fn, ok := c.functions[id.Name]
		if !ok {
			return "", typeError(n.Pos(), diagnostic.TypeUndefinedFunction, "undefined function '%s'", id.Name)
		}
		if len(args) != len(fn.Params) {
			return "", typeError(n.Pos(), diagnostic.TypeFunctionArity, "function '%s' expects %d arguments, got %d", id.Name, len(fn.Params), len(args))
		}
		for i, arg := range args {
			t, err := c.checkExpr(arg)
			if err != nil {
				return "", err
			}
			if t != fn.Params[i] {
				return "", typeError(arg.Pos(), diagnostic.TypeArgumentMismatch, "argument %d to '%s' has type %s; expected %s", i+1, id.Name, t, fn.Params[i])
			}
		}
		return fn.Return, nil
	}
	return "", typeError(e.Pos(), diagnostic.TypeUnsupportedExpression, "unsupported expression node %T", e)
}

func indexedAssignmentRoot(expr ast.Expr) (*ast.IdentifierExpr, bool) {
	for {
		index, ok := expr.(*ast.IndexExpr)
		if !ok {
			break
		}
		expr = index.Collection
	}
	root, ok := expr.(*ast.IdentifierExpr)
	return root, ok
}

func astBoolLiteral(expr ast.Expr) (bool, bool) {
	literal, ok := expr.(*ast.LiteralExpr)
	if !ok || literal.Kind != "bool" {
		return false, false
	}
	return literal.Bool, true
}

func typeError(pos token.Position, code string, format string, args ...any) error {
	return diagnostic.New(code, "type", pos, fmt.Sprintf(format, args...))
}
