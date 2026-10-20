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

var arrayAssignmentBenchmarkWorkloads = []struct {
	size   int
	rounds int
}{
	{size: 16, rounds: 2048},
	{size: 64, rounds: 512},
	{size: 256, rounds: 256},
	{size: 1024, rounds: 16},
}

// BenchmarkNativeProcessStartup estimates the process-launch floor using a
// no-op executable built with the same host Go toolchain as the native backend.
// It is a reference measurement, not a value to subtract mechanically from the
// array benchmark: executable size and runtime initialization can differ.
func BenchmarkNativeProcessStartup(b *testing.B) {
	const goSource = "package main\n\nfunc main() {}\n"
	bin := filepath.Join(b.TempDir(), "native-noop")
	if err := toolchain.BuildNative(goSource, bin); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(bin)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N), "ns/process")
}

func BenchmarkArrayElementAssignmentVM(b *testing.B) {
	for _, workload := range arrayAssignmentBenchmarkWorkloads {
		size, rounds := workload.size, workload.rounds
		b.Run(fmt.Sprintf("n=%d/rounds=%d", size, rounds), func(b *testing.B) {
			tree, err := parser.Parse(arrayAssignmentWorkloadSourceWithRounds(size, rounds), "array-assignment-benchmark.ast")
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
			assignments := float64(b.N) * float64(size) * float64(rounds)
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/assignments, "ns/assignment")
		})
	}
}

func BenchmarkArrayElementAssignmentNative(b *testing.B) {
	for _, workload := range arrayAssignmentBenchmarkWorkloads {
		size, rounds := workload.size, workload.rounds
		b.Run(fmt.Sprintf("n=%d/rounds=%d", size, rounds), func(b *testing.B) {
			tree, err := parser.Parse(arrayAssignmentWorkloadSourceWithRounds(size, rounds), "array-assignment-benchmark.ast")
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
			// process; the no-op benchmark above provides a startup reference.
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cmd := exec.Command(bin)
				cmd.Stdout = io.Discard
				cmd.Stderr = io.Discard
				if err := cmd.Run(); err != nil {
					b.Fatal(err)
				}
			}
			assignments := float64(b.N) * float64(size) * float64(rounds)
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/assignments, "ns/assignment")
		})
	}
}
