// Package codegen lowers the normalized, type-checked Aster IR to a self-contained Go program.
// The Go backend is an initial native backend; generated executables include
// Aster runtime checks rather than relying on unchecked Go arithmetic.
package codegen

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/musashimusashino666/aster/internal/ast"
	"github.com/musashimusashino666/aster/internal/checker"
	"github.com/musashimusashino666/aster/internal/diagnostic"
	"github.com/musashimusashino666/aster/internal/langtypes"
	"github.com/musashimusashino666/aster/internal/limits"
	"github.com/musashimusashino666/aster/internal/token"
)

type limitedBuffer struct {
	data     bytes.Buffer
	limit    int
	exceeded bool
}

func (b *limitedBuffer) WriteString(s string) (int, error) {
	if b.exceeded {
		return 0, nil
	}
	if len(s) > b.limit-b.data.Len() {
		b.exceeded = true
		return 0, nil
	}
	return b.data.WriteString(s)
}

func (b *limitedBuffer) WriteByte(value byte) error {
	if b.exceeded || b.data.Len() >= b.limit {
		b.exceeded = true
		return nil
	}
	return b.data.WriteByte(value)
}

func (b *limitedBuffer) String() string { return b.data.String() }

type generator struct {
	out         limitedBuffer
	indent      int
	ctx         context.Context
	sourceSpans map[sourceSpanKey]sourceSpanValue
}

func (g *generator) checkContext() error {
	if g.ctx == nil {
		return nil
	}
	if err := g.ctx.Err(); err != nil {
		if err == context.DeadlineExceeded {
			return diagnostic.WrapToolchain(fmt.Errorf("native code generation timed out: %w", err))
		}
		return fmt.Errorf("native code generation canceled: %w", err)
	}
	return nil
}

func (g *generator) checkNativeSourceLimit(sourceName string, pos token.Position) error {
	if !g.out.exceeded {
		return nil
	}
	return diagnostic.NewAtSource(diagnostic.TypeResourceLimit, "codegen", sourceName, pos,
		fmt.Sprintf("generated Native source exceeds %d-byte limit", g.out.limit))
}

func firstNativeSourcePosition(tree *ast.Program) token.Position {
	if tree == nil {
		return token.Position{}
	}
	for _, decl := range tree.Decls {
		switch n := decl.(type) {
		case *ast.Function:
			return n.Pos()
		case *ast.StructDecl:
			return n.Pos()
		case *ast.EnumDecl:
			return n.Pos()
		case *ast.StmtDecl:
			if n.Statement != nil {
				return n.Statement.Pos()
			}
		}
	}
	return token.Position{}
}

func nativeDeclarationPosition(tree *ast.Program, name string) token.Position {
	if tree != nil {
		for _, decl := range tree.Decls {
			switch n := decl.(type) {
			case *ast.Function:
				if n.Name == name {
					return n.Pos()
				}
			case *ast.StructDecl:
				if n.Name == name {
					return n.Pos()
				}
			case *ast.EnumDecl:
				if n.Name == name {
					return n.Pos()
				}
			}
		}
	}
	return firstNativeSourcePosition(tree)
}

// Generate returns native-backend source. The resulting file can be compiled by
// the Go toolchain; users of Aster do not need to edit the generated source.
// Generate type-checks tree and lowers the resulting typed program to Go.
// Call GenerateTyped when the checker result is already available.
func Generate(tree *ast.Program) (string, error) {
	typed, err := checker.Analyze(tree)
	if err != nil {
		return "", err
	}
	return GenerateTyped(typed)
}

// GenerateTyped lowers the checker's normalized IR to Go. Expression types,
// resolved symbols, function targets, and checked operations come from the checker phase.
func GenerateTyped(typed *checker.TypedProgram) (string, error) {
	return GenerateTypedContext(context.Background(), typed)
}

// GenerateTypedContext lowers the normalized IR to Go, returning promptly when
// ctx is canceled. The context should cover code generation and any subsequent
// native toolchain work that shares the same CLI deadline.
func GenerateTypedContext(ctx context.Context, typed *checker.TypedProgram) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("codegen: nil context")
	}
	return generateTypedWithContextAndLimit(ctx, typed, limits.MaxNativeSourceBytes)
}

func generateTypedWithLimit(typed *checker.TypedProgram, sourceLimit int) (string, error) {
	return generateTypedWithContextAndLimit(context.Background(), typed, sourceLimit)
}

