// Package generics expands explicit generic-function calls into concrete functions.
package generics

import (
	"fmt"
	"strings"

	"github.com/aster-lang/aster/internal/ast"
	"github.com/aster-lang/aster/internal/diagnostic"
	"github.com/aster-lang/aster/internal/langtypes"
	"github.com/aster-lang/aster/internal/limits"
	"github.com/aster-lang/aster/internal/token"
)

type expander struct {
	templates  map[string]*ast.Function
	knownTypes map[string]bool
	instances  map[string]*ast.Function
	ordered    []*ast.Function
	nextID     int
}

// Expand specializes explicitly instantiated generic functions before type
// checking. The returned AST contains only concrete functions; all later
// compiler stages remain unaware of generic templates.
func Expand(program *ast.Program) (*ast.Program, error) {
	if program == nil {
		return nil, fmt.Errorf("generics: nil program")
	}
	e := &expander{
		templates: map[string]*ast.Function{},
		knownTypes: map[string]bool{
			"i64": true, "f64": true, "bool": true, "string": true, "unit": true,
		},
		instances: map[string]*ast.Function{},
	}
	functionNames := map[string]*ast.Function{}
	for _, decl := range program.Decls {
		switch n := decl.(type) {
		case *ast.Function:
			if prior, exists := functionNames[n.Name]; exists {
				return nil, typeError(n.Pos(), diagnostic.TypeDuplicateFunction, "function '%s' is already declared", prior.Name)
			}
			functionNames[n.Name] = n
			if len(n.TypeParams) > 0 {
				if len(n.TypeParams) > limits.MaxGenericTypeParameters {
					return nil, typeError(n.Pos(), diagnostic.TypeGenericInstantiationLimit, "generic function '%s' declares %d type parameters; maximum is %d", n.Name, len(n.TypeParams), limits.MaxGenericTypeParameters)
				}
				seen := map[string]bool{}
				for _, name := range n.TypeParams {
					if !langtypes.ValidName(name) || reservedTypeName(name) || strings.HasPrefix(name, "__") || seen[name] {
						return nil, typeError(n.Pos(), diagnostic.TypeInvalidGenericParameter, "invalid, reserved, or duplicate type parameter '%s' in function '%s'", name, n.Name)
					}
					seen[name] = true
				}
				e.templates[n.Name] = n
			}
		case *ast.StructDecl:
			e.knownTypes[n.Name] = true
		case *ast.EnumDecl:
			e.knownTypes[n.Name] = true
		}
	}

	// Preserve AST node identity for programs that do not use generic templates.
	// Existing TypedProgram.TypeOf callers may retain expression pointers from the
	// parsed program, so avoid cloning the AST when expansion is unnecessary.
	if len(e.templates) == 0 {
		return program, nil
	}

	out := &ast.Program{SourceName: program.SourceName}
	for _, decl := range program.Decls {
		switch n := decl.(type) {
		case *ast.Function:
			if len(n.TypeParams) > 0 {
				continue // generic templates are never passed to the regular checker
			}
			copy, err := e.rewriteFunction(n, nil, 0)
			if err != nil {
				return nil, err
			}
			out.Decls = append(out.Decls, copy)
		case *ast.StmtDecl:
			stmt, err := e.rewriteStmt(n.Statement, nil, 0)
			if err != nil {
				return nil, err
			}
			out.Decls = append(out.Decls, &ast.StmtDecl{Statement: stmt})
		default:
			// Struct and enum declarations are immutable input to this phase.
			out.Decls = append(out.Decls, decl)
		}
	}
	out.Decls = append(out.Decls, funcDecls(e.ordered)...)
	return out, nil
}

func funcDecls(in []*ast.Function) []ast.Decl {
	out := make([]ast.Decl, len(in))
	for i, fn := range in {
		out[i] = fn
	}
	return out
}

func reservedTypeName(name string) bool {
	switch name {
	case "i64", "f64", "bool", "string", "unit", "Array":
		return true
	default:
		return false
	}
}

