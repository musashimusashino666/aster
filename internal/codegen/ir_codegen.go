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

func irPositionArgs(node ir.Node) string {
	pos := node.Pos()
	return fmt.Sprintf("%d, %d", pos.Line, pos.Column)
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
		g.line(name + " := " + expr)
		g.line("_ = " + name)
		return nil
	case *ir.AssignStmt:
		expr, err := g.irExpression(n.Value)
		if err != nil {
			return err
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
		g.line("return " + expr)
		return nil
	default:
		return fmt.Errorf("codegen: unsupported IR statement %T", stmt)
	}
}

func (g *generator) irExpression(expr ir.Expr) (string, error) {
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
		return goType(t) + "{" + strings.Join(items, ", ") + "}", nil
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
			return "(-" + right + ")", nil
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
			args[i] = lowered
		}
		joined := strings.Join(args, ", ")
		switch n.Target {
		case ir.CallPrint:
			return "asterPrint(" + joined + ", " + irPositionArgs(n) + ")", nil
		case ir.CallLen:
			if len(n.Args) != 1 {
				return "", fmt.Errorf("codegen: malformed IR len call")
			}
			if n.Args[0].ExprType() != "string" {
				return "int64(len(" + joined + "))", nil
			}
			return "asterLenString(" + joined + ")", nil
		case ir.CallAssert:
			return "asterAssert(" + joined + ", " + irPositionArgs(n) + ")", nil
		case ir.CallUserFunction:
			return irFunctionName(n.Function) + "(" + joined + ", " + irPositionArgs(n) + ")", nil
		default:
			return "", fmt.Errorf("codegen: unsupported IR call target %q", n.Target)
		}
	default:
		return "", fmt.Errorf("codegen: unsupported IR expression %T", expr)
	}
}
