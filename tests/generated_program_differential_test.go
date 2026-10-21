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

	"github.com/musashimusashino666/aster/internal/bytecode"
	"github.com/musashimusashino666/aster/internal/checker"
	"github.com/musashimusashino666/aster/internal/codegen"
	"github.com/musashimusashino666/aster/internal/parser"
	"github.com/musashimusashino666/aster/internal/toolchain"
	"github.com/musashimusashino666/aster/internal/vm"
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


func seededAggregateValues(rng *rand.Rand, length int) []int {
	values := make([]int, length)
	for i := range values {
		values[i] = rng.Intn(11) - 5
	}
	return values
}

func seededAggregateArrayLiteral(values []int) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strconv.Itoa(value)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func seededAggregateArrayDisplay(values []int) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strconv.Itoa(value)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func seededAggregateMatrixLiteral(rows [][]int) string {
	parts := make([]string, len(rows))
	for i, row := range rows {
		parts[i] = seededAggregateArrayLiteral(row)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func seededAggregateMatrixDisplay(rows [][]int) string {
	parts := make([]string, len(rows))
	for i, row := range rows {
		parts[i] = seededAggregateArrayDisplay(row)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// seededAggregateProgram generates a deterministic program that exercises all
// enum variants, match payload extraction, struct-wrapped nested arrays,
// copies, function argument/return boundaries, and mutation after extraction.
// Values stay small so arithmetic traps cannot obscure backend mismatches.
func seededAggregateProgram(seed int64) (string, string) {
	rng := rand.New(rand.NewSource(seed))
	flat := seededAggregateValues(rng, 3)
	matrix := [][]int{seededAggregateValues(rng, 3), seededAggregateValues(rng, 3)}
	boxedRows := [][]int{seededAggregateValues(rng, 3), seededAggregateValues(rng, 3)}
	flatMutation := 40 + rng.Intn(30)
	matrixMutation := 70 + rng.Intn(30)
	boxedMutation := 100 + rng.Intn(30)
	tag := 200 + rng.Intn(50)

	var source strings.Builder
	source.WriteString("struct Bundle { tag: i64, rows: Array<Array<i64>> }\n")
	source.WriteString("enum Payload { Empty, Flat(Array<i64>), Matrix(Array<Array<i64>>), Boxed(Bundle), }\n\n")
	source.WriteString("fn relay(value: Payload) -> Payload {\n")
	source.WriteString("    return match value {\n")
	source.WriteString("        Payload::Empty => Payload::Empty,\n")
	source.WriteString("        Payload::Flat(items) => Payload::Flat(items),\n")
	source.WriteString("        Payload::Matrix(rows) => Payload::Matrix(rows),\n")
	source.WriteString("        Payload::Boxed(bundle) => Payload::Boxed(bundle),\n")
	source.WriteString("    };\n")
	source.WriteString("}\n\n")

	source.WriteString("let emptyRoundTrip = relay(Payload::Empty);\n")
	source.WriteString("print(match emptyRoundTrip { Payload::Empty => 101, Payload::Flat(items) => items[0], Payload::Matrix(rows) => rows[0][0], Payload::Boxed(bundle) => bundle.tag, });\n")

	fmt.Fprintf(&source, "let mut flat: Array<i64> = %s;\n", seededAggregateArrayLiteral(flat))
	source.WriteString("let flatPacket = Payload::Flat(flat);\n")
	source.WriteString("let flatCopy = flatPacket;\n")
	source.WriteString("let flatRoundTrip = relay(flatCopy);\n")
	source.WriteString("let mut extractedFlat: Array<i64> = match flatRoundTrip {\n")
	source.WriteString("    Payload::Empty => [0],\n")
	source.WriteString("    Payload::Flat(items) => items,\n")
	source.WriteString("    Payload::Matrix(rows) => rows[0],\n")
	source.WriteString("    Payload::Boxed(bundle) => bundle.rows[0],\n")
	source.WriteString("};\n")
	fmt.Fprintf(&source, "extractedFlat[0] = %d;\n", flatMutation)
	source.WriteString("print(flat);\n")
	source.WriteString("print(match flatPacket { Payload::Empty => [0], Payload::Flat(items) => items, Payload::Matrix(rows) => rows[0], Payload::Boxed(bundle) => bundle.rows[0], });\n")
	source.WriteString("print(match flatRoundTrip { Payload::Empty => [0], Payload::Flat(items) => items, Payload::Matrix(rows) => rows[0], Payload::Boxed(bundle) => bundle.rows[0], });\n")
	source.WriteString("print(extractedFlat);\n")

	fmt.Fprintf(&source, "let mut matrix: Array<Array<i64>> = %s;\n", seededAggregateMatrixLiteral(matrix))
	source.WriteString("let matrixPacket = Payload::Matrix(matrix);\n")
	source.WriteString("let matrixCopy = matrixPacket;\n")
	source.WriteString("let matrixRoundTrip = relay(matrixCopy);\n")
	source.WriteString("let mut extractedMatrix: Array<Array<i64>> = match matrixRoundTrip {\n")
	source.WriteString("    Payload::Empty => [[0]],\n")
	source.WriteString("    Payload::Flat(items) => [items],\n")
	source.WriteString("    Payload::Matrix(rows) => rows,\n")
	source.WriteString("    Payload::Boxed(bundle) => bundle.rows,\n")
	source.WriteString("};\n")
	fmt.Fprintf(&source, "extractedMatrix[0][1] = %d;\n", matrixMutation)
	source.WriteString("print(matrix);\n")
	source.WriteString("print(match matrixPacket { Payload::Empty => [[0]], Payload::Flat(items) => [items], Payload::Matrix(rows) => rows, Payload::Boxed(bundle) => bundle.rows, });\n")
	source.WriteString("print(match matrixRoundTrip { Payload::Empty => [[0]], Payload::Flat(items) => [items], Payload::Matrix(rows) => rows, Payload::Boxed(bundle) => bundle.rows, });\n")
	source.WriteString("print(extractedMatrix);\n")

	fmt.Fprintf(&source, "let mut boxedRows: Array<Array<i64>> = %s;\n", seededAggregateMatrixLiteral(boxedRows))
	fmt.Fprintf(&source, "let bundle = Bundle(%d, boxedRows);\n", tag)
	source.WriteString("let boxedPacket = Payload::Boxed(bundle);\n")
	source.WriteString("let boxedCopy = boxedPacket;\n")
	source.WriteString("let boxedRoundTrip = relay(boxedCopy);\n")
	source.WriteString("let mut extractedBoxRows: Array<Array<i64>> = match boxedRoundTrip {\n")
	source.WriteString("    Payload::Empty => [[0]],\n")
	source.WriteString("    Payload::Flat(items) => [items],\n")
	source.WriteString("    Payload::Matrix(rows) => rows,\n")
	source.WriteString("    Payload::Boxed(bundle) => bundle.rows,\n")
	source.WriteString("};\n")
	fmt.Fprintf(&source, "extractedBoxRows[1][0] = %d;\n", boxedMutation)
	source.WriteString("print(boxedRows);\n")
	source.WriteString("print(match boxedPacket { Payload::Empty => [[0]], Payload::Flat(items) => [items], Payload::Matrix(rows) => rows, Payload::Boxed(bundle) => bundle.rows, });\n")
	source.WriteString("print(match boxedRoundTrip { Payload::Empty => [[0]], Payload::Flat(items) => [items], Payload::Matrix(rows) => rows, Payload::Boxed(bundle) => bundle.rows, });\n")
	source.WriteString("print(extractedBoxRows);\n")
	source.WriteString("print(match boxedRoundTrip { Payload::Empty => -101, Payload::Flat(items) => items[0], Payload::Matrix(rows) => rows[0][0], Payload::Boxed(bundle) => bundle.tag, });\n")

	mutatedFlat := append([]int(nil), flat...)
	mutatedFlat[0] = flatMutation
	mutatedMatrix := [][]int{append([]int(nil), matrix[0]...), append([]int(nil), matrix[1]...)}
	mutatedMatrix[0][1] = matrixMutation
	mutatedBoxedRows := [][]int{append([]int(nil), boxedRows[0]...), append([]int(nil), boxedRows[1]...)}
	mutatedBoxedRows[1][0] = boxedMutation
	want := []string{
		"101",
		seededAggregateArrayDisplay(flat),
		seededAggregateArrayDisplay(flat),
		seededAggregateArrayDisplay(flat),
		seededAggregateArrayDisplay(mutatedFlat),
		seededAggregateMatrixDisplay(matrix),
		seededAggregateMatrixDisplay(matrix),
		seededAggregateMatrixDisplay(matrix),
		seededAggregateMatrixDisplay(mutatedMatrix),
		seededAggregateMatrixDisplay(boxedRows),
		seededAggregateMatrixDisplay(boxedRows),
		seededAggregateMatrixDisplay(boxedRows),
		seededAggregateMatrixDisplay(mutatedBoxedRows),
		strconv.Itoa(tag),
	}
	return source.String(), strings.Join(want, "\n") + "\n"
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
	case "aggregate_payload_overflow":
		increment := 1 + rng.Intn(9)
		return fmt.Sprintf("struct Bundle { values: Array<i64> }\nenum Payload { Boxed(Bundle), }\nfn relay(value: Payload) -> Payload { return match value { Payload::Boxed(bundle) => Payload::Boxed(bundle), }; }\nlet payload = Payload::Boxed(Bundle([9223372036854775807]));\nprint(match relay(payload) { Payload::Boxed(bundle) => bundle.values[0] + %d, });\n", increment), "overflow"
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


func TestSeededAggregateProgramsMatchBackends(t *testing.T) {
	seeds := []int64{0xA66E, 0xBADD, 20261021}
	for _, seed := range seeds {
		seed := seed
		t.Run(fmt.Sprintf("seed_%x", seed), func(t *testing.T) {
			source, want := seededAggregateProgram(seed)
			repeatedSource, repeatedWant := seededAggregateProgram(seed)
			if repeatedSource != source || repeatedWant != want {
				t.Fatalf("aggregate generator is not deterministic for seed %#x", seed)
			}

			result := runSeededProgramOnBothBackends(t, source, "seeded-aggregate-differential.ast")
			if result.vmErr != nil {
				t.Fatalf("VM execution failed for seed %#x: %v\noutput: %q\nsource:\n%s", seed, result.vmErr, result.vmOutput, source)
			}
			if result.nativeErr != nil || result.nativeExitCode != 0 {
				t.Fatalf("Native execution failed for seed %#x: error=%v exit=%d\noutput:\n%s\nsource:\n%s", seed, result.nativeErr, result.nativeExitCode, result.nativeOutput, source)
			}
			if result.vmOutput != want {
				t.Fatalf("unexpected VM output for seed %#x:\n got: %q\nwant: %q\nsource:\n%s", seed, result.vmOutput, want, source)
			}
			if result.nativeOutput != want {
				t.Fatalf("unexpected Native output for seed %#x:\n got: %q\nwant: %q\nsource:\n%s", seed, result.nativeOutput, want, source)
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
		{name: "aggregate_payload_overflow", seed: 0xA66E0F},
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