func (e *expander) instantiate(template *ast.Function, args []ast.TypeRef, at token.Position, callerSub map[string]ast.TypeRef, callerDepth int) (string, error) {
	if len(args) != len(template.TypeParams) {
		return "", typeError(at, diagnostic.TypeGenericArgumentCount, "generic function '%s' expects %d type arguments, got %d", template.Name, len(template.TypeParams), len(args))
	}
	concrete := make([]ast.TypeRef, len(args))
	for i, arg := range args {
		concrete[i] = substituteType(arg, callerSub)
		if err := e.validateTypeArgument(concrete[i], at); err != nil {
			return "", err
		}
	}
	keyParts := make([]string, len(concrete))
	for i, arg := range concrete {
		keyParts[i] = typeKey(arg)
	}
	key := template.Name + "<" + strings.Join(keyParts, ",") + ">"
	if existing, ok := e.instances[key]; ok {
		return existing.Name, nil
	}
	depth := callerDepth + 1
	if depth > limits.MaxGenericExpansionDepth {
		return "", typeError(at, diagnostic.TypeGenericInstantiationLimit, "generic instantiation expansion exceeds depth limit %d", limits.MaxGenericExpansionDepth)
	}
	if len(e.instances) >= limits.MaxGenericFunctionInstantiations {
		return "", typeError(at, diagnostic.TypeGenericInstantiationLimit, "generic function instances exceed limit %d", limits.MaxGenericFunctionInstantiations)
	}
	sub := make(map[string]ast.TypeRef, len(template.TypeParams))
	for i, name := range template.TypeParams {
		sub[name] = concrete[i]
	}
	e.nextID++
	name := fmt.Sprintf("__aster_generic_fn_%06d", e.nextID)
	instance := &ast.Function{At: template.At, Name: name}
	// Register before walking the body so same-instantiation recursion hits the cache.
	e.instances[key] = instance
	e.ordered = append(e.ordered, instance)
	instance.Params = make([]ast.Param, len(template.Params))
	for i, param := range template.Params {
		instance.Params[i] = ast.Param{At: param.At, Name: param.Name, Type: substituteType(param.Type, sub)}
	}
	if template.ReturnType != nil {
		ret := substituteType(*template.ReturnType, sub)
		instance.ReturnType = &ret
	}
	body, err := e.rewriteBlock(template.Body, sub, depth)
	if err != nil {
		return "", err
	}
	instance.Body = body
	return instance.Name, nil
}

func (e *expander) validateTypeArgument(t ast.TypeRef, at token.Position) error {
	if t.Name == "Array" {
		if t.Element == nil {
			return typeError(at, diagnostic.TypeInvalidTypeAnnotation, "generic argument 'Array' requires an element type")
		}
		if t.Element.Name == "unit" && t.Element.Element == nil {
			return typeError(at, diagnostic.TypeInvalidTypeAnnotation, "Array cannot contain unit values")
		}
		return e.validateTypeArgument(*t.Element, at)
	}
	if t.Element != nil {
		return typeError(at, diagnostic.TypeGenericArgumentsOnNonGeneric, "generic type arguments for '%s' are not supported in this implementation stage", t.Name)
	}
	if !e.knownTypes[t.Name] {
		return typeError(at, diagnostic.TypeInvalidTypeAnnotation, "unknown generic type argument '%s'", t.Name)
	}
	return nil
}

func typeKey(t ast.TypeRef) string {
	if t.Element != nil {
		return t.Name + "<" + typeKey(*t.Element) + ">"
	}
	return t.Name
}

func substituteType(t ast.TypeRef, sub map[string]ast.TypeRef) ast.TypeRef {
	if t.Element == nil {
		if replacement, ok := sub[t.Name]; ok {
			return cloneTypeRef(replacement)
		}
		return t
	}
	out := t
	element := substituteType(*t.Element, sub)
	out.Element = &element
	return out
}

func cloneTypeRef(t ast.TypeRef) ast.TypeRef {
	out := t
	if t.Element != nil {
		el := cloneTypeRef(*t.Element)
		out.Element = &el
	}
	return out
}

func typeError(pos token.Position, code, format string, args ...any) error {
	return diagnostic.New(code, "type", pos, fmt.Sprintf(format, args...))
}

func (e *expander) rewriteFunction(fn *ast.Function, sub map[string]ast.TypeRef, depth int) (*ast.Function, error) {
	out := &ast.Function{At: fn.At, Name: fn.Name}
	out.Params = make([]ast.Param, len(fn.Params))
	for i, p := range fn.Params {
		out.Params[i] = ast.Param{At: p.At, Name: p.Name, Type: substituteType(p.Type, sub)}
	}
	if fn.ReturnType != nil {
		ret := substituteType(*fn.ReturnType, sub)
		out.ReturnType = &ret
	}
	body, err := e.rewriteBlock(fn.Body, sub, depth)
	if err != nil {
		return nil, err
	}
	out.Body = body
	return out, nil
}

