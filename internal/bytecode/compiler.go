package bytecode

import (
	"fmt"

	"github.com/musashimusashino666/aster/internal/ast"
	"github.com/musashimusashino666/aster/internal/checker"
)

// Compile type-checks an Aster AST and lowers its normalized IR to portable
// bytecode. Use CompileTyped when checker.Analyze has already run.
func Compile(tree *ast.Program) (*Program, error) {
	typed, err := checker.Analyze(tree)
	if err != nil {
		return nil, err
	}
	return CompileTyped(typed)
}

// CompileTyped lowers the checker's normalized IR to validated bytecode.
// It does not read or re-resolve the source AST.
func CompileTyped(typed *checker.TypedProgram) (*Program, error) {
	if typed == nil || typed.IR == nil {
		return nil, fmt.Errorf("bytecode: typed program has no normalized IR")
	}
	return compileIR(typed.IR)
}
