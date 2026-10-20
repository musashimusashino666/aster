package vm

import (
	"io"
	"strconv"
	"strings"
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

func BenchmarkVMFunctionCallLoop(b *testing.B) {
	program := benchmarkBytecode(b, `fn increment(value: i64) -> i64 { return value + 1; } let mut i: i64 = 0; let mut total: i64 = 0; while i < 1000 { total = increment(total); i = i + 1; } print(total);`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Run(program, io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVMArrayTraversal(b *testing.B) {
	var source strings.Builder
	source.WriteString("let values: Array<i64> = [")
	for i := 0; i < 256; i++ {
		if i != 0 {
			source.WriteString(", " )
		}
		source.WriteString(strconv.Itoa(i))
	}
	source.WriteString("]; let mut i: i64 = 0; let mut total: i64 = 0; while i < len(values) { total = total + values[i]; i = i + 1; } print(total);")
	program := benchmarkBytecode(b, source.String())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Run(program, io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVMStringConcatAndUnicodeLen(b *testing.B) {
	program := benchmarkBytecode(b, `let base: string = "Aster界"; let mut value: string = base; let mut i: i64 = 0; let mut total: i64 = 0; while i < 64 { total = total + len(value); value = value + base; i = i + 1; } print(total);`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Run(program, io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVMFormattedArrayPrint(b *testing.B) {
	var source strings.Builder
	source.WriteString("let values: Array<i64> = [")
	for i := 0; i < 128; i++ {
		if i != 0 {
			source.WriteString(", " )
		}
		source.WriteString(strconv.Itoa(i))
	}
	source.WriteString("]; print(values);")
	program := benchmarkBytecode(b, source.String())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Run(program, io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}