func (e *expander) rewriteBlock(block *ast.BlockStmt, sub map[string]ast.TypeRef, depth int) (*ast.BlockStmt, error) {
	out := &ast.BlockStmt{At: block.At, Statements: make([]ast.Stmt, len(block.Statements))}
	for i, stmt := range block.Statements {
		copy, err := e.rewriteStmt(stmt, sub, depth)
		if err != nil {
			return nil, err
		}
		out.Statements[i] = copy
	}
	return out, nil
}

func (e *expander) rewriteStmt(stmt ast.Stmt, sub map[string]ast.TypeRef, depth int) (ast.Stmt, error) {
	switch n := stmt.(type) {
	case *ast.BlockStmt:
		return e.rewriteBlock(n, sub, depth)
	case *ast.LetStmt:
		out := &ast.LetStmt{At: n.At, Name: n.Name, Mutable: n.Mutable}
		if n.Annotation != nil {
			v := substituteType(*n.Annotation, sub)
			out.Annotation = &v
		}
		value, err := e.rewriteExpr(n.Value, sub, depth)
		if err != nil {
			return nil, err
		}
		out.Value = value
		return out, nil
	case *ast.AssignStmt:
		value, err := e.rewriteExpr(n.Value, sub, depth)
		if err != nil {
			return nil, err
		}
		return &ast.AssignStmt{At: n.At, Name: n.Name, Value: value}, nil
	case *ast.IndexAssignStmt:
		target, err := e.rewriteExpr(n.Target, sub, depth)
		if err != nil {
			return nil, err
		}
		value, err := e.rewriteExpr(n.Value, sub, depth)
		if err != nil {
			return nil, err
		}
		return &ast.IndexAssignStmt{At: n.At, Target: target, Value: value}, nil
	case *ast.ExprStmt:
		expr, err := e.rewriteExpr(n.Expr, sub, depth)
		if err != nil {
			return nil, err
		}
		return &ast.ExprStmt{At: n.At, Expr: expr}, nil
	case *ast.IfStmt:
		cond, err := e.rewriteExpr(n.Condition, sub, depth)
		if err != nil {
			return nil, err
		}
		then, err := e.rewriteBlock(n.Then, sub, depth)
		if err != nil {
			return nil, err
		}
		var otherwise ast.Stmt
		if n.Else != nil {
			otherwise, err = e.rewriteStmt(n.Else, sub, depth)
			if err != nil {
				return nil, err
			}
		}
		return &ast.IfStmt{At: n.At, Condition: cond, Then: then, Else: otherwise}, nil
	case *ast.WhileStmt:
		cond, err := e.rewriteExpr(n.Condition, sub, depth)
		if err != nil {
			return nil, err
		}
		body, err := e.rewriteBlock(n.Body, sub, depth)
		if err != nil {
			return nil, err
		}
		return &ast.WhileStmt{At: n.At, Condition: cond, Body: body}, nil
	case *ast.ReturnStmt:
		out := &ast.ReturnStmt{At: n.At}
		if n.Value != nil {
			value, err := e.rewriteExpr(n.Value, sub, depth)
			if err != nil {
				return nil, err
			}
			out.Value = value
		}
		return out, nil
	default:
		return nil, typeError(stmt.Pos(), diagnostic.TypeUnsupportedStatement, "generic expansion does not support statement %T", stmt)
	}
}

