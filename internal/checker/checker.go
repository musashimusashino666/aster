// Package checker performs static type checking before any Aster program runs.
package checker

import (
	"fmt"
	"strings"

	"github.com/aster-lang/aster/internal/ast"
	"github.com/aster-lang/aster/internal/diagnostic"
	"github.com/aster-lang/aster/internal/generics"
	"github.com/aster-lang/aster/internal/ir"
	"github.com/aster-lang/aster/internal/langtypes"
	"github.com/aster-lang/aster/internal/limits"
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

type StructFieldInfo struct {
	Name string
	Type Type
	Decl ast.StructField
}

type StructInfo struct {
	Name   string
	Fields []StructFieldInfo
	Decl   *ast.StructDecl
}

type EnumVariantInfo struct {
	Name    string
	Payload []Type
	Decl    ast.EnumVariant
}

type EnumInfo struct {
	Name     string
	Variants []EnumVariantInfo
	Decl     *ast.EnumDecl
}

// TypedProgram is the checked view of an AST. Expression types and function
// signatures are produced by the checker and consumed by backend generators;
// backends must not independently infer these semantic facts.
type TypedProgram struct {
	Tree            *ast.Program
	IR              *ir.Program
	expressionTypes map[ast.Expr]Type
	functions       map[string]FunctionInfo
	structs         map[string]StructInfo
	enums           map[string]EnumInfo
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
	structs         map[string]StructInfo
	enums           map[string]EnumInfo
	expressionTypes map[ast.Expr]Type
	currentReturn   Type
	inFunction      bool
	scopes          *scope
}

// Check validates program without retaining expression-type metadata.
func Check(program *ast.Program) error {
	expanded, err := generics.Expand(program)
	if err != nil {
		return err
	}
	c := &Checker{functions: map[string]FunctionInfo{}, structs: map[string]StructInfo{}, enums: map[string]EnumInfo{}}
	return c.checkProgram(expanded)
}

// Analyze validates program and returns the semantic information produced by
// the checker for use by compiler backends.
func Analyze(program *ast.Program) (*TypedProgram, error) {
	expanded, err := generics.Expand(program)
	if err != nil {
		return nil, err
	}
	program = expanded
	c := &Checker{
		functions:       map[string]FunctionInfo{},
		structs:         map[string]StructInfo{},
		enums:           map[string]EnumInfo{},
		expressionTypes: map[ast.Expr]Type{},
	}
	if err := c.checkProgram(program); err != nil {
		return nil, err
	}
	typed := &TypedProgram{
		Tree:            program,
		expressionTypes: c.expressionTypes,
		functions:       c.functions,
		enums:           c.enums,
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
	// Register nominal types before checking function signatures so fields,
	// parameters, and return types may refer to later declarations.
	functionNames := make(map[string]struct{})
	for _, d := range program.Decls {
		if f, ok := d.(*ast.Function); ok {
			functionNames[f.Name] = struct{}{}
		}
	}
	enumTypeCount := 0
	enumVariantCount := 0
	enumPayloadCount := 0
	// Register every enum name before resolving any payload type, so enums and
	// structs can refer forward to one another in the same source file.
	for _, d := range program.Decls {
		e, ok := d.(*ast.EnumDecl)
		if !ok {
			continue
		}
		enumTypeCount++
		if enumTypeCount > limits.MaxEnumTypes {
			return typeError(e.Pos(), diagnostic.TypeResourceLimit, "program exceeds enum type limit (%d)", limits.MaxEnumTypes)
		}
		if !langtypes.ValidName(e.Name) || builtinName(e.Name) || builtinTypeName(e.Name) {
			return typeError(e.Pos(), diagnostic.TypeDuplicateEnum, "invalid or reserved enum name '%s'", e.Name)
		}
		if _, exists := functionNames[e.Name]; exists {
			return typeError(e.Pos(), diagnostic.TypeDuplicateEnum, "enum name '%s' conflicts with a function", e.Name)
		}
		if _, exists := c.enums[e.Name]; exists {
			return typeError(e.Pos(), diagnostic.TypeDuplicateEnum, "enum '%s' is already declared", e.Name)
		}
		info := EnumInfo{Name: e.Name, Decl: e}
		seen := make(map[string]struct{})
		for _, variant := range e.Variants {
			enumVariantCount++
			if enumVariantCount > limits.MaxEnumVariants {
				return typeError(variant.At, diagnostic.TypeResourceLimit, "program exceeds enum variant limit (%d)", limits.MaxEnumVariants)
			}
			enumPayloadCount += len(variant.Payload)
			if enumPayloadCount > limits.MaxEnumPayloadFields {
				return typeError(variant.At, diagnostic.TypeResourceLimit, "program exceeds enum payload-field limit (%d)", limits.MaxEnumPayloadFields)
			}
			if len(variant.Name) == 0 || len(variant.Name) > langtypes.MaxTypeNameBytes || !langtypes.ValidName(variant.Name) {
				return typeError(variant.At, diagnostic.TypeDuplicateEnumVariant, "invalid variant name '%s' in enum '%s'", variant.Name, e.Name)
			}
			if _, duplicate := seen[variant.Name]; duplicate {
				return typeError(variant.At, diagnostic.TypeDuplicateEnumVariant, "variant '%s' is duplicated in enum '%s'", variant.Name, e.Name)
			}
			seen[variant.Name] = struct{}{}
			info.Variants = append(info.Variants, EnumVariantInfo{Name: variant.Name, Decl: variant})
		}
		if len(info.Variants) == 0 {
			return typeError(e.Pos(), diagnostic.TypeDuplicateEnumVariant, "enum '%s' must declare at least one variant", e.Name)
		}
		c.enums[e.Name] = info
	}
	for _, d := range program.Decls {
		s, ok := d.(*ast.StructDecl)
		if !ok {
			continue
		}
		if !langtypes.ValidName(s.Name) {
			return typeError(s.Pos(), diagnostic.TypeInvalidStructName, "invalid or reserved struct name '%s'", s.Name)
		}
		if _, exists := c.enums[s.Name]; exists {
			return typeError(s.Pos(), diagnostic.TypeDuplicateStruct, "struct name '%s' conflicts with an enum", s.Name)
		}
		if builtinName(s.Name) || builtinTypeName(s.Name) {
			return typeError(s.Pos(), diagnostic.TypeDuplicateStruct, "struct name '%s' is reserved", s.Name)
		}
		if _, exists := functionNames[s.Name]; exists {
			return typeError(s.Pos(), diagnostic.TypeDuplicateStruct, "struct name '%s' conflicts with a function", s.Name)
		}
		if _, exists := c.structs[s.Name]; exists {
			return typeError(s.Pos(), diagnostic.TypeDuplicateStruct, "struct '%s' is already declared", s.Name)
		}
		c.structs[s.Name] = StructInfo{Name: s.Name, Decl: s}
	}
	// Resolve enum payload types after all nominal names have been registered.
	for _, d := range program.Decls {
		e, ok := d.(*ast.EnumDecl)
		if !ok {
			continue
		}
		info := c.enums[e.Name]
		for i, variant := range e.Variants {
			payload := make([]Type, 0, len(variant.Payload))
			for _, payloadRef := range variant.Payload {
				typ, err := c.typeFromRef(payloadRef)
				if err != nil || typ == Unit {
					if err == nil {
						err = fmt.Errorf("enum payloads cannot have type unit")
					}
					return typeError(payloadRef.At, diagnostic.TypeInvalidEnumPayload, "invalid payload type for %s::%s: %v", e.Name, variant.Name, err)
				}
				payload = append(payload, typ)
			}
			info.Variants[i].Payload = payload
		}
		c.enums[e.Name] = info
	}
	// Populate field metadata now that all nominal names are known.
	for _, d := range program.Decls {
		s, ok := d.(*ast.StructDecl)
		if !ok {
			continue
		}
		fields := make([]StructFieldInfo, 0, len(s.Fields))
		seen := make(map[string]struct{})
		for _, field := range s.Fields {
			if len(field.Name) > langtypes.MaxTypeNameBytes || (!langtypes.ValidName(field.Name) && field.Name != "Array") {
				return typeError(field.At, diagnostic.TypeInvalidStructFieldName, "invalid struct field name '%s'", field.Name)
			}
			if _, exists := seen[field.Name]; exists {
				return typeError(field.At, diagnostic.TypeDuplicateStructField, "field '%s' is duplicated in struct '%s'", field.Name, s.Name)
			}
			seen[field.Name] = struct{}{}
			typ, err := c.typeFromRef(field.Type)
			if err != nil {
				return typeError(field.At, diagnostic.TypeInvalidTypeAnnotation, "%v", err)
			}
			if typ == Unit {
				return typeError(field.At, diagnostic.TypeInvalidStructField, "struct fields cannot have type unit")
			}
			fields = append(fields, StructFieldInfo{Name: field.Name, Type: typ, Decl: field})
		}
		info := c.structs[s.Name]
		info.Fields = fields
		c.structs[s.Name] = info
	}
	if err := c.validateStructLayoutCycles(); err != nil {
		return err
	}

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
			t, err := c.typeFromRef(p.Type)
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
			t, err := c.typeFromRef(*f.ReturnType)
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
	cannotFallThrough := false
	for _, d := range program.Decls {
		sd, ok := d.(*ast.StmtDecl)
		if !ok {
			continue
		}
		if cannotFallThrough {
			return typeError(sd.Statement.Pos(), diagnostic.TypeUnreachableStatement, "unreachable statement")
		}
		terminates, err := c.checkStmt(sd.Statement)
		if err != nil {
			return err
		}
		cannotFallThrough = terminates
	}
	return nil
}
func builtinName(n string) bool {
	return n == "print" || n == "len" || n == "assert" || n == "i64" || n == "f64"
}

func builtinTypeName(n string) bool {
	switch n {
	case "i64", "f64", "bool", "string", "unit", "Array":
		return true
	default:
		return false
	}
}

// typeFromRef preserves the package-level helper used by primitive/Array
// type-depth tests. Named types require a Checker with its declaration table.
func typeFromRef(r ast.TypeRef) (Type, error) {
	return (&Checker{structs: map[string]StructInfo{}}).typeFromRef(r)
}

func (c *Checker) typeFromRef(r ast.TypeRef) (Type, error) {
	return c.typeFromRefDepth(r, 0)
}

func (c *Checker) typeFromRefDepth(r ast.TypeRef, depth int) (Type, error) {
	if r.Name == "Array" {
		if r.Element == nil {
			return "", fmt.Errorf("type Array requires an element type, e.g. Array<i64>")
		}
		if depth >= langtypes.MaxNestingDepth {
			return "", fmt.Errorf("type nesting exceeds limit %d", langtypes.MaxNestingDepth)
		}
		elem, err := c.typeFromRefDepth(*r.Element, depth+1)
		if err != nil {
			return "", err
		}
		if elem == Unit {
			return "", fmt.Errorf("Array cannot contain unit values")
		}
		return Type("Array<" + elem.String() + ">"), nil
	}
	if r.Element != nil || len(r.Args) > 0 {
		return "", fmt.Errorf("type '%s' does not accept generic type arguments", r.Name)
	}
	switch r.Name {
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
		if _, ok := c.structs[r.Name]; ok {
			return Type(r.Name), nil
		}
		if _, ok := c.enums[r.Name]; ok {
			return Type(r.Name), nil
		}
		return "", fmt.Errorf("unknown type '%s'", r.Name)
	}
}

// validateStructLayoutCycles rejects direct by-value recursive nominal layouts
// across structs and enums. Arrays are an indirection boundary.
func (c *Checker) validateStructLayoutCycles() error {
	state := make(map[string]uint8)
	var visit func(string) error
	visit = func(name string) error {
		switch state[name] {
		case 1:
			if info, ok := c.structs[name]; ok {
				return typeError(info.Decl.Pos(), diagnostic.TypeRecursiveStruct, "struct '%s' has a recursive by-value layout", name)
			}
			info := c.enums[name]
			return typeError(info.Decl.Pos(), diagnostic.TypeRecursiveEnumLayout, "enum '%s' has a recursive by-value layout", name)
		case 2:
			return nil
		}
		state[name] = 1
		if info, ok := c.structs[name]; ok {
			for _, field := range info.Fields {
				if _, isArray := arrayElement(field.Type); isArray {
					continue
				}
				target := string(field.Type)
				if _, isStruct := c.structs[target]; isStruct {
					if err := visit(target); err != nil {
						return err
					}
				} else if _, isEnum := c.enums[target]; isEnum {
					if err := visit(target); err != nil {
						return err
					}
				}
			}
		} else if info, ok := c.enums[name]; ok {
			for _, variant := range info.Variants {
				for _, payloadType := range variant.Payload {
					if _, isArray := arrayElement(payloadType); isArray {
						continue
					}
					target := string(payloadType)
					if _, isStruct := c.structs[target]; isStruct {
						if err := visit(target); err != nil {
							return err
						}
					} else if _, isEnum := c.enums[target]; isEnum {
						if err := visit(target); err != nil {
							return err
						}
					}
				}
			}
		}
		state[name] = 2
		return nil
	}
	for name := range c.structs {
		if err := visit(name); err != nil {
			return err
		}
	}
	for name := range c.enums {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

func printableType(t Type) bool {
	if elem, ok := arrayElement(t); ok {
		return printableType(elem)
	}
	return t == I64 || t == F64 || t == Bool || t == String
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
		if builtinName(n.Name) {
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
			annotated, e := c.typeFromRef(*n.Annotation)
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

// checkExprExpected checks an expression using a known expected type. In particular,
// an empty array literal is well-typed when its element type is supplied by the
// surrounding struct field; the same context is propagated to nested arrays.
func (c *Checker) checkExprExpected(e ast.Expr, expected Type) (Type, error) {
	array, ok := e.(*ast.ArrayExpr)
	elemType, isArray := arrayElement(expected)
	if !ok || !isArray {
		return c.checkExpr(e)
	}
	for _, item := range array.Elements {
		got, err := c.checkExprExpected(item, elemType)
		if err != nil {
			return "", err
		}
		if got != elemType {
			return "", typeError(item.Pos(), diagnostic.TypeHeterogeneousArray,
				"array elements must have one type; expected %s, got %s", elemType, got)
		}
	}
	if c.expressionTypes != nil {
		c.expressionTypes[e] = expected
	}
	return expected, nil
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
	case *ast.EnumVariantExpr:
		info, ok := c.enums[n.EnumName]
		if !ok {
			return "", typeError(n.Pos(), diagnostic.TypeUnknownEnumVariant, "unknown enum '%s'", n.EnumName)
		}
		variantIndex := -1
		variant := EnumVariantInfo{}
		for i, candidate := range info.Variants {
			if candidate.Name == n.VariantName {
				variantIndex, variant = i, candidate
				break
			}
		}
		if variantIndex < 0 {
			return "", typeError(n.Pos(), diagnostic.TypeUnknownEnumVariant, "enum '%s' has no variant '%s'", n.EnumName, n.VariantName)
		}
		if n.HasPayload != (len(variant.Payload) > 0) || len(n.Args) != len(variant.Payload) {
			return "", typeError(n.Pos(), diagnostic.TypeEnumConstructorArity, "variant %s::%s requires %d payload arguments", n.EnumName, n.VariantName, len(variant.Payload))
		}
		for i, arg := range n.Args {
			got, err := c.checkExprExpected(arg, variant.Payload[i])
			if err != nil {
				return "", err
			}
			if got != variant.Payload[i] {
				return "", typeError(arg.Pos(), diagnostic.TypeEnumPayloadMismatch, "payload %d of %s::%s expects %s, got %s", i+1, n.EnumName, n.VariantName, variant.Payload[i], got)
			}
		}
		return Type(n.EnumName), nil
	case *ast.MatchExpr:
		return c.checkMatchExpr(n)
	case *ast.FieldExpr:
		objectType, err := c.checkExpr(n.Object)
		if err != nil {
			return "", err
		}
		info, ok := c.structs[string(objectType)]
		if !ok {
			return "", typeError(n.Pos(), diagnostic.TypeFieldOnNonStruct, "field selection requires a struct value, got %s", objectType)
		}
		for _, field := range info.Fields {
			if field.Name == n.Name {
				return field.Type, nil
			}
		}
		return "", typeError(n.Pos(), diagnostic.TypeUnknownStructField, "struct '%s' has no field '%s'", info.Name, n.Name)
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
		if len(n.TypeArgs) > 0 {
			return "", typeError(n.Pos(), diagnostic.TypeGenericArgumentsOnNonGeneric, "explicit type arguments are not supported for this call")
		}
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
			if !printableType(argType) {
				return "", typeError(args[0].Pos(), diagnostic.TypePrintStructUnsupported, "print does not yet support values of type %s", argType)
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
		if structInfo, isStruct := c.structs[id.Name]; isStruct {
			if len(args) != len(structInfo.Fields) {
				return "", typeError(n.Pos(), diagnostic.TypeStructConstructorArity, "struct '%s' expects %d fields, got %d", id.Name, len(structInfo.Fields), len(args))
			}
			for i, arg := range args {
				got, err := c.checkExprExpected(arg, structInfo.Fields[i].Type)
				if err != nil {
					return "", err
				}
				want := structInfo.Fields[i].Type
				if got != want {
					return "", typeError(arg.Pos(), diagnostic.TypeStructFieldMismatch, "field '%s' of struct '%s' expects %s, got %s", structInfo.Fields[i].Name, id.Name, want, got)
				}
			}
			return Type(id.Name), nil
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

func (c *Checker) checkMatchExpr(n *ast.MatchExpr) (Type, error) {
	subjectType, err := c.checkExpr(n.Subject)
	if err != nil {
		return "", err
	}
	enumInfo, ok := c.enums[string(subjectType)]
	if !ok {
		return "", typeError(n.Subject.Pos(), diagnostic.TypeMatchNonEnum, "match subject must have an enum type, got %s", subjectType)
	}
	if len(n.Arms) == 0 {
		return "", typeError(n.Pos(), diagnostic.TypeMatchNonExhaustive, "match expression requires at least one arm")
	}
	covered := make(map[int]bool, len(enumInfo.Variants))
	wildcardSeen := false
	var resultType Type
	haveResult := false
	for _, arm := range n.Arms {
		pattern := arm.Pattern
		var bindings []Variable
		if pattern.Wildcard {
			if wildcardSeen || len(covered) == len(enumInfo.Variants) {
				return "", typeError(pattern.At, diagnostic.TypeMatchUnreachableArm, "wildcard arm cannot match any remaining variant")
			}
			wildcardSeen = true
		} else {
			if wildcardSeen {
				return "", typeError(pattern.At, diagnostic.TypeMatchUnreachableArm, "arm for %s::%s is unreachable after wildcard", pattern.EnumName, pattern.VariantName)
			}
			if pattern.EnumName != enumInfo.Name {
				return "", typeError(pattern.At, diagnostic.TypeMatchVariantEnum, "pattern %s::%s does not belong to matched enum %s", pattern.EnumName, pattern.VariantName, enumInfo.Name)
			}
			variantIndex := -1
			variant := EnumVariantInfo{}
			for i, candidate := range enumInfo.Variants {
				if candidate.Name == pattern.VariantName {
					variantIndex, variant = i, candidate
					break
				}
			}
			if variantIndex < 0 {
				return "", typeError(pattern.At, diagnostic.TypeUnknownEnumVariant, "enum '%s' has no variant '%s'", pattern.EnumName, pattern.VariantName)
			}
			if covered[variantIndex] {
				return "", typeError(pattern.At, diagnostic.TypeMatchDuplicateArm, "variant %s::%s is matched more than once", pattern.EnumName, pattern.VariantName)
			}
			covered[variantIndex] = true
			if pattern.HasPayload != (len(variant.Payload) > 0) || len(pattern.Bindings) != len(variant.Payload) {
				return "", typeError(pattern.At, diagnostic.TypeMatchPayloadArity, "pattern %s::%s requires %d payload bindings", pattern.EnumName, pattern.VariantName, len(variant.Payload))
			}
			seenNames := make(map[string]struct{}, len(pattern.Bindings))
			for i, name := range pattern.Bindings {
				if len(name) == 0 || len(name) > langtypes.MaxTypeNameBytes || !langtypes.ValidName(name) {
					return "", typeError(pattern.At, diagnostic.TypeMatchDuplicateBinding, "invalid payload binding '%s'", name)
				}
				if _, duplicate := seenNames[name]; duplicate {
					return "", typeError(pattern.At, diagnostic.TypeMatchDuplicateBinding, "payload binding '%s' is repeated", name)
				}
				seenNames[name] = struct{}{}
				if builtinName(name) {
					return "", typeError(pattern.At, diagnostic.TypeReservedBinding, "payload binding '%s' is reserved", name)
				}
				bindings = append(bindings, Variable{Type: variant.Payload[i]})
			}
		}
		oldScope := c.scopes
		c.scopes = newScope(oldScope)
		if !pattern.Wildcard {
			for i, name := range pattern.Bindings {
				if !c.scopes.define(name, bindings[i]) {
					c.scopes = oldScope
					return "", typeError(pattern.At, diagnostic.TypeMatchDuplicateBinding, "payload binding '%s' is repeated", name)
				}
			}
		}
		armType, armErr := c.checkExpr(arm.Expr)
		c.scopes = oldScope
		if armErr != nil {
			return "", armErr
		}
		if !haveResult {
			resultType, haveResult = armType, true
		} else if resultType != armType {
			return "", typeError(arm.At, diagnostic.TypeMatchArmTypeMismatch, "match arms must have one result type; expected %s, got %s", resultType, armType)
		}
	}
	if !wildcardSeen && len(covered) != len(enumInfo.Variants) {
		var missing []string
		for i, variant := range enumInfo.Variants {
			if !covered[i] {
				missing = append(missing, variant.Name)
			}
		}
		return "", typeError(n.Pos(), diagnostic.TypeMatchNonExhaustive, "match on %s is missing variants: %s", enumInfo.Name, strings.Join(missing, ", "))
	}
	if !haveResult {
		return "", typeError(n.Pos(), diagnostic.TypeMatchNonExhaustive, "match expression has no reachable arms")
	}
	return resultType, nil
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
