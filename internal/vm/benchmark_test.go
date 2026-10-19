package vm

import (
	"io"
	"testing"

	"github.com/aster-lang/aster/internal/bytecode"
	"github.com/aster-lang/aster/internal/checker"
	"github.com/aster-lang/aster/internal/parser"
)

func benchmarkBytecode(b *testing.B, source string) *bytecode.Program {
	b.Helper()
	tree, err := parser.Parse(source, "benchmark.ast")
	if err != nil {
		b.Fatal(err)
	}
	if err = checker.Check(tree); err != nil {
		b.Fatal(err)
	}
	program, err := bytecode.Compile(tree)
	if err != nil {
		b.Fatal(err)
	}
	return program
}

func BenchmarkVMNumericLoop(b *testing.B) {
	program := benchmarkBytecode(b, `let mut i: i64 = 0; let mut sum: i64 = 0; while i < 1000 { sum = sum + i; i = i + 1; } print(sum);`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Run(program, io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVMRecursiveFibonacci(b *testing.B) {
	program := benchmarkBytecode(b, `fn fib(n: i64) -> i64 { if n <= 1 { return n; } else { return fib(n - 1) + fib(n - 2); } } print(fib(18));`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Run(program, io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}
