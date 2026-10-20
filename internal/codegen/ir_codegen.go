package codegen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aster-lang/aster/internal/ir"
	"github.com/aster-lang/aster/internal/langtypes"
	"github.com/aster-lang/aster/internal/limits"
)

func irLocalName(id ir.SymbolID) string {
	return "v_" + strconv.Itoa(int(id))
}

func irFunctionName(id ir.FunctionID) string {
	return "asterFn_" + strconv.Itoa(int(id))
}

func irStructTypeName(name string) string { return "asterType_" + name }

func isAggregateType(t string) bool {
	if _, ok := arrayElementType(t); ok {
		return true
	}
	switch t {
	case "i64", "f64", "bool", "string", "unit":
		return false
	default:
		return true
	}
}

func (g *generator) irStructType(structType ir.StructType) error {
	if !langtypes.ValidName(structType.Name) {
		return fmt.Errorf("codegen: invalid IR struct name %q", structType.Name)
	}
	g.line("type " + irStructTypeName(structType.Name) + " struct {")
	g.indent++
	for i, field := range structType.Fields {
		if !langtypes.ValidName(string(field.Type)) || field.Type == "unit" {
			return fmt.Errorf("codegen: invalid IR field type %q in struct %q", field.Type, structType.Name)
		}
		g.line(fmt.Sprintf("f_%d %s", i, goType(string(field.Type))))
	}
	g.indent--
	g.line("}")
	g.line("func (v " + irStructTypeName(structType.Name) + ") MarkArrayShared() {")
	g.indent++
	for i := range structType.Fields {
		g.line(fmt.Sprintf("asterMarkArrayShared(v.f_%d)", i))
	}
	g.indent--
	g.line("}")
	g.line("")
	return nil
}
func validateEnumResourceLimits(enumTypes []ir.EnumType) error {
	if len(enumTypes) > limits.MaxEnumTypes {
		return fmt.Errorf("codegen: enum type metadata exceeds %d-type safety limit", limits.MaxEnumTypes)
	}
	totalVariants := 0
	totalPayloadFields := 0
	for _, enumType := range enumTypes {
		totalVariants += len(enumType.Variants)
		if totalVariants > limits.MaxEnumVariants {
			return fmt.Errorf("codegen: enum variant metadata exceeds %d-variant safety limit", limits.MaxEnumVariants)
		}
		for _, variant := range enumType.Variants {
			totalPayloadFields += len(variant.Payload)
			if totalPayloadFields > limits.MaxEnumPayloadFields {
				return fmt.Errorf("codegen: enum payload metadata exceeds %d-field safety limit", limits.MaxEnumPayloadFields)
			}
		}
	}
	return nil
}

func (g *generator) irEnumType(enumType ir.EnumType) error {
	if !langtypes.ValidName(enumType.Name) || len(enumType.Variants) == 0 {
		return fmt.Errorf("codegen: invalid IR enum %q", enumType.Name)
	}
	for i, variant := range enumType.Variants {
		if !langtypes.ValidName(variant.Name) {
			return fmt.Errorf("codegen: invalid IR variant name %q in enum %q", variant.Name, enumType.Name)
		}
		if len(variant.Payload) == 0 {
			continue
		}
		g.line("type asterPayload_" + enumType.Name + "_" + strconv.Itoa(i) + " struct {")
		g.indent++
		for j, typ := range variant.Payload {
			if !langtypes.ValidName(string(typ)) || typ == "unit" {
				return fmt.Errorf("codegen: invalid enum payload type %q in %s::%s", typ, enumType.Name, variant.Name)
			}
			g.line(fmt.Sprintf("p_%d %s", j, goType(string(typ))))
		}
		g.indent--
		g.line("}")
	}
	g.line("type " + irStructTypeName(enumType.Name) + " struct {")
	g.indent++
	g.line("tag int")
	for i, variant := range enumType.Variants {
		if len(variant.Payload) > 0 {
			g.line(fmt.Sprintf("v_%d asterPayload_%s_%d", i, enumType.Name, i))
		}
	}
	g.indent--
	g.line("}")
	g.line("func (v " + irStructTypeName(enumType.Name) + ") MarkArrayShared() {")
	g.indent++
	g.line("switch v.tag {")
	g.indent++
	for i, variant := range enumType.Variants {
		if len(variant.Payload) == 0 {
			continue
		}
		g.line(fmt.Sprintf("case %d:", i))
		g.indent++
		for j, typ := range variant.Payload {
			if isAggregateType(string(typ)) {
				g.line(fmt.Sprintf("asterMarkArrayShared(v.v_%d.p_%d)", i, j))
			}
		}
		g.indent--
	}
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.line("")
	return nil
}

