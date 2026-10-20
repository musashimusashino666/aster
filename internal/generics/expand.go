// Package generics monomorphizes generic functions, structs, and enums to concrete declarations.
package generics

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aster-lang/aster/internal/ast"
	"github.com/aster-lang/aster/internal/diagnostic"
	"github.com/aster-lang/aster/internal/langtypes"
	"github.com/aster-lang/aster/internal/limits"
	"github.com/aster-lang/aster/internal/token"
)

type structInstanceInfo struct {
	template string
	args     []ast.TypeRef
}

type expander struct {
	templates          map[string]*ast.Function
	functions          map[string]*ast.Function
	structTemplates    map[string]*ast.StructDecl
	structInstances    map[string]*ast.StructDecl
	structInstanceInfo map[string]structInstanceInfo
	enumTemplates      map[string]*ast.EnumDecl
	enumInstances      map[string]*ast.EnumDecl
	enumInstanceInfo   map[string]structInstanceInfo
	enumDecls          map[string]*ast.EnumDecl
	knownTypes         map[string]bool
	structFields       map[string]map[string]ast.TypeRef
	instances          map[string]*ast.Function
	ordered            []*ast.Function
	orderedStructs     []*ast.StructDecl
	orderedEnums       []*ast.EnumDecl
	variables          map[string]ast.TypeRef
	expectedReturn     *ast.TypeRef
	nextID             int
	nextStructID       int
	nextEnumID         int
}

func (e *expander) totalMonomorphizedInstances() int {
	return len(e.instances) + len(e.structInstances) + len(e.enumInstances)
}

func (e *expander) checkTotalMonomorphizedLimit(at token.Position, code, kind string) error {
	if e.totalMonomorphizedInstances() >= limits.MaxGenericMonomorphizedDeclarations {
		return typeError(at, code, "total generic instances exceed limit %d while instantiating generic %s",
			limits.MaxGenericMonomorphizedDeclarations, kind)
	}
	return nil
}

