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

const arrayAssignmentBenchmarkRounds = 256

func BenchmarkArrayElementAssignmentVM(b *testing.B) {
	for _, size := range arrayAssignmentBenchmarkSizes {
		b.Run(fmt.Sprintf("n=%d/rounds=%d", size, arrayAssignmentBenchmarkRounds), func(b *testing.B) {
			tree, err := parser.Parse(arrayAssignmentWorkloadSourceWithRounds(size, arrayAssignmentBenchmarkRounds), "array-assignment-benchmark.ast")
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
			assignments := float64(b.N) * float64(size) * float64(arrayAssignmentBenchmarkRounds)
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/assignments, "ns/assignment")
		})
	}
}

func BenchmarkArrayElementAssignmentNative(b *testing.B) {
	for _, size := range arrayAssignmentBenchmarkSizes {
		b.Run(fmt.Sprintf("n=%d/rounds=%d", size, arrayAssignmentBenchmarkRounds), func(b *testing.B) {
			tree, err := parser.Parse(arrayAssignmentWorkloadSourceWithRounds(size, arrayAssignmentBenchmarkRounds), "array-assignment-benchmark.ast")
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

			// Compilation is excluded. Each timed operation still launches a
			// process, but 256 rounds amortize startup across many assignments.
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cmd := exec.Command(bin)
				cmd.Stdout = io.Discard
				cmd.Stderr = io.Discard
				if err := cmd.Run(); err != nil {
					b.Fatal(err)
				}
			}
			assignments := float64(b.N) * float64(size) * float64(arrayAssignmentBenchmarkRounds)
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/assignments, "ns/assignment")
		})
	}
}
