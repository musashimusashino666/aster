package tests

import (
	"bytes"
	"fmt"
	"math/rand"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/bytecode"
	"github.com/aster-lang/aster/internal/checker"
	"github.com/aster-lang/aster/internal/codegen"
	"github.com/aster-lang/aster/internal/parser"
	"github.com/aster-lang/aster/internal/toolchain"
	"github.com/aster-lang/aster/internal/vm"
)

type seededBackendResult struct {
	vmOutput       string
	vmErr          error
	nativeOutput   string
	nativeErr      error
	nativeExitCode int
}

func runSeededProgramOnBothBackends(t *testing.T, source, name string) seededBackendResult {
	t.Helper()

	tree, err := parser.Parse(source, name)
	if err != nil {
		t.Fatalf("seeded source did not parse: %v\nsource:\n%s", err, source)
	}
	typed, err := checker.Analyze(tree)
	if err != nil {
		t.Fatalf("seeded source did not type-check: %v\nsource:\n%s", err, source)
	}
	program, err := bytecode.CompileTyped(typed)
	if err != nil {
		t.Fatalf("seeded source did not compile to bytecode: %v\nsource:\n%s", err, source)
	}

	var vmOutput bytes.Buffer
	vmErr := vm.Run(program, &vmOutput)

	goSource, err := codegen.GenerateTyped(typed)
	if err != nil {
		t.Fatalf("seeded source did not generate Native Go: %v\nsource:\n%s", err, source)
	}
	bin := filepath.Join(t.TempDir(), "aster-seeded-differential")
	if err := toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatalf("generated Native program did not build: %v\nsource:\n%s", err, source)
	}

	output, nativeErr := exec.Command(bin).CombinedOutput()
	exitCode := 0
	if nativeErr != nil {
		if exitErr, ok := nativeErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return seededBackendResult{
		vmOutput:       vmOutput.String(),
		vmErr:          vmErr,
		nativeOutput:   string(output),
		nativeErr:      nativeErr,
		nativeExitCode: exitCode,
	}
}

func seededSafeI64Expr(rng *rand.Rand, depth int) string {
	literal := func() string { return strconv.Itoa(rng.Intn(11) - 5) }
	if depth <= 0 {
		return literal()
	}
	switch rng.Intn(5) {
	case 0:
		return literal()
	case 1:
		return "(-" + seededSafeI64Expr(rng, depth-1) + ")"
	case 2:
		return "(" + seededSafeI64Expr(rng, depth-1) + " + " + seededSafeI64Expr(rng, depth-1) + ")"
	case 3:
		return "(" + seededSafeI64Expr(rng, depth-1) + " - " + seededSafeI64Expr(rng, depth-1) + ")"
	default:
		return "(" + seededSafeI64Expr(rng, depth-1) + " * " + seededSafeI64Expr(rng, depth-1) + ")"
	}
}

func seededI64Row(rng *rand.Rand, length int) string {
	values := make([]string, length)
	for i := range values {
		values[i] = seededSafeI64Expr(rng, 0)
	}
	return "[" + strings.Join(values, ", ") + "]"
}