func generateTypedWithContextAndLimit(ctx context.Context, typed *checker.TypedProgram, sourceLimit int) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("codegen: nil context")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if typed == nil {
		return "", fmt.Errorf("codegen: nil typed program")
	}
	if typed.IR == nil {
		return "", fmt.Errorf("codegen: typed program has no normalized IR")
	}
	if err := validateEnumResourceLimits(typed.IR.EnumTypes); err != nil {
		return "", err
	}
	g := &generator{
		out: limitedBuffer{limit: sourceLimit}, ctx: ctx,
		sourceSpans: make(map[sourceSpanKey]sourceSpanValue),
	}
	for _, fn := range typed.IR.Functions {
		ret := string(fn.ReturnType)
		if !langtypes.ValidName(ret) {
			return "", fmt.Errorf("codegen: invalid IR return type %q for function %q", ret, fn.Name)
		}
		for _, param := range fn.Params {
			name := string(param.Type)
			if !langtypes.ValidName(name) || name == string(checker.Unit) {
				return "", fmt.Errorf("codegen: invalid IR parameter type %q for function %q", name, fn.Name)
			}
		}
	}
	g.writePreamble()
	if err := g.checkContext(); err != nil {
		return "", err
	}
	if err := g.checkNativeSourceLimit(typed.IR.SourceName, firstNativeSourcePosition(typed.Tree)); err != nil {
		return "", err
	}
	for _, structType := range typed.IR.StructTypes {
		if err := g.checkContext(); err != nil {
			return "", err
		}
		if err := g.irStructType(structType); err != nil {
			return "", err
		}
		if err := g.checkNativeSourceLimit(typed.IR.SourceName, nativeDeclarationPosition(typed.Tree, structType.Name)); err != nil {
			return "", err
		}
	}
	for _, enumType := range typed.IR.EnumTypes {
		if err := g.checkContext(); err != nil {
			return "", err
		}
		if err := g.irEnumType(enumType); err != nil {
			return "", err
		}
		if err := g.checkNativeSourceLimit(typed.IR.SourceName, nativeDeclarationPosition(typed.Tree, enumType.Name)); err != nil {
			return "", err
		}
	}
	for _, f := range typed.IR.Functions {
		if err := g.checkContext(); err != nil {
			return "", err
		}
		if err := g.irFunction(f); err != nil {
			return "", err
		}
		if err := g.checkNativeSourceLimit(typed.IR.SourceName, f.Pos()); err != nil {
			return "", err
		}
	}
	g.line("func main() {")
	g.indent++
	g.line("asterCallDepth = 1")
	g.line("asterSourceName = " + strconv.Quote(typed.IR.SourceName))
	g.line("defer func() { if recovered := recover(); recovered != nil { if failure, ok := recovered.(asterRuntimeFailure); ok { fmt.Fprintln(os.Stderr, failure.Error()); asterWriteRuntimeDiagnostic(failure); os.Exit(1) }; panic(recovered) } }()")
	for _, stmt := range typed.IR.Body {
		if err := g.checkContext(); err != nil {
			return "", err
		}
		if err := g.irStatement(stmt); err != nil {
			return "", err
		}
		if err := g.checkNativeSourceLimit(typed.IR.SourceName, stmt.Pos()); err != nil {
			return "", err
		}
	}
	g.indent--
	g.line("}")
	if err := g.writeSourceSpanTable(); err != nil {
		return "", err
	}
	if err := g.checkNativeSourceLimit(typed.IR.SourceName, firstNativeSourcePosition(typed.Tree)); err != nil {
		return "", err
	}
	return g.out.String(), nil
}
func (g *generator) writePreamble() {
	const fuelLimitMarker = "const asterMaxFuelUnits = 100000000"
	fuelLimit := "const asterMaxFuelUnits = " + strconv.Itoa(limits.MaxNativeFuelUnits)
	g.out.WriteString(strings.Replace(nativePreamble, fuelLimitMarker, fuelLimit, 1))
}

func goType(t string) string {
	switch t {
	case "unit":
		return "struct{}"
	case "i64":
		return "int64"
	case "f64":
		return "float64"
	case "bool":
		return "bool"
	case "string":
		return "string"
	default:
		if elem, ok := arrayElementType(t); ok {
			return "asterArray[" + goType(elem) + "]"
		}
		return "asterType_" + t
	}
}
func arrayElementType(t string) (string, bool) {
	if strings.HasPrefix(t, "Array<") && strings.HasSuffix(t, ">") {
		return t[len("Array<") : len(t)-1], true
	}
	return "", false
}
func (g *generator) line(s string) {
	g.out.WriteString(strings.Repeat("\t", g.indent))
	g.out.WriteString(s)
	g.out.WriteByte('\n')
}
