package tests

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/aster-lang/aster/internal/bytecode"
	"github.com/aster-lang/aster/internal/checker"
	"github.com/aster-lang/aster/internal/codegen"
	"github.com/aster-lang/aster/internal/parser"
	"github.com/aster-lang/aster/internal/toolchain"
	"github.com/aster-lang/aster/internal/vm"
)

var arrayAssignmentBenchmarkSizes = []int{16, 64, 256}

func BenchmarkArrayElementAssignmentVM(b *testing.B) {
	for _, size := range arrayAssignmentBenchmarkSizes {
		b.Run(fmt.Sprintf("n=%d", size), func(b *testing.B) {
			tree, err := parser.Parse(arrayAssignmentWorkloadSource(size), "array-assignment-benchmark.ast")
			if err != nil {
				b.Fatal(err)
			}
			typed, err := checker.Analyze(tree)
			if err != nil {
				b.Fatal(err)
			}
			program, err := bytecode.CompileTyped(typed)
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := vm.Run(program, io.Discard); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkArrayElementAssignmentNative(b *testing.B) {
	for _, size := range arrayAssignmentBenchmarkSizes {
		b.Run(fmt.Sprintf("n=%d", size), func(b *testing.B) {
			tree, err := parser.Parse(arrayAssignmentWorkloadSource(size), "array-assignment-benchmark.ast")
			if err != nil {
				b.Fatal(err)
			}
			typed, err := checker.Analyze(tree)
			if err != nil {
				b.Fatal(err)
			}
			goSource, err := codegen.GenerateTyped(typed)
			if err != nil {
				b.Fatal(err)
			}
			bin := filepath.Join(b.TempDir(), "aster-array-assignment")
			if err := toolchain.BuildNative(goSource, bin); err != nil {
				b.Fatal(err)
			}

			// Compilation is excluded from the timed section. Each benchmark
			// operation includes native process startup and program execution.
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cmd := exec.Command(bin)
				cmd.Stdout = io.Discard
				cmd.Stderr = io.Discard
				if err := cmd.Run(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