func irPositionArgs(node ir.Node) string {
	pos := node.Pos()
	return fmt.Sprintf("%d, %d", pos.Line, pos.Column)
}

// needsArrayShareAtBoundary reports expression forms that return an existing
// array storage owner. Fresh literals and slices already have independent
// outer storage; reads through an array index must be marked shared when their
// result is stored or otherwise becomes independently mutable.
func needsArrayShareAtBoundary(expr ir.Expr) bool {
	switch expr.(type) {
	case *ir.LocalExpr, *ir.IndexExpr, *ir.FieldExpr:
		return true
	default:
		return false
	}
}

func (g *generator) irFunction(fn *ir.Function) error {
	ret := string(fn.ReturnType)
	if !langtypes.ValidName(ret) {
		return fmt.Errorf("codegen: invalid IR return type %q for function %q", ret, fn.Name)
	}
	params := make([]string, len(fn.Params))
	for i, param := range fn.Params {
		paramType := string(param.Type)
		if !langtypes.ValidName(paramType) || param.Type == "unit" {
			return fmt.Errorf("codegen: invalid IR parameter type %q for function %q", paramType, fn.Name)
		}
		params[i] = irLocalName(param.ID) + " " + goType(paramType)
	}
	params = append(params, "_asterCallLine int", "_asterCallColumn int")
	g.line(fmt.Sprintf("func %s(%s) %s {", irFunctionName(fn.ID), strings.Join(params, ", "), goType(ret)))
	g.indent++
	g.line("asterEnterCall(_asterCallLine, _asterCallColumn)")
	g.line("defer asterLeaveCall()")
	if fn.Body != nil {
		if err := g.irBlockContents(fn.Body); err != nil {
			return err
		}
	}
	if fn.ReturnType == "unit" {
		g.line("return struct{}{}")
	}
	g.indent--
	g.line("}")
	g.line("")
	return nil
}