// Expand specializes explicitly instantiated generic functions before type
// checking. The returned AST contains only concrete functions; all later
// compiler stages remain unaware of generic templates.
func Expand(program *ast.Program) (*ast.Program, error) {
	if program == nil {
		return nil, fmt.Errorf("generics: nil program")
	}
	e := &expander{
		templates:          map[string]*ast.Function{},
		functions:          map[string]*ast.Function{},
		structTemplates:    map[string]*ast.StructDecl{},
		structInstances:    map[string]*ast.StructDecl{},
		structInstanceInfo: map[string]structInstanceInfo{},
		enumTemplates:      map[string]*ast.EnumDecl{},
		enumInstances:      map[string]*ast.EnumDecl{},
		enumInstanceInfo:   map[string]structInstanceInfo{},
		enumDecls:          map[string]*ast.EnumDecl{},
		structFields:       map[string]map[string]ast.TypeRef{},
		knownTypes: map[string]bool{
			"i64": true, "f64": true, "bool": true, "string": true, "unit": true,
		},
		instances: map[string]*ast.Function{},
		variables: map[string]ast.TypeRef{},
	}
	functionNames := map[string]*ast.Function{}
	declaredFunctionNames := map[string]bool{}
	structNames := map[string]*ast.StructDecl{}
	enumNames := map[string]*ast.EnumDecl{}
	for _, decl := range program.Decls {
		if fn, ok := decl.(*ast.Function); ok {
			declaredFunctionNames[fn.Name] = true
		}
	}
	for _, decl := range program.Decls {
		switch n := decl.(type) {
		case *ast.StructDecl:
			if prior, exists := structNames[n.Name]; exists {
				return nil, typeError(n.Pos(), diagnostic.TypeDuplicateStruct, "struct '%s' is already declared", prior.Name)
			}
			structNames[n.Name] = n
			e.knownTypes[n.Name] = true
		case *ast.EnumDecl:
			if prior, exists := enumNames[n.Name]; exists {
				return nil, typeError(n.Pos(), diagnostic.TypeDuplicateEnum, "enum '%s' is already declared", prior.Name)
			}
			enumNames[n.Name] = n
			e.knownTypes[n.Name] = true
		}
	}
	for _, decl := range program.Decls {
		switch n := decl.(type) {
		case *ast.Function:
			if prior, exists := functionNames[n.Name]; exists {
				return nil, typeError(n.Pos(), diagnostic.TypeDuplicateFunction, "function '%s' is already declared", prior.Name)
			}
			functionNames[n.Name] = n
			e.functions[n.Name] = n
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
			if _, conflictsWithEnum := enumNames[n.Name]; conflictsWithEnum {
				return nil, typeError(n.Pos(), diagnostic.TypeDuplicateStruct, "struct name '%s' conflicts with an enum", n.Name)
			}
			if declaredFunctionNames[n.Name] {
				return nil, typeError(n.Pos(), diagnostic.TypeDuplicateStruct, "struct name '%s' conflicts with a function", n.Name)
			}
			if !langtypes.ValidName(n.Name) {
				return nil, typeError(n.Pos(), diagnostic.TypeInvalidStructName, "invalid struct name '%s'", n.Name)
			}
			if reservedTypeName(n.Name) {
				return nil, typeError(n.Pos(), diagnostic.TypeDuplicateStruct, "struct name '%s' is reserved", n.Name)
			}
			if len(n.TypeParams) > 0 {
				if len(n.TypeParams) > limits.MaxGenericTypeParameters {
					return nil, typeError(n.Pos(), diagnostic.TypeGenericInstantiationLimit, "generic struct '%s' declares %d type parameters; maximum is %d", n.Name, len(n.TypeParams), limits.MaxGenericTypeParameters)
				}
				seen := map[string]bool{}
				for _, name := range n.TypeParams {
					if !langtypes.ValidName(name) || reservedTypeName(name) || strings.HasPrefix(name, "__") || seen[name] {
						return nil, typeError(n.Pos(), diagnostic.TypeInvalidGenericParameter, "invalid, reserved, or duplicate type parameter '%s' in struct '%s'", name, n.Name)
					}
					seen[name] = true
				}
				e.structTemplates[n.Name] = n
				continue
			}
			fields := make(map[string]ast.TypeRef, len(n.Fields))
			for _, field := range n.Fields {
				fields[field.Name] = field.Type
			}
			e.structFields[n.Name] = fields
		case *ast.EnumDecl:
			if _, conflictsWithStruct := structNames[n.Name]; conflictsWithStruct {
				return nil, typeError(n.Pos(), diagnostic.TypeDuplicateEnum, "enum name '%s' conflicts with a struct", n.Name)
			}
			if declaredFunctionNames[n.Name] {
				return nil, typeError(n.Pos(), diagnostic.TypeDuplicateEnum, "enum name '%s' conflicts with a function", n.Name)
			}
			if !langtypes.ValidName(n.Name) || reservedTypeName(n.Name) {
				return nil, typeError(n.Pos(), diagnostic.TypeDuplicateEnum, "invalid or reserved enum name '%s'", n.Name)
			}
			e.knownTypes[n.Name] = true
			if len(n.TypeParams) > 0 {
				if len(n.TypeParams) > limits.MaxGenericTypeParameters {
					return nil, typeError(n.Pos(), diagnostic.TypeGenericInstantiationLimit, "generic enum '%s' declares %d type parameters; maximum is %d", n.Name, len(n.TypeParams), limits.MaxGenericTypeParameters)
				}
				seen := map[string]bool{}
				for _, name := range n.TypeParams {
					if !langtypes.ValidName(name) || reservedTypeName(name) || strings.HasPrefix(name, "__") || seen[name] {
						return nil, typeError(n.Pos(), diagnostic.TypeInvalidGenericParameter, "invalid, reserved, or duplicate type parameter '%s' in enum '%s'", name, n.Name)
					}
					seen[name] = true
				}
				variantNames := map[string]bool{}
				for _, variant := range n.Variants {
					if !langtypes.ValidName(variant.Name) || variantNames[variant.Name] {
						return nil, typeError(variant.At, diagnostic.TypeDuplicateEnumVariant, "invalid or duplicate variant '%s' in enum '%s'", variant.Name, n.Name)
					}
					variantNames[variant.Name] = true
				}
				e.enumTemplates[n.Name] = n
			}
			e.enumDecls[n.Name] = n
		}
	}

	// Preserve AST node identity for programs that do not use generic templates.
	// Existing TypedProgram.TypeOf callers may retain expression pointers from the
	// parsed program, so avoid cloning the AST when expansion is unnecessary.
	if len(e.templates) == 0 && len(e.structTemplates) == 0 && len(e.enumTemplates) == 0 {
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
		case *ast.StructDecl:
			if len(n.TypeParams) > 0 {
				continue // generic templates are emitted only as concrete instances
			}
			copy, err := e.rewriteStruct(n, nil, 0)
			if err != nil {
				return nil, err
			}
			out.Decls = append(out.Decls, copy)
		case *ast.EnumDecl:
			if len(n.TypeParams) > 0 {
				continue // generic templates are emitted only as concrete instances
			}
			copy, err := e.rewriteEnum(n, nil, 0)
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
	out.Decls = append(out.Decls, structDecls(e.orderedStructs)...)
	for _, enumDecl := range e.orderedEnums {
		out.Decls = append(out.Decls, enumDecl)
	}
	out.Decls = append(out.Decls, funcDecls(e.ordered)...)
	return out, nil
}

func structDecls(in []*ast.StructDecl) []ast.Decl {
	out := make([]ast.Decl, len(in))
	for i, decl := range in {
		out[i] = decl
	}
	return out
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
		resolved, err := e.resolveType(substituteType(arg, callerSub), at, callerDepth+1)
		if err != nil {
			return "", err
		}
		concrete[i] = resolved
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
	if err := e.checkTotalMonomorphizedLimit(at, diagnostic.TypeGenericInstantiationLimit, "function"); err != nil {
		return "", err
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
	e.functions[name] = instance
	e.ordered = append(e.ordered, instance)
	instance.Params = make([]ast.Param, len(template.Params))
	for i, param := range template.Params {
		typ, err := e.resolveType(substituteType(param.Type, sub), param.At, depth)
		if err != nil {
			return "", err
		}
		instance.Params[i] = ast.Param{At: param.At, Name: param.Name, Type: typ}
	}
	if template.ReturnType != nil {
		ret, err := e.resolveType(substituteType(*template.ReturnType, sub), template.ReturnType.At, depth)
		if err != nil {
			return "", err
		}
		instance.ReturnType = &ret
	}
	previousVariables := e.variables
	e.variables = make(map[string]ast.TypeRef, len(instance.Params))
	for _, param := range instance.Params {
		e.variables[param.Name] = cloneTypeRef(param.Type)
	}
	previousExpectedReturn := e.expectedReturn
	e.expectedReturn = nil
	if instance.ReturnType != nil {
		expected := cloneTypeRef(*instance.ReturnType)
		e.expectedReturn = &expected
	}
	body, err := e.rewriteBlock(template.Body, sub, depth)
	e.expectedReturn = previousExpectedReturn
	e.variables = previousVariables
	if err != nil {
		return "", err
	}
	instance.Body = body
	return instance.Name, nil
}

func (e *expander) instantiateStruct(template *ast.StructDecl, args []ast.TypeRef, at token.Position, callerSub map[string]ast.TypeRef, callerDepth int) (string, error) {
	if len(args) != len(template.TypeParams) {
		return "", typeError(at, diagnostic.TypeGenericStructArgumentCount, "generic struct '%s' expects %d type arguments, got %d", template.Name, len(template.TypeParams), len(args))
	}
	concrete := make([]ast.TypeRef, len(args))
	for i, arg := range args {
		resolved, err := e.resolveType(substituteType(arg, callerSub), at, callerDepth+1)
		if err != nil {
			return "", err
		}
		concrete[i] = resolved
	}
	parts := make([]string, len(concrete))
	for i, arg := range concrete {
		parts[i] = typeKey(arg)
	}
	key := template.Name + "<" + strings.Join(parts, ",") + ">"
	if existing, ok := e.structInstances[key]; ok {
		return existing.Name, nil
	}
	depth := callerDepth + 1
	if depth > limits.MaxGenericExpansionDepth {
		return "", typeError(at, diagnostic.TypeGenericStructInstantiationLimit, "generic struct expansion exceeds depth limit %d", limits.MaxGenericExpansionDepth)
	}
	if len(e.structInstances) >= limits.MaxGenericStructInstantiations {
		return "", typeError(at, diagnostic.TypeGenericStructInstantiationLimit, "generic struct instances exceed limit %d", limits.MaxGenericStructInstantiations)
	}
	if err := e.checkTotalMonomorphizedLimit(at, diagnostic.TypeGenericStructInstantiationLimit, "struct"); err != nil {
		return "", err
	}
	sub := make(map[string]ast.TypeRef, len(template.TypeParams))
	for i, name := range template.TypeParams {
		sub[name] = concrete[i]
	}
	e.nextStructID++
	name := fmt.Sprintf("__aster_generic_struct_%06d", e.nextStructID)
	instance := &ast.StructDecl{At: template.At, Name: name}
	// Cache the placeholder before resolving its fields so recursion through
	// Array<T> and exact self references reuse this nominal identity.
	e.structInstances[key] = instance
	e.structInstanceInfo[name] = structInstanceInfo{template: template.Name, args: cloneTypeArgs(concrete)}
	e.knownTypes[name] = true
	e.orderedStructs = append(e.orderedStructs, instance)
	fields := make(map[string]ast.TypeRef, len(template.Fields))
	instance.Fields = make([]ast.StructField, len(template.Fields))
	for i, field := range template.Fields {
		typ, err := e.resolveType(substituteType(field.Type, sub), field.At, depth)
		if err != nil {
			return "", err
		}
		instance.Fields[i] = ast.StructField{At: field.At, Name: field.Name, Type: typ}
		fields[field.Name] = cloneTypeRef(typ)
	}
	e.structFields[name] = fields
	return name, nil
}

func (e *expander) instantiateEnum(template *ast.EnumDecl, args []ast.TypeRef, at token.Position, callerSub map[string]ast.TypeRef, callerDepth int) (string, error) {
	if len(args) != len(template.TypeParams) {
		return "", typeError(at, diagnostic.TypeGenericEnumArgumentCount, "generic enum '%s' expects %d type arguments, got %d", template.Name, len(template.TypeParams), len(args))
	}
	concrete := make([]ast.TypeRef, len(args))
	for i, arg := range args {
		resolved, err := e.resolveType(substituteType(arg, callerSub), at, callerDepth+1)
		if err != nil {
			return "", err
		}
		concrete[i] = resolved
	}
	parts := make([]string, len(concrete))
	for i, arg := range concrete {
		parts[i] = typeKey(arg)
	}
	key := template.Name + "<" + strings.Join(parts, ",") + ">"
	if existing, ok := e.enumInstances[key]; ok {
		return existing.Name, nil
	}
	depth := callerDepth + 1
	if depth > limits.MaxGenericExpansionDepth {
		return "", typeError(at, diagnostic.TypeGenericEnumInstantiationLimit, "generic enum expansion exceeds depth limit %d", limits.MaxGenericExpansionDepth)
	}
	if len(e.enumInstances) >= limits.MaxGenericEnumInstantiations {
		return "", typeError(at, diagnostic.TypeGenericEnumInstantiationLimit, "generic enum instances exceed limit %d", limits.MaxGenericEnumInstantiations)
	}
	if err := e.checkTotalMonomorphizedLimit(at, diagnostic.TypeGenericEnumInstantiationLimit, "enum"); err != nil {
		return "", err
	}
	sub := make(map[string]ast.TypeRef, len(template.TypeParams))
	for i, param := range template.TypeParams {
		sub[param] = concrete[i]
	}
	e.nextEnumID++
	name := fmt.Sprintf("__aster_generic_enum_%06d", e.nextEnumID)
	instance := &ast.EnumDecl{At: template.At, Name: name}
	// Register before resolving payloads so identical recursive applications
	// reuse this nominal identity instead of recursively expanding forever.
	e.enumInstances[key] = instance
	e.enumInstanceInfo[name] = structInstanceInfo{template: template.Name, args: cloneTypeArgs(concrete)}
	e.enumDecls[name] = instance
	e.knownTypes[name] = true
	e.orderedEnums = append(e.orderedEnums, instance)
	instance.Variants = make([]ast.EnumVariant, len(template.Variants))
	for i, variant := range template.Variants {
		out := ast.EnumVariant{At: variant.At, Name: variant.Name, Payload: make([]ast.TypeRef, len(variant.Payload))}
		for j, payload := range variant.Payload {
			typ, err := e.resolveType(substituteType(payload, sub), payload.At, depth)
			if err != nil {
				return "", err
			}
			out.Payload[j] = typ
		}
		instance.Variants[i] = out
	}
	return name, nil
}

func (e *expander) inferEnumTypeArguments(template *ast.EnumDecl, variantName string, args []ast.Expr, at token.Position) ([]ast.TypeRef, error) {
	return e.inferEnumTypeArgumentsWithContext(template, variantName, args, at, nil, 0)
}

// inferEnumTypeArgumentsWithContext allows one payload to establish type parameters
// that disambiguate another payload, such as Pair::Item(Option::None, 7).
// Payloads that cannot yet be typed are retried after other payloads have supplied
// enough information to construct their expected concrete types.
func (e *expander) inferEnumTypeArgumentsWithContext(template *ast.EnumDecl, variantName string, args []ast.Expr, at token.Position, callerSub map[string]ast.TypeRef, depth int) ([]ast.TypeRef, error) {
	var variant *ast.EnumVariant
	for i := range template.Variants {
		if template.Variants[i].Name == variantName {
			variant = &template.Variants[i]
			break
		}
	}
	if variant == nil {
		return nil, typeError(at, diagnostic.TypeUnknownEnumVariant, "enum '%s' has no variant '%s'", template.Name, variantName)
	}
	if len(args) != len(variant.Payload) {
		return nil, typeError(at, diagnostic.TypeEnumConstructorArity, "variant %s::%s requires %d payload arguments", template.Name, variantName, len(variant.Payload))
	}
	parameters := make(map[string]bool, len(template.TypeParams))
	for _, param := range template.TypeParams {
		parameters[param] = true
	}
	inferred := make(map[string]ast.TypeRef, len(template.TypeParams))
	pending := make([]int, 0, len(args))
	for i, pattern := range variant.Payload {
		if !containsParameter(pattern, parameters) {
			continue
		}
		actual, ok := e.inferExprType(args[i])
		if !ok {
			// Normalize nested calls so bad arity and unknown variants retain
			// their own stable diagnostics instead of being masked as TYP088.
			normalized, normalizeErr := e.rewriteExprExpected(args[i], callerSub, depth+1, nil)
			if normalizeErr != nil {
				if !isDeferredTypeInferenceDiagnostic(normalizeErr) {
					return nil, normalizeErr
				}
			} else {
				actual, ok = e.inferExprType(normalized)
			}
		}
		if !ok {
			pending = append(pending, i)
			continue
		}
		if err := e.unifyType(pattern, actual, parameters, inferred); err != nil {
			return nil, typeError(args[i].Pos(), diagnostic.TypeGenericEnumInference, "cannot infer type arguments for '%s' consistently: %v", template.Name, err)
		}
	}

	for len(pending) > 0 {
		nextPending := make([]int, 0, len(pending))
		progress := false
		for _, i := range pending {
			pattern := variant.Payload[i]
			if !typePatternResolved(pattern, parameters, inferred) {
				nextPending = append(nextPending, i)
				continue
			}
			expected, err := e.resolveType(substituteType(pattern, inferred), args[i].Pos(), depth+1)
			if err != nil {
				return nil, err
			}
			rewritten, err := e.rewriteExprExpected(args[i], callerSub, depth+1, &expected)
			if err != nil {
				return nil, err
			}
			actual, ok := e.inferExprType(rewritten)
			if !ok {
				nextPending = append(nextPending, i)
				continue
			}
			if err := e.unifyType(pattern, actual, parameters, inferred); err != nil {
				return nil, typeError(args[i].Pos(), diagnostic.TypeGenericEnumInference, "cannot infer type arguments for '%s' consistently: %v", template.Name, err)
			}
			progress = true
		}
		if !progress {
			break
		}
		pending = nextPending
	}

	out := make([]ast.TypeRef, len(template.TypeParams))
	for i, param := range template.TypeParams {
		typ, ok := inferred[param]
		if !ok {
			return nil, typeError(at, diagnostic.TypeGenericEnumInference, "cannot infer type parameter '%s' for enum '%s'; provide an expected concrete enum type or use explicit type arguments such as %s::<...>::%s", param, template.Name, template.Name, variantName)
		}
		out[i] = typ
	}
	return out, nil
}

func isDeferredTypeInferenceDiagnostic(err error) bool {
	var diagnosticError *diagnostic.Diagnostic
	if !errors.As(err, &diagnosticError) {
		return false
	}
	return (diagnosticError.Code == diagnostic.TypeGenericEnumInference ||
		diagnosticError.Code == diagnostic.TypeGenericStructInference) &&
		!strings.Contains(diagnosticError.Message, "consistently")
}

func typePatternResolved(pattern ast.TypeRef, parameters map[string]bool, inferred map[string]ast.TypeRef) bool {
	if pattern.Element != nil {
		return typePatternResolved(*pattern.Element, parameters, inferred)
	}
	if len(pattern.Args) == 0 && parameters[pattern.Name] {
		_, ok := inferred[pattern.Name]
		return ok
	}
	for _, arg := range pattern.Args {
		if !typePatternResolved(arg, parameters, inferred) {
			return false
		}
	}
	return true
}

func (e *expander) rewriteEnum(template *ast.EnumDecl, sub map[string]ast.TypeRef, depth int) (*ast.EnumDecl, error) {
	out := &ast.EnumDecl{At: template.At, Name: template.Name, Variants: make([]ast.EnumVariant, len(template.Variants))}
	// Keep the declaration visible while resolving its own payload types.
	e.enumDecls[out.Name] = out
	for i, variant := range template.Variants {
		copy := ast.EnumVariant{At: variant.At, Name: variant.Name, Payload: make([]ast.TypeRef, len(variant.Payload))}
		for j, payload := range variant.Payload {
			typ, err := e.resolveType(substituteType(payload, sub), payload.At, depth)
			if err != nil {
				return nil, err
			}
			copy.Payload[j] = typ
		}
		out.Variants[i] = copy
	}
	return out, nil
}

func cloneTypeArgs(args []ast.TypeRef) []ast.TypeRef {
	out := make([]ast.TypeRef, len(args))
	for i, arg := range args {
		out[i] = cloneTypeRef(arg)
	}
	return out
}

func (e *expander) inferStructTypeArguments(template *ast.StructDecl, args []ast.Expr, at token.Position) ([]ast.TypeRef, error) {
	if len(args) != len(template.Fields) {
		return nil, typeError(at, diagnostic.TypeStructConstructorArity, "struct '%s' expects %d fields, got %d", template.Name, len(template.Fields), len(args))
	}
	parameters := make(map[string]bool, len(template.TypeParams))
	for _, name := range template.TypeParams {
		parameters[name] = true
	}
	inferred := make(map[string]ast.TypeRef, len(template.TypeParams))
	for i, field := range template.Fields {
		if !containsParameter(field.Type, parameters) {
			continue
		}
		actual, ok := e.inferExprType(args[i])
		if !ok {
			return nil, typeError(args[i].Pos(), diagnostic.TypeGenericStructInference, "cannot infer type arguments for field '%s' of '%s'; use %s::<...>(...)", field.Name, template.Name, template.Name)
		}
		if err := e.unifyType(field.Type, actual, parameters, inferred); err != nil {
			return nil, typeError(args[i].Pos(), diagnostic.TypeGenericStructInference, "cannot infer type arguments for '%s' consistently: %v", template.Name, err)
		}
	}
	out := make([]ast.TypeRef, len(template.TypeParams))
	for i, name := range template.TypeParams {
		typ, ok := inferred[name]
		if !ok {
			return nil, typeError(at, diagnostic.TypeGenericStructInference, "cannot infer type parameter '%s' for struct '%s'; use %s::<...>(...)", name, template.Name, template.Name)
		}
		out[i] = typ
	}
	return out, nil
}

// resolveType lowers type applications into concrete nominal names. Array keeps
// its existing Element representation; named generic structs use Args.
func (e *expander) resolveType(typ ast.TypeRef, at token.Position, depth int) (ast.TypeRef, error) {
	if typ.Name == "Array" {
		if typ.Element == nil || len(typ.Args) != 0 {
			return ast.TypeRef{}, typeError(at, diagnostic.TypeInvalidTypeAnnotation, "Array requires exactly one element type")
		}
		element, err := e.resolveType(*typ.Element, at, depth+1)
		if err != nil {
			return ast.TypeRef{}, err
		}
		if element.Name == "unit" && element.Element == nil && len(element.Args) == 0 {
			return ast.TypeRef{}, typeError(at, diagnostic.TypeInvalidTypeAnnotation, "Array cannot contain unit values")
		}
		typ.Element = &element
		return typ, nil
	}
	if typ.Element != nil {
		return ast.TypeRef{}, typeError(at, diagnostic.TypeGenericArgumentsOnNonGeneric, "type '%s' does not accept an Array-style type argument", typ.Name)
	}
	if template, isGeneric := e.enumTemplates[typ.Name]; isGeneric {
		if len(typ.Args) == 0 {
			return ast.TypeRef{}, typeError(at, diagnostic.TypeGenericEnumArgumentCount, "generic enum '%s' requires %d type arguments", typ.Name, len(template.TypeParams))
		}
		args := make([]ast.TypeRef, len(typ.Args))
		for i, arg := range typ.Args {
			resolved, err := e.resolveType(arg, at, depth+1)
			if err != nil {
				return ast.TypeRef{}, err
			}
			args[i] = resolved
		}
		name, err := e.instantiateEnum(template, args, at, nil, depth)
		if err != nil {
			return ast.TypeRef{}, err
		}
		return ast.TypeRef{Name: name, At: typ.At}, nil
	}
	if template, isGeneric := e.structTemplates[typ.Name]; isGeneric {
		if len(typ.Args) == 0 {
			return ast.TypeRef{}, typeError(at, diagnostic.TypeGenericStructArgumentCount, "generic struct '%s' requires %d type arguments", typ.Name, len(template.TypeParams))
		}
		args := make([]ast.TypeRef, len(typ.Args))
		for i, arg := range typ.Args {
			resolved, err := e.resolveType(arg, at, depth+1)
			if err != nil {
				return ast.TypeRef{}, err
			}
			args[i] = resolved
		}
		name, err := e.instantiateStruct(template, args, at, nil, depth)
		if err != nil {
			return ast.TypeRef{}, err
		}
		return ast.TypeRef{Name: name, At: typ.At}, nil
	}
	if len(typ.Args) != 0 {
		return ast.TypeRef{}, typeError(at, diagnostic.TypeGenericArgumentsOnNonGeneric, "type '%s' does not accept generic type arguments", typ.Name)
	}
	if !e.knownTypes[typ.Name] {
		return ast.TypeRef{}, typeError(at, diagnostic.TypeInvalidTypeAnnotation, "unknown type '%s'", typ.Name)
	}
	return typ, nil
}

func (e *expander) rewriteStruct(template *ast.StructDecl, sub map[string]ast.TypeRef, depth int) (*ast.StructDecl, error) {
	out := &ast.StructDecl{At: template.At, Name: template.Name}
	out.Fields = make([]ast.StructField, len(template.Fields))
	for i, field := range template.Fields {
		typ, err := e.resolveType(substituteType(field.Type, sub), field.At, depth)
		if err != nil {
			return nil, err
		}
		out.Fields[i] = ast.StructField{At: field.At, Name: field.Name, Type: typ}
	}
	fields := make(map[string]ast.TypeRef, len(out.Fields))
	for _, field := range out.Fields {
		fields[field.Name] = cloneTypeRef(field.Type)
	}
	e.structFields[out.Name] = fields
	return out, nil
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
	if t.Element != nil || len(t.Args) > 0 {
		return typeError(at, diagnostic.TypeGenericArgumentsOnNonGeneric, "generic type arguments for '%s' were not resolved", t.Name)
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
	if len(t.Args) > 0 {
		parts := make([]string, len(t.Args))
		for i, arg := range t.Args {
			parts[i] = typeKey(arg)
		}
		return t.Name + "<" + strings.Join(parts, ",") + ">"
	}
	return t.Name
}

func cloneTypeScope(source map[string]ast.TypeRef) map[string]ast.TypeRef {
	out := make(map[string]ast.TypeRef, len(source))
	for name, typ := range source {
		out[name] = cloneTypeRef(typ)
	}
	return out
}

func containsParameter(typ ast.TypeRef, parameters map[string]bool) bool {
	if typ.Element != nil {
		return containsParameter(*typ.Element, parameters)
	}
	if parameters[typ.Name] {
		return true
	}
	for _, arg := range typ.Args {
		if containsParameter(arg, parameters) {
			return true
		}
	}
	return false
}

func (e *expander) inferCallTypeArguments(template *ast.Function, args []ast.Expr, at token.Position) ([]ast.TypeRef, error) {
	if len(args) != len(template.Params) {
		return nil, typeError(at, diagnostic.TypeGenericInference, "cannot infer type arguments for '%s': expected %d value arguments, got %d", template.Name, len(template.Params), len(args))
	}
	parameters := make(map[string]bool, len(template.TypeParams))
	for _, name := range template.TypeParams {
		parameters[name] = true
	}
	inferred := make(map[string]ast.TypeRef, len(template.TypeParams))
	for i, pattern := range template.Params {
		if !containsParameter(pattern.Type, parameters) {
			continue
		}
		actual, ok := e.inferExprType(args[i])
		if !ok {
			return nil, typeError(args[i].Pos(), diagnostic.TypeGenericInference, "cannot infer type arguments for '%s' from argument %d; add explicit type arguments with %s::<...>(...)", template.Name, i+1, template.Name)
		}
		if err := e.unifyType(pattern.Type, actual, parameters, inferred); err != nil {
			return nil, typeError(args[i].Pos(), diagnostic.TypeGenericInference, "cannot infer type arguments for '%s' consistently: %v", template.Name, err)
		}
	}
	out := make([]ast.TypeRef, len(template.TypeParams))
	for i, name := range template.TypeParams {
		typ, ok := inferred[name]
		if !ok {
			return nil, typeError(at, diagnostic.TypeGenericInference, "cannot infer type parameter '%s' for '%s'; use %s::<...>(...)", name, template.Name, template.Name)
		}
		out[i] = typ
	}
	return out, nil
}

func (e *expander) unifyType(pattern, actual ast.TypeRef, parameters map[string]bool, inferred map[string]ast.TypeRef) error {
	if pattern.Element == nil && len(pattern.Args) == 0 && parameters[pattern.Name] {
		if previous, exists := inferred[pattern.Name]; exists {
			if typeKey(previous) != typeKey(actual) {
				return fmt.Errorf("type parameter '%s' would be both %s and %s", pattern.Name, typeKey(previous), typeKey(actual))
			}
			return nil
		}
		inferred[pattern.Name] = cloneTypeRef(actual)
		return nil
	}
	if pattern.Name == "Array" && pattern.Element != nil {
		if actual.Name != "Array" || actual.Element == nil {
			return fmt.Errorf("expected %s but argument has type %s", typeKey(pattern), typeKey(actual))
		}
		return e.unifyType(*pattern.Element, *actual.Element, parameters, inferred)
	}
	if pattern.Name == actual.Name && len(pattern.Args) == len(actual.Args) && len(pattern.Args) > 0 {
		for i := range pattern.Args {
			if err := e.unifyType(pattern.Args[i], actual.Args[i], parameters, inferred); err != nil {
				return err
			}
		}
		return nil
	}
	if info, ok := e.structInstanceInfo[actual.Name]; ok && pattern.Name == info.template && len(pattern.Args) == len(info.args) {
		for i := range pattern.Args {
			if err := e.unifyType(pattern.Args[i], info.args[i], parameters, inferred); err != nil {
				return err
			}
		}
		return nil
	}
	if info, ok := e.enumInstanceInfo[actual.Name]; ok && pattern.Name == info.template && len(pattern.Args) == len(info.args) {
		for i := range pattern.Args {
			if err := e.unifyType(pattern.Args[i], info.args[i], parameters, inferred); err != nil {
				return err
			}
		}
		return nil
	}
	// Concrete parameter types are still checked by the regular checker after
	// specialization; they do not participate in type-variable inference.
	return nil
}

func (e *expander) inferExprType(expr ast.Expr) (ast.TypeRef, bool) {
	concrete := func(name string) (ast.TypeRef, bool) {
		return ast.TypeRef{Name: name, At: expr.Pos()}, name != ""
	}
	switch n := expr.(type) {
	case *ast.LiteralExpr:
		return concrete(n.Kind)
	case *ast.IdentifierExpr:
		typ, ok := e.variables[n.Name]
		return cloneTypeRef(typ), ok
	case *ast.ArrayExpr:
		if len(n.Elements) == 0 {
			return ast.TypeRef{}, false
		}
		element, ok := e.inferExprType(n.Elements[0])
		if !ok || element.Name == "unit" {
			return ast.TypeRef{}, false
		}
		for _, item := range n.Elements[1:] {
			other, ok := e.inferExprType(item)
			if !ok || typeKey(other) != typeKey(element) {
				return ast.TypeRef{}, false
			}
		}
		return ast.TypeRef{Name: "Array", At: n.At, Element: &element}, true
	case *ast.FieldExpr:
		object, ok := e.inferExprType(n.Object)
		if !ok {
			return ast.TypeRef{}, false
		}
		fields, ok := e.structFields[object.Name]
		if !ok {
			return ast.TypeRef{}, false
		}
		typ, ok := fields[n.Name]
		return cloneTypeRef(typ), ok
	case *ast.UnaryExpr:
		right, ok := e.inferExprType(n.Right)
		if !ok {
			return ast.TypeRef{}, false
		}
		if n.Op == "!" {
			return concrete("bool")
		}
		return right, true
	case *ast.BinaryExpr:
		left, lok := e.inferExprType(n.Left)
		right, rok := e.inferExprType(n.Right)
		if !lok || !rok || typeKey(left) != typeKey(right) {
			return ast.TypeRef{}, false
		}
		switch n.Op {
		case "==", "!=", "<", "<=", ">", ">=", "&&", "||":
			return concrete("bool")
		default:
			return left, true
		}
	case *ast.CallExpr:
		id, ok := n.Callee.(*ast.IdentifierExpr)
		if !ok {
			return ast.TypeRef{}, false
		}
		if template, ok := e.structTemplates[id.Name]; ok {
			inferred, err := e.inferStructTypeArguments(template, n.Args, n.At)
			if err != nil {
				return ast.TypeRef{}, false
			}
			name, err := e.instantiateStruct(template, inferred, n.At, nil, 0)
			if err != nil {
				return ast.TypeRef{}, false
			}
			return concrete(name)
		}
		if _, ok := e.structFields[id.Name]; ok {
			return concrete(id.Name)
		}
		switch id.Name {
		case "print", "assert":
			return concrete("unit")
		case "len", "i64":
			return concrete("i64")
		case "f64":
			return concrete("f64")
		}
		if template, generic := e.templates[id.Name]; generic {
			var typeArgs []ast.TypeRef
			if len(n.TypeArgs) > 0 {
				typeArgs = cloneTypeArgs(n.TypeArgs)
			} else {
				var err error
				typeArgs, err = e.inferCallTypeArguments(template, n.Args, n.At)
				if err != nil {
					return ast.TypeRef{}, false
				}
			}
			if len(typeArgs) != len(template.TypeParams) || template.ReturnType == nil {
				return ast.TypeRef{}, false
			}
			sub := make(map[string]ast.TypeRef, len(template.TypeParams))
			for i, param := range template.TypeParams {
				sub[param] = typeArgs[i]
			}
			ret, err := e.resolveType(substituteType(*template.ReturnType, sub), n.At, 0)
			if err != nil {
				return ast.TypeRef{}, false
			}
			return ret, true
		}
		fn, ok := e.functions[id.Name]
		if !ok || fn.ReturnType == nil {
			return ast.TypeRef{}, false
		}
		return cloneTypeRef(*fn.ReturnType), true
	case *ast.EnumVariantExpr:
		if template, ok := e.enumTemplates[n.EnumName]; ok {
			args := cloneTypeArgs(n.TypeArgs)
			if len(args) == 0 {
				var err error
				args, err = e.inferEnumTypeArguments(template, n.VariantName, n.Args, n.At)
				if err != nil {
					return ast.TypeRef{}, false
				}
			}
			name, err := e.instantiateEnum(template, args, n.At, nil, 0)
			if err != nil {
				return ast.TypeRef{}, false
			}
			return concrete(name)
		}
		return concrete(n.EnumName)
	case *ast.MatchExpr:
		if len(n.Arms) == 0 {
			return ast.TypeRef{}, false
		}
		typ, ok := e.inferExprType(n.Arms[0].Expr)
		if !ok {
			return ast.TypeRef{}, false
		}
		for _, arm := range n.Arms[1:] {
			other, ok := e.inferExprType(arm.Expr)
			if !ok || typeKey(other) != typeKey(typ) {
				return ast.TypeRef{}, false
			}
		}
		return typ, true
	case *ast.IndexExpr:
		collection, ok := e.inferExprType(n.Collection)
		if !ok || collection.Name != "Array" || collection.Element == nil {
			return ast.TypeRef{}, false
		}
		return cloneTypeRef(*collection.Element), true
	case *ast.SliceExpr:
		collection, ok := e.inferExprType(n.Collection)
		if !ok || collection.Name != "Array" || collection.Element == nil {
			return ast.TypeRef{}, false
		}
		return collection, true
	}
	return ast.TypeRef{}, false
}

func substituteType(t ast.TypeRef, sub map[string]ast.TypeRef) ast.TypeRef {
	if t.Element == nil && len(t.Args) == 0 {
		if replacement, ok := sub[t.Name]; ok {
			return cloneTypeRef(replacement)
		}
		return t
	}
	out := t
	if t.Element != nil {
		element := substituteType(*t.Element, sub)
		out.Element = &element
	}
	if len(t.Args) > 0 {
		out.Args = make([]ast.TypeRef, len(t.Args))
		for i, arg := range t.Args {
			out.Args[i] = substituteType(arg, sub)
		}
	}
	return out
}

func cloneTypeRef(t ast.TypeRef) ast.TypeRef {
	out := t
	if t.Element != nil {
		el := cloneTypeRef(*t.Element)
		out.Element = &el
	}
	if len(t.Args) > 0 {
		out.Args = make([]ast.TypeRef, len(t.Args))
		for i, arg := range t.Args {
			out.Args[i] = cloneTypeRef(arg)
		}
	}
	return out
}

func typeError(pos token.Position, code, format string, args ...any) error {
	return diagnostic.New(code, "type", pos, fmt.Sprintf(format, args...))
}

func (e *expander) rewriteFunction(fn *ast.Function, sub map[string]ast.TypeRef, depth int) (*ast.Function, error) {
	out := &ast.Function{At: fn.At, Name: fn.Name}
	out.Params = make([]ast.Param, len(fn.Params))
	previousVariables := e.variables
	e.variables = make(map[string]ast.TypeRef, len(fn.Params))
	for i, p := range fn.Params {
		typ, err := e.resolveType(substituteType(p.Type, sub), p.At, depth)
		if err != nil {
			e.variables = previousVariables
			return nil, err
		}
		out.Params[i] = ast.Param{At: p.At, Name: p.Name, Type: typ}
		e.variables[p.Name] = cloneTypeRef(typ)
	}
	if fn.ReturnType != nil {
		ret, err := e.resolveType(substituteType(*fn.ReturnType, sub), fn.ReturnType.At, depth)
		if err != nil {
			e.variables = previousVariables
			return nil, err
		}
		out.ReturnType = &ret
	}
	previousExpectedReturn := e.expectedReturn
	e.expectedReturn = nil
	if out.ReturnType != nil {
		expected := cloneTypeRef(*out.ReturnType)
		e.expectedReturn = &expected
	}
	body, err := e.rewriteBlock(fn.Body, sub, depth)
	e.expectedReturn = previousExpectedReturn
	e.variables = previousVariables
	if err != nil {
		return nil, err
	}
	out.Body = body
	return out, nil
}

func (e *expander) rewriteBlock(block *ast.BlockStmt, sub map[string]ast.TypeRef, depth int) (*ast.BlockStmt, error) {
	previousVariables := e.variables
	e.variables = cloneTypeScope(previousVariables)
	defer func() { e.variables = previousVariables }()
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
			v, err := e.resolveType(substituteType(*n.Annotation, sub), n.Annotation.At, depth)
			if err != nil {
				return nil, err
			}
			out.Annotation = &v
		}
		var expected *ast.TypeRef
		if out.Annotation != nil {
			valueType := cloneTypeRef(*out.Annotation)
			expected = &valueType
		}
		value, err := e.rewriteExprExpected(n.Value, sub, depth, expected)
		if err != nil {
			return nil, err
		}
		out.Value = value
		if out.Annotation != nil {
			e.variables[n.Name] = cloneTypeRef(*out.Annotation)
		} else if inferred, ok := e.inferExprType(out.Value); ok {
			e.variables[n.Name] = inferred
		}
		return out, nil
	case *ast.AssignStmt:
		var expected *ast.TypeRef
		if variableType, ok := e.variables[n.Name]; ok {
			valueType := cloneTypeRef(variableType)
			expected = &valueType
		}
		value, err := e.rewriteExprExpected(n.Value, sub, depth, expected)
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
			value, err := e.rewriteExprExpected(n.Value, sub, depth, e.expectedReturn)
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
	return e.rewriteExprExpected(expr, sub, depth, nil)
}

func (e *expander) rewriteExprExpected(expr ast.Expr, sub map[string]ast.TypeRef, depth int, expected *ast.TypeRef) (ast.Expr, error) {
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
		id, named := n.Callee.(*ast.IdentifierExpr)
		var expectedArgs []*ast.TypeRef
		if named && len(n.Args) > 0 {
			if fn, ok := e.functions[id.Name]; ok {
				if _, generic := e.templates[id.Name]; !generic && len(fn.Params) == len(n.Args) {
					expectedArgs = make([]*ast.TypeRef, len(n.Args))
					for i, param := range fn.Params {
						typ, err := e.resolveType(substituteType(param.Type, sub), param.At, depth)
						if err == nil {
							valueType := cloneTypeRef(typ)
							expectedArgs[i] = &valueType
						}
					}
				}
			}
		}
		if named && expected != nil {
			if template, generic := e.structTemplates[id.Name]; generic {
				expectedType, err := e.resolveType(substituteType(*expected, sub), n.At, depth)
				if err == nil {
					if info, ok := e.structInstanceInfo[expectedType.Name]; ok && info.template == template.Name {
						fieldSub := make(map[string]ast.TypeRef, len(template.TypeParams))
						for i, param := range template.TypeParams {
							fieldSub[param] = cloneTypeRef(info.args[i])
						}
						expectedArgs = make([]*ast.TypeRef, len(template.Fields))
						for i, field := range template.Fields {
							typ, fieldErr := e.resolveType(substituteType(field.Type, fieldSub), field.At, depth)
							if fieldErr == nil {
								valueType := cloneTypeRef(typ)
								expectedArgs[i] = &valueType
							}
						}
					}
				}
			}
		}
		args := make([]ast.Expr, len(n.Args))
		for i, arg := range n.Args {
			var argExpected *ast.TypeRef
			if i < len(expectedArgs) {
				argExpected = expectedArgs[i]
			}
			value, err := e.rewriteExprExpected(arg, sub, depth, argExpected)
			if err != nil {
				return nil, err
			}
			args[i] = value
		}
		if len(n.TypeArgs) > 0 {
			if !named {
				return nil, typeError(n.At, diagnostic.TypeGenericArgumentsOnNonGeneric, "explicit generic arguments require a named function or struct target")
			}
			if template, ok := e.templates[id.Name]; ok {
				instanceName, err := e.instantiate(template, n.TypeArgs, n.At, sub, depth)
				if err != nil {
					return nil, err
				}
				return &ast.CallExpr{At: n.At, Callee: &ast.IdentifierExpr{At: id.At, Name: instanceName}, Args: args}, nil
			}
			if template, ok := e.structTemplates[id.Name]; ok {
				instanceName, err := e.instantiateStruct(template, n.TypeArgs, n.At, sub, depth)
				if err != nil {
					return nil, err
				}
				return &ast.CallExpr{At: n.At, Callee: &ast.IdentifierExpr{At: id.At, Name: instanceName}, Args: args}, nil
			}
			return nil, typeError(n.At, diagnostic.TypeGenericArgumentsOnNonGeneric, "function or struct '%s' does not accept explicit type arguments", id.Name)
		}
		if named {
			if template, generic := e.templates[id.Name]; generic {
				inferred, inferErr := e.inferCallTypeArguments(template, args, n.At)
				if inferErr != nil {
					return nil, inferErr
				}
				instanceName, instantiateErr := e.instantiate(template, inferred, n.At, sub, depth)
				if instantiateErr != nil {
					return nil, instantiateErr
				}
				return &ast.CallExpr{At: n.At, Callee: &ast.IdentifierExpr{At: id.At, Name: instanceName}, Args: args}, nil
			}
			if template, generic := e.structTemplates[id.Name]; generic {
				inferred, inferErr := e.inferStructTypeArguments(template, args, n.At)
				if inferErr != nil {
					return nil, inferErr
				}
				instanceName, instantiateErr := e.instantiateStruct(template, inferred, n.At, sub, depth)
				if instantiateErr != nil {
					return nil, instantiateErr
				}
				return &ast.CallExpr{At: n.At, Callee: &ast.IdentifierExpr{At: id.At, Name: instanceName}, Args: args}, nil
			}
		}
		callee, err := e.rewriteExpr(n.Callee, sub, depth)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{At: n.At, Callee: callee, Args: args}, nil
	case *ast.EnumVariantExpr:
		enumName := n.EnumName
		var enumDecl *ast.EnumDecl
		if template, generic := e.enumTemplates[n.EnumName]; generic {
			args := cloneTypeArgs(n.TypeArgs)
			if len(args) == 0 && expected != nil {
				expectedType, err := e.resolveType(substituteType(*expected, sub), n.At, depth)
				if err != nil {
					return nil, err
				}
				if info, ok := e.enumInstanceInfo[expectedType.Name]; ok && info.template == template.Name {
					args = cloneTypeArgs(info.args)
				}
			}
			if len(args) == 0 {
				var err error
				args, err = e.inferEnumTypeArgumentsWithContext(template, n.VariantName, n.Args, n.At, sub, depth)
				if err != nil {
					return nil, err
				}
			}
			var err error
			enumName, err = e.instantiateEnum(template, args, n.At, sub, depth)
			if err != nil {
				return nil, err
			}
			enumDecl = e.enumDecls[enumName]
		} else {
			if len(n.TypeArgs) > 0 {
				return nil, typeError(n.At, diagnostic.TypeGenericArgumentsOnNonGeneric, "enum '%s' does not accept explicit type arguments", n.EnumName)
			}
			enumDecl = e.enumDecls[enumName]
		}
		var payloadTypes []ast.TypeRef
		if enumDecl != nil {
			for _, variant := range enumDecl.Variants {
				if variant.Name == n.VariantName {
					payloadTypes = variant.Payload
					break
				}
			}
		}
		args := make([]ast.Expr, len(n.Args))
		for i, arg := range n.Args {
			var payloadExpected *ast.TypeRef
			if i < len(payloadTypes) {
				valueType := cloneTypeRef(payloadTypes[i])
				payloadExpected = &valueType
			}
			value, err := e.rewriteExprExpected(arg, sub, depth, payloadExpected)
			if err != nil {
				return nil, err
			}
			args[i] = value
		}
		return &ast.EnumVariantExpr{At: n.At, EnumName: enumName, VariantName: n.VariantName, Args: args, HasPayload: n.HasPayload}, nil
	case *ast.MatchExpr:
		subject, err := e.rewriteExpr(n.Subject, sub, depth)
		if err != nil {
			return nil, err
		}
		matchEnumName := ""
		if subjectType, ok := e.inferExprType(subject); ok {
			matchEnumName = subjectType.Name
		}
		var genericInfo structInstanceInfo
		isGenericInstance := false
		if info, ok := e.enumInstanceInfo[matchEnumName]; ok {
			genericInfo, isGenericInstance = info, true
		}
		enumDecl := e.enumDecls[matchEnumName]
		arms := make([]ast.MatchArm, len(n.Arms))
		for i, arm := range n.Arms {
			pattern := arm.Pattern
			pattern.Bindings = append([]string(nil), arm.Pattern.Bindings...)
			if isGenericInstance && pattern.EnumName == genericInfo.template {
				pattern.EnumName = matchEnumName
			}
			previousVariables := e.variables
			e.variables = cloneTypeScope(previousVariables)
			if enumDecl != nil && !pattern.Wildcard {
				for _, variant := range enumDecl.Variants {
					if variant.Name == pattern.VariantName {
						for j, binding := range pattern.Bindings {
							if j < len(variant.Payload) {
								e.variables[binding] = cloneTypeRef(variant.Payload[j])
							}
						}
						break
					}
				}
			}
			value, err := e.rewriteExprExpected(arm.Expr, sub, depth, expected)
			e.variables = previousVariables
			if err != nil {
				return nil, err
			}
			arms[i] = ast.MatchArm{At: arm.At, Pattern: pattern, Expr: value}
		}
		return &ast.MatchExpr{At: n.At, Subject: subject, Arms: arms}, nil
	case *ast.ArrayExpr:
		elements := make([]ast.Expr, len(n.Elements))
		var elementExpected *ast.TypeRef
		if expected != nil && expected.Name == "Array" && expected.Element != nil {
			valueType := cloneTypeRef(*expected.Element)
			elementExpected = &valueType
		}
		for i, item := range n.Elements {
			value, err := e.rewriteExprExpected(item, sub, depth, elementExpected)
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