// seededArrayProgram generates a valid program that exercises independent
// array values, rebinding, array replacement, slices, function calls, and
// nested mutation. Its small arithmetic expressions avoid intentional traps,
// so any runtime failure or backend-output mismatch is actionable.
func seededArrayProgram(seed int64) string {
	rng := rand.New(rand.NewSource(seed))
	const rowCount, rowLength = 3, 4

	rows := make([]string, rowCount)
	for i := range rows {
		rows[i] = seededI64Row(rng, rowLength)
	}

	var source strings.Builder
	source.WriteString("fn adjustFirst(values: Array<i64>, delta: i64) -> Array<i64> {\n")
	source.WriteString("    let mut result: Array<i64> = values;\n")
	source.WriteString("    result[0] = result[0] + delta;\n")
	source.WriteString("    return result;\n")
	source.WriteString("}\n\n")
	fmt.Fprintf(&source, "let mut matrix: Array<Array<i64>> = [%s];\n", strings.Join(rows, ", "))
	source.WriteString("let snapshot = matrix;\n")

	// Three cycles cover each operation shape with fresh local names. All
	// generated row/column indices are in range and every row remains length 4.
	for i := 0; i < 18; i++ {
		row := rng.Intn(rowCount)
		column := rng.Intn(rowLength)
		value := seededSafeI64Expr(rng, 2)
		switch i % 6 {
		case 0:
			fmt.Fprintf(&source, "matrix[%d][%d] = %s;\n", row, column, value)
			source.WriteString("print(matrix);\n")
		case 1:
			name := fmt.Sprintf("rowCopy%d", i)
			fmt.Fprintf(&source, "let mut %s: Array<i64> = matrix[%d];\n", name, row)
			fmt.Fprintf(&source, "%s[%d] = %s;\n", name, column, value)
			source.WriteString("print(matrix);\n")
			fmt.Fprintf(&source, "print(%s);\n", name)
		case 2:
			name := fmt.Sprintf("rebound%d", i)
			fmt.Fprintf(&source, "let mut %s: Array<i64> = %s;\n", name, seededI64Row(rng, rowLength))
			fmt.Fprintf(&source, "%s = matrix[%d];\n", name, row)
			fmt.Fprintf(&source, "%s[%d] = %s;\n", name, column, value)
			source.WriteString("print(matrix);\n")
			fmt.Fprintf(&source, "print(%s);\n", name)
		case 3:
			name := fmt.Sprintf("candidate%d", i)
			fmt.Fprintf(&source, "let mut %s: Array<i64> = %s;\n", name, seededI64Row(rng, rowLength))
			fmt.Fprintf(&source, "matrix[%d] = %s;\n", row, name)
			fmt.Fprintf(&source, "%s[%d] = %s;\n", name, column, value)
			source.WriteString("print(matrix);\n")
			fmt.Fprintf(&source, "print(%s);\n", name)
		case 4:
			name := fmt.Sprintf("slice%d", i)
			start := rng.Intn(rowLength)
			end := start + 1 + rng.Intn(rowLength-start)
			fmt.Fprintf(&source, "let mut %s: Array<i64> = matrix[%d][%d:%d];\n", name, row, start, end)
			sliceColumn := rng.Intn(end - start)
			fmt.Fprintf(&source, "%s[%d] = %s;\n", name, sliceColumn, value)
			source.WriteString("print(matrix);\n")
			fmt.Fprintf(&source, "print(%s);\n", name)
		case 5:
			name := fmt.Sprintf("returned%d", i)
			delta := rng.Intn(7) - 3
			fmt.Fprintf(&source, "let mut %s: Array<i64> = adjustFirst(matrix[%d], %d);\n", name, row, delta)
			fmt.Fprintf(&source, "%s[%d] = %s;\n", name, column, value)
			source.WriteString("print(matrix);\n")
			fmt.Fprintf(&source, "print(%s);\n", name)
		}
	}

	source.WriteString("print(matrix);\n")
	source.WriteString("print(snapshot);\n")
	return source.String()
}

// seededRuntimeFailureProgram generates one valid program that fails at
// runtime. The requested failure class is held constant, while the seed varies
// values so a regression remains reproducible without hardcoding every source.
func seededRuntimeFailureProgram(seed int64, kind string) (string, string) {
	rng := rand.New(rand.NewSource(seed))
	switch kind {
	case "integer_overflow":
		increment := 1 + rng.Intn(9)
		return fmt.Sprintf("let maximum: i64 = 9223372036854775807;\nprint(maximum + %d);\n", increment), "overflow"
	case "division_by_zero":
		numerator := 1 + rng.Intn(19)
		return fmt.Sprintf("print(%d / 0);\n", numerator), "division by zero"
	case "assertion_failure":
		witness := rng.Intn(31) - 15
		return fmt.Sprintf("let witness: i64 = %d;\nassert(false);\n", witness), "assertion failed"
	case "out_of_bounds":
		length := 1 + rng.Intn(5)
		values := seededI64Row(rng, length)
		invalidIndex := length + 1 + rng.Intn(17)
		return fmt.Sprintf("let values: Array<i64> = %s;\nprint(values[%d]);\n", values, invalidIndex), "out of bounds"
	default:
		panic("unknown seeded runtime failure kind: " + kind)
	}
}

