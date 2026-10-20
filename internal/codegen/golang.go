// Package codegen lowers the normalized, type-checked Aster IR to a self-contained Go program.
// The Go backend is an initial native backend; generated executables include
// Aster runtime checks rather than relying on unchecked Go arithmetic.
package codegen

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/aster-lang/aster/internal/ast"
	"github.com/aster-lang/aster/internal/checker"
	"github.com/aster-lang/aster/internal/langtypes"
)

type generator struct {
	out    bytes.Buffer
	indent int
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
	if typed == nil {
		return "", fmt.Errorf("codegen: nil typed program")
	}
	if typed.IR == nil {
		return "", fmt.Errorf("codegen: typed program has no normalized IR")
	}
	g := &generator{}
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
	for _, structType := range typed.IR.StructTypes {
		if err := g.irStructType(structType); err != nil {
			return "", err
		}
	}
	for _, enumType := range typed.IR.EnumTypes {
		if err := g.irEnumType(enumType); err != nil {
			return "", err
		}
	}
	for _, f := range typed.IR.Functions {
		if err := g.irFunction(f); err != nil {
			return "", err
		}
	}
	g.line("func main() {")
	g.indent++
	g.line("asterCallDepth = 1")
	g.line("asterSourceName = " + strconv.Quote(typed.IR.SourceName))
	g.line("defer func() { if recovered := recover(); recovered != nil { if failure, ok := recovered.(asterRuntimeFailure); ok { fmt.Fprintln(os.Stderr, failure.Error()); os.Exit(1) }; panic(recovered) } }()")
	for _, stmt := range typed.IR.Body {
		if err := g.irStatement(stmt); err != nil {
			return "", err
		}
	}
	g.indent--
	g.line("}")
	return g.out.String(), nil
}
func (g *generator) writePreamble() {
	g.out.WriteString(nativePreamble)
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
