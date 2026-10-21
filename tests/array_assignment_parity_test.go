package tests

import (
	"bytes"
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

func TestArrayAssignmentPreservesCopiedValuesAcrossBackends(t *testing.T) {
	source := `fn identity(values: Array<i64>) -> Array<i64> { return values; }
let mut values = [1, 2, 3];
let snapshot = values;
let returned = identity(values);
values[1] = 99;
print(values);
print(snapshot);
print(returned);

let mut matrix = [[1, 2], [3, 4]];
let matrixSnapshot = matrix;
matrix[0][1] = 88;
print(matrix);
print(matrixSnapshot);`
	tree, err := parser.Parse(source, "array-assignment-value-semantics.ast")
	if err != nil {
		t.Fatal(err)
	}
	typed, err := checker.Analyze(tree)
	if err != nil {
		t.Fatal(err)
	}

	program, err := bytecode.CompileTyped(typed)
	if err != nil {
		t.Fatal(err)
	}
	var vmOutput bytes.Buffer
	if err := vm.Run(program, &vmOutput); err != nil {
		t.Fatal(err)
	}

	goSource, err := codegen.GenerateTyped(typed)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "aster-array-assignment")
	if err := toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin)
	var nativeOutput bytes.Buffer
	cmd.Stdout = &nativeOutput
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		t.Fatalf("native backend failed: %v", err)
	}

	const want = "[1 99 3]\n[1 2 3]\n[1 2 3]\n[[1 88] [3 4]]\n[[1 2] [3 4]]\n"
	if got := vmOutput.String(); got != want {
		t.Fatalf("unexpected VM output: got %q want %q", got, want)
	}
	if got := nativeOutput.String(); got != want {
		t.Fatalf("unexpected native output: got %q want %q", got, want)
	}
	if vmOutput.String() != nativeOutput.String() {
		t.Fatalf("backend output differs: VM=%q native=%q", vmOutput.String(), nativeOutput.String())
	}
}

// arrayAssignmentWorkloadSourceWithRounds repeatedly assigns every array element
// and consumes the final values so that the work remains observable to both backends.
func arrayAssignmentWorkloadSourceWithRounds(size, rounds int) string {
	var source strings.Builder
	source.WriteString("let mut values: Array<i64> = [")
	for i := 0; i < size; i++ {
		if i > 0 {
			source.WriteString(", ")
		}
		source.WriteByte('0')
	}
	source.WriteString("]; let mut round: i64 = 0; while round < ")
	source.WriteString(strconv.Itoa(rounds))
	source.WriteString(" { let mut i: i64 = 0; while i < len(values) { values[i] = i; i = i + 1; } round = round + 1; } ")
	source.WriteString("let mut checksum: i64 = 0; let mut j: i64 = 0; while j < len(values) { checksum = checksum + values[j]; j = j + 1; } print(checksum);")
	return source.String()
}