func TestSeededArrayProgramsMatchBackends(t *testing.T) {
	seeds := []int64{0xA57E, 0xC0FFEE, 0x5EED, 20261020}
	for _, seed := range seeds {
		seed := seed
		t.Run(fmt.Sprintf("seed_%x", seed), func(t *testing.T) {
			source := seededArrayProgram(seed)
			if repeated := seededArrayProgram(seed); repeated != source {
				t.Fatalf("generator is not deterministic for seed %#x", seed)
			}

			result := runSeededProgramOnBothBackends(t, source, "seeded-array-differential.ast")
			if result.vmErr != nil {
				t.Fatalf("VM execution failed for seed %#x: %v\noutput: %q\nsource:\n%s", seed, result.vmErr, result.vmOutput, source)
			}
			if result.nativeErr != nil || result.nativeExitCode != 0 {
				t.Fatalf("Native execution failed for seed %#x: error=%v exit=%d\noutput:\n%s\nsource:\n%s", seed, result.nativeErr, result.nativeExitCode, result.nativeOutput, source)
			}
			if result.vmOutput != result.nativeOutput {
				t.Fatalf("backend output differs for seed %#x\nVM:\n%s\nNative:\n%s\nsource:\n%s", seed, result.vmOutput, result.nativeOutput, source)
			}
		})
	}
}

func TestSeededRuntimeFailureClassAndExitStatusMatch(t *testing.T) {
	cases := []struct {
		name string
		seed int64
	}{
		{name: "integer_overflow", seed: 0xBAD5EED},
		{name: "division_by_zero", seed: 0xB0A0D5},
		{name: "assertion_failure", seed: 0xA55E47},
		{name: "out_of_bounds", seed: 0x0B0A75},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			source, wantDiagnostic := seededRuntimeFailureProgram(tc.seed, tc.name)
			repeated, repeatedDiagnostic := seededRuntimeFailureProgram(tc.seed, tc.name)
			if repeated != source || repeatedDiagnostic != wantDiagnostic {
				t.Fatalf("generator is not deterministic for seed %#x and kind %s", tc.seed, tc.name)
			}

			result := runSeededProgramOnBothBackends(t, source, "seeded-runtime-failure.ast")
			if result.vmErr == nil || !strings.Contains(result.vmErr.Error(), wantDiagnostic) {
				t.Fatalf("VM should report %q for seed %#x, got error=%v output=%q\nsource:\n%s", wantDiagnostic, tc.seed, result.vmErr, result.vmOutput, source)
			}
			if result.vmOutput != "" {
				t.Fatalf("failed VM program should not emit a partial value, got %q", result.vmOutput)
			}
			if result.nativeErr == nil || result.nativeExitCode == 0 {
				t.Fatalf("Native should exit unsuccessfully for seed %#x, got error=%v exit=%d output=%q\nsource:\n%s", tc.seed, result.nativeErr, result.nativeExitCode, result.nativeOutput, source)
			}
			if got := nativeDiagnosticPayload(strings.TrimSpace(result.nativeOutput)); got != nativeDiagnosticPayload(result.vmErr.Error()) {
				t.Fatalf("backend diagnostic payloads differ for seed %#x: VM=%q Native=%q\nsource:\n%s", tc.seed, nativeDiagnosticPayload(result.vmErr.Error()), got, source)
			}
		})
	}
}