func (g *generator) irBlockContents(block *ir.BlockStmt) error {
	for _, stmt := range block.Statements {
		if err := g.irStatement(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (g *generator) irStatement(stmt ir.Stmt) error {
	if err := g.checkContext(); err != nil {
		return err
	}
	switch n := stmt.(type) {
	case *ir.BlockStmt:
		g.line("{")
		g.indent++
		err := g.irBlockContents(n)
		g.indent--
		g.line("}")
		return err
	case *ir.LetStmt:
		expr, err := g.irExpression(n.Value)
		if err != nil {
			return err
		}
		t := string(n.Local.Type)
		if !langtypes.ValidName(t) || t == "unit" {
			return fmt.Errorf("codegen: invalid IR local type %q for %q", t, n.Local.Name)
		}
		name := irLocalName(n.Local.ID)
		if isAggregateType(t) && needsArrayShareAtBoundary(n.Value) {
			expr = "asterShareValue(" + expr + ")"
		}
		g.line(name + " := " + expr)
		g.line("_ = " + name)
		return nil
	case *ir.IndexAssignStmt:
		root := irLocalName(n.Target)
		indices := make([]string, len(n.Indices))
		for i, index := range n.Indices {
			lowered, err := g.irExpression(index)
			if err != nil {
				return err
			}
			indices[i] = lowered
		}
		value, err := g.irExpression(n.Value)
		if err != nil {
			return err
		}
		g.line(fmt.Sprintf("%s = asterSetIndexPath(%s, []int64{%s}, %s, %s).(%s)",
			root, root, strings.Join(indices, ", "), value, irPositionArgs(n), goType(string(n.RootType))))
		return nil
	case *ir.AssignStmt:
		expr, err := g.irExpression(n.Value)
		if err != nil {
			return err
		}
		if isAggregateType(string(n.Value.ExprType())) && needsArrayShareAtBoundary(n.Value) {
			expr = "asterShareValue(" + expr + ")"
		}
		g.line(irLocalName(n.Target) + " = " + expr)
		return nil
	case *ir.ExprStmt:
		expr, err := g.irExpression(n.Expr)
		if err != nil {
			return err
		}
		g.line("_ = " + expr)
		return nil
	case *ir.IfStmt:
		if condition, constant := ir.BoolLiteral(n.Condition); constant {
			if condition {
				return g.irBlockContents(n.Then)
			}
			if n.Else != nil {
				return g.irStatement(n.Else)
			}
			return nil
		}
		condition, err := g.irExpression(n.Condition)
		if err != nil {
			return err
		}
		g.line("if " + condition + " {")
		g.indent++
		err = g.irBlockContents(n.Then)
		g.indent--
		if err != nil {
			return err
		}
		if n.Else != nil {
			g.line("} else {")
			g.indent++
			err = g.irStatement(n.Else)
			g.indent--
			if err != nil {
				return err
			}
			g.line("}")
		} else {
			g.line("}")
		}
		return nil
	case *ir.WhileStmt:
		if condition, constant := ir.BoolLiteral(n.Condition); constant {
			if !condition {
				return nil
			}
			g.line("for {")
			g.indent++
			err := g.irBlockContents(n.Body)
			g.indent--
			g.line("}")
			return err
		}
		condition, err := g.irExpression(n.Condition)
		if err != nil {
			return err
		}
		g.line("for " + condition + " {")
		g.indent++
		err = g.irBlockContents(n.Body)
		g.indent--
		g.line("}")
		return err
	case *ir.ReturnStmt:
		if n.Value == nil {
			g.line("return struct{}{}")
			return nil
		}
		expr, err := g.irExpression(n.Value)
		if err != nil {
			return err
		}
		if isAggregateType(string(n.Value.ExprType())) {
			expr = "asterShareValue(" + expr + ")"
		}
		g.line("return " + expr)
		return nil
	default:
		return fmt.Errorf("codegen: unsupported IR statement %T", stmt)
	}
}

func (g *generator) irEnumExpression(n *ir.EnumExpr) (string, error) {
	enumName := string(n.ResolvedType)
	arguments := make([]string, len(n.Args))
	for i, arg := range n.Args {
		value, err := g.irExpression(arg)
		if err != nil {
			return "", err
		}
		if isAggregateType(string(arg.ExprType())) {
			value = "asterShareValue(" + value + ")"
		}
		arguments[i] = value
	}
	if len(arguments) == 0 {
		return fmt.Sprintf("%s{tag: %d}", irStructTypeName(enumName), n.VariantIndex), nil
	}
	fields := make([]string, len(arguments))
	for i, arg := range arguments {
		fields[i] = fmt.Sprintf("p_%d: %s", i, arg)
	}
	return fmt.Sprintf("%s{tag: %d, v_%d: asterPayload_%s_%d{%s}}",
		irStructTypeName(enumName), n.VariantIndex, n.VariantIndex, enumName, n.VariantIndex, strings.Join(fields, ", ")), nil
}

func (g *generator) irMatchExpression(n *ir.MatchExpr) (string, error) {
	subject, err := g.irExpression(n.Subject)
	if err != nil {
		return "", err
	}
	if isAggregateType(string(n.Subject.ExprType())) && needsArrayShareAtBoundary(n.Subject) {
		subject = "asterShareValue(" + subject + ")"
	}
	subjectName := irLocalName(n.SubjectLocal.ID)
	parts := []string{"func() " + goType(string(n.ExprType())) + " {", subjectName + " := " + subject, "switch " + subjectName + ".tag {"}
	for _, arm := range n.Arms {
		if len(arm.VariantIndexes) == 0 {
			return "", fmt.Errorf("codegen: empty match arm target list")
		}
		targets := make([]string, len(arm.VariantIndexes))
		for i, target := range arm.VariantIndexes {
			targets[i] = strconv.Itoa(target)
		}
		parts = append(parts, "case "+strings.Join(targets, ", ")+":")
		if len(arm.Bindings) > 0 && len(arm.VariantIndexes) != 1 {
			return "", fmt.Errorf("codegen: payload bindings must belong to a single enum variant")
		}
		variantIndex := arm.VariantIndexes[0]
		for payloadIndex, binding := range arm.Bindings {
			value := fmt.Sprintf("%s.v_%d.p_%d", subjectName, variantIndex, payloadIndex)
			if isAggregateType(string(binding.Type)) {
				value = "asterShareValue(" + value + ")"
			}
			parts = append(parts, irLocalName(binding.ID)+" := "+value)
		}
		value, err := g.irExpression(arm.Expr)
		if err != nil {
			return "", err
		}
		parts = append(parts, "return "+value)
	}
	parts = append(parts, "}; panic("+strconv.Quote("invalid enum match dispatch")+") }()")
	return strings.Join(parts, "\n"), nil
}

func (g *generator) irExpression(expr ir.Expr) (string, error) {
	if err := g.checkContext(); err != nil {
		return "", err
	}
	switch n := expr.(type) {
	case *ir.LiteralExpr:
		switch n.Kind {
		case "i64":
			return "int64(" + strconv.FormatInt(n.Int, 10) + ")", nil
		case "f64":
			return "float64(" + strconv.FormatFloat(n.Float, 'g', -1, 64) + ")", nil
		case "bool":
			return strconv.FormatBool(n.Bool), nil
		case "string":
			if len(n.String) > limits.MaxStringBytes {
				pos := n.Pos()
				return "", fmt.Errorf("%d:%d: codegen: string literal exceeds %d-byte limit", pos.Line, pos.Column, limits.MaxStringBytes)
			}
			return strconv.Quote(n.String), nil
		default:
			return "", fmt.Errorf("codegen: unknown IR literal kind %q", n.Kind)
		}
	case *ir.ArrayExpr:
		if len(n.Elements) > limits.MaxArrayLength {
			pos := n.Pos()
			return "", fmt.Errorf("%d:%d: codegen: array literal has %d elements; maximum is %d", pos.Line, pos.Column, len(n.Elements), limits.MaxArrayLength)
		}
		items := make([]string, len(n.Elements))
		for i, item := range n.Elements {
			lowered, err := g.irExpression(item)
			if err != nil {
				return "", err
			}
			items[i] = lowered
		}
		t := string(n.ExprType())
		if !langtypes.ValidName(t) {
			return "", fmt.Errorf("codegen: invalid IR array type %q", t)
		}
		elem, ok := arrayElementType(t)
		if !ok {
			return "", fmt.Errorf("codegen: invalid IR array element type for %q", t)
		}
		return "asterNewArray([]" + goType(elem) + "{" + strings.Join(items, ", ") + "})", nil
	case *ir.EnumExpr:
		return g.irEnumExpression(n)
	case *ir.MatchExpr:
		return g.irMatchExpression(n)
	case *ir.FieldExpr:
		object, err := g.irExpression(n.Object)
		if err != nil {
			return "", err
		}
		if n.FieldIndex < 0 {
			return "", fmt.Errorf("codegen: invalid field index %d", n.FieldIndex)
		}
		return "(" + object + ").f_" + strconv.Itoa(n.FieldIndex), nil
	case *ir.IndexExpr:
		collection, err := g.irExpression(n.Collection)
		if err != nil {
			return "", err
		}
		index, err := g.irExpression(n.Index)
		if err != nil {
			return "", err
		}
		if n.Op != ir.OpIndexChecked {
			return "", fmt.Errorf("codegen: unsupported IR index operation %q", n.Op)
		}
		return "asterIndexI64(" + collection + ", " + index + ", " + irPositionArgs(n) + ")", nil
	case *ir.SliceExpr:
		collection, err := g.irExpression(n.Collection)
		if err != nil {
			return "", err
		}
		start, err := g.irExpression(n.Start)
		if err != nil {
			return "", err
		}
		end, err := g.irExpression(n.End)
		if err != nil {
			return "", err
		}
		if n.Op != ir.OpSliceChecked {
			return "", fmt.Errorf("codegen: unsupported IR slice operation %q", n.Op)
		}
		return "asterSlice(" + collection + ", " + start + ", " + end + ", " + irPositionArgs(n) + ")", nil
	case *ir.LocalExpr:
		return irLocalName(n.Symbol), nil
	case *ir.UnaryExpr:
		right, err := g.irExpression(n.Right)
		if err != nil {
			return "", err
		}
		switch n.Op {
		case ir.OpNegI64Checked:
			return "asterNegI64(" + right + ", " + irPositionArgs(n) + ")", nil
		case ir.OpNegF64:
			return "asterNegF64(" + right + ")", nil
		case ir.OpNotBool:
			return "(!" + right + ")", nil
		default:
			return "", fmt.Errorf("codegen: unsupported IR unary operation %q", n.Op)
		}
	case *ir.BinaryExpr:
		left, err := g.irExpression(n.Left)
		if err != nil {
			return "", err
		}
		right, err := g.irExpression(n.Right)
		if err != nil {
			return "", err
		}
		checkedHelpers := map[ir.RuntimeOp]string{
			ir.OpAddI64Checked:       "asterAddI64",
			ir.OpSubI64Checked:       "asterSubI64",
			ir.OpMulI64Checked:       "asterMulI64",
			ir.OpDivI64Checked:       "asterDivI64",
			ir.OpModI64Checked:       "asterModI64",
			ir.OpAddF64Checked:       "asterAddF64",
			ir.OpSubF64Checked:       "asterSubF64",
			ir.OpMulF64Checked:       "asterMulF64",
			ir.OpDivF64Checked:       "asterDivF64",
			ir.OpConcatStringChecked: "asterAddString",
		}
		if helper, ok := checkedHelpers[n.Op]; ok {
			return helper + "(" + left + ", " + right + ", " + irPositionArgs(n) + ")", nil
		}
		goOperators := map[ir.RuntimeOp]string{
			ir.OpEqual:                  "==",
			ir.OpNotEqual:               "!=",
			ir.OpLess:                   "<",
			ir.OpLessEqual:              "<=",
			ir.OpGreater:                ">",
			ir.OpGreaterEqual:           ">=",
			ir.OpLogicalAndShortCircuit: "&&",
			ir.OpLogicalOrShortCircuit:  "||",
		}
		if op, ok := goOperators[n.Op]; ok {
			return "(" + left + " " + op + " " + right + ")", nil
		}
		return "", fmt.Errorf("codegen: unsupported IR binary operation %q", n.Op)
	case *ir.CallExpr:
		args := make([]string, len(n.Args))
		for i, arg := range n.Args {
			lowered, err := g.irExpression(arg)
			if err != nil {
				return "", err
			}
			if n.Target == ir.CallUserFunction || n.Target == ir.CallStructConstructor {
				if isAggregateType(string(arg.ExprType())) {
					lowered = "asterShareValue(" + lowered + ")"
				}
			}
			args[i] = lowered
		}
		joined := strings.Join(args, ", ")
		switch n.Target {
		case ir.CallStructConstructor:
			fields := make([]string, len(args))
			for i, arg := range args {
				fields[i] = fmt.Sprintf("f_%d: %s", i, arg)
			}
			return irStructTypeName(n.FunctionName) + "{" + strings.Join(fields, ", ") + "}", nil
		case ir.CallPrint:
			return "asterPrint(" + joined + ", " + irPositionArgs(n) + ")", nil
		case ir.CallLen:
			if len(n.Args) != 1 {
				return "", fmt.Errorf("codegen: malformed IR len call")
			}
			if n.Args[0].ExprType() != "string" {
				return "asterLenArray(" + joined + ")", nil
			}
			return "asterLenString(" + joined + ")", nil
		case ir.CallAssert:
			return "asterAssert(" + joined + ", " + irPositionArgs(n) + ")", nil
		case ir.CallI64Cast:
			if len(n.Args) != 1 {
				return "", fmt.Errorf("codegen: malformed i64 cast call")
			}
			switch n.Args[0].ExprType() {
			case "i64":
				return args[0], nil
			case "f64":
				return "asterF64ToI64(" + args[0] + ", " + irPositionArgs(n) + ")", nil
			default:
				return "", fmt.Errorf("codegen: invalid i64 cast source type %q", n.Args[0].ExprType())
			}
		case ir.CallF64Cast:
			if len(n.Args) != 1 {
				return "", fmt.Errorf("codegen: malformed f64 cast call")
			}
			switch n.Args[0].ExprType() {
			case "f64":
				return args[0], nil
			case "i64":
				return "asterI64ToF64(" + args[0] + ")", nil
			default:
				return "", fmt.Errorf("codegen: invalid f64 cast source type %q", n.Args[0].ExprType())
			}
		case ir.CallUserFunction:
			callArgs := joined
			if callArgs != "" {
				callArgs += ", "
			}
			return irFunctionName(n.Function) + "(" + callArgs + irPositionArgs(n) + ")", nil
		default:
			return "", fmt.Errorf("codegen: unsupported IR call target %q", n.Target)
		}
	default:
		return "", fmt.Errorf("codegen: unsupported IR expression %T", expr)
	}
}