func (e *expander) rewriteExpr(expr ast.Expr, sub map[string]ast.TypeRef, depth int) (ast.Expr, error) {
	switch n := expr.(type) {
	case *ast.IdentifierExpr:
		return &ast.IdentifierExpr{At: n.At, Name: n.Name}, nil
	case *ast.LiteralExpr:
		return &ast.LiteralExpr{At: n.At, Kind: n.Kind, Text: n.Text, Int: n.Int, Float: n.Float, String: n.String, Bool: n.Bool}, nil
	case *ast.FieldExpr:
		object, err := e.rewriteExpr(n.Object, sub, depth)
		if err != nil {
			return nil, err
		}
		return &ast.FieldExpr{At: n.At, Object: object, Name: n.Name}, nil
	case *ast.UnaryExpr:
		right, err := e.rewriteExpr(n.Right, sub, depth)
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{At: n.At, Op: n.Op, Right: right}, nil
	case *ast.BinaryExpr:
		left, err := e.rewriteExpr(n.Left, sub, depth)
		if err != nil {
			return nil, err
		}
		right, err := e.rewriteExpr(n.Right, sub, depth)
		if err != nil {
			return nil, err
		}
		return &ast.BinaryExpr{At: n.At, Left: left, Op: n.Op, Right: right}, nil
	case *ast.CallExpr:
		args := make([]ast.Expr, len(n.Args))
		for i, arg := range n.Args {
			value, err := e.rewriteExpr(arg, sub, depth)
			if err != nil {
				return nil, err
			}
			args[i] = value
		}
		id, named := n.Callee.(*ast.IdentifierExpr)
		if len(n.TypeArgs) > 0 {
			if !named {
				return nil, typeError(n.At, diagnostic.TypeGenericArgumentsOnNonGeneric, "explicit generic arguments require a named function target")
			}
			template, ok := e.templates[id.Name]
			if !ok {
				return nil, typeError(n.At, diagnostic.TypeGenericArgumentsOnNonGeneric, "function '%s' does not accept explicit type arguments", id.Name)
			}
			instanceName, err := e.instantiate(template, n.TypeArgs, n.At, sub, depth)
			if err != nil {
				return nil, err
			}
			return &ast.CallExpr{At: n.At, Callee: &ast.IdentifierExpr{At: id.At, Name: instanceName}, Args: args}, nil
		}
		if named {
			if _, generic := e.templates[id.Name]; generic {
				return nil, typeError(n.At, diagnostic.TypeGenericInference, "generic function '%s' requires explicit type arguments in this implementation stage (use %s::<...>(...))", id.Name, id.Name)
			}
		}
		callee, err := e.rewriteExpr(n.Callee, sub, depth)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{At: n.At, Callee: callee, Args: args}, nil
	case *ast.EnumVariantExpr:
		args := make([]ast.Expr, len(n.Args))
		for i, arg := range n.Args {
			value, err := e.rewriteExpr(arg, sub, depth)
			if err != nil {
				return nil, err
			}
			args[i] = value
		}
		return &ast.EnumVariantExpr{At: n.At, EnumName: n.EnumName, VariantName: n.VariantName, Args: args, HasPayload: n.HasPayload}, nil
	case *ast.MatchExpr:
		subject, err := e.rewriteExpr(n.Subject, sub, depth)
		if err != nil {
			return nil, err
		}
		arms := make([]ast.MatchArm, len(n.Arms))
		for i, arm := range n.Arms {
			value, err := e.rewriteExpr(arm.Expr, sub, depth)
			if err != nil {
				return nil, err
			}
			pattern := arm.Pattern
			pattern.Bindings = append([]string(nil), arm.Pattern.Bindings...)
			arms[i] = ast.MatchArm{At: arm.At, Pattern: pattern, Expr: value}
		}
		return &ast.MatchExpr{At: n.At, Subject: subject, Arms: arms}, nil
	case *ast.ArrayExpr:
		elements := make([]ast.Expr, len(n.Elements))
		for i, item := range n.Elements {
			value, err := e.rewriteExpr(item, sub, depth)
			if err != nil {
				return nil, err
			}
			elements[i] = value
		}
		return &ast.ArrayExpr{At: n.At, Elements: elements}, nil
	case *ast.IndexExpr:
		collection, err := e.rewriteExpr(n.Collection, sub, depth)
		if err != nil {
			return nil, err
		}
		index, err := e.rewriteExpr(n.Index, sub, depth)
		if err != nil {
			return nil, err
		}
		return &ast.IndexExpr{At: n.At, Collection: collection, Index: index}, nil
	case *ast.SliceExpr:
		collection, err := e.rewriteExpr(n.Collection, sub, depth)
		if err != nil {
			return nil, err
		}
		start, err := e.rewriteExpr(n.Start, sub, depth)
		if err != nil {
			return nil, err
		}
		end, err := e.rewriteExpr(n.End, sub, depth)
		if err != nil {
			return nil, err
		}
		return &ast.SliceExpr{At: n.At, Collection: collection, Start: start, End: end}, nil
	default:
		return nil, typeError(expr.Pos(), diagnostic.TypeUnsupportedExpression, "generic expansion does not support expression %T", expr)
	}
}
