package tests

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/bytecode"
	"github.com/aster-lang/aster/internal/checker"
	"github.com/aster-lang/aster/internal/codegen"
	"github.com/aster-lang/aster/internal/parser"
	"github.com/aster-lang/aster/internal/toolchain"
	"github.com/aster-lang/aster/internal/vm"
)

func TestVMAndNativeBackendParity(t *testing.T) {
	source := `fn identity(values: Array<i64>) -> Array<i64> { return values; }
fn triangular(n: i64) -> i64 {
    let mut i: i64 = 0;
    let mut total: i64 = 0;
    while i <= n { total = total + i; i = i + 1; }
    return total;
}
print(triangular(7));
print("Aster");
let data: Array<i64> = [2, 4, 6];
print(data[1]);
print(len(data));
print(data);
print(identity(data));
let minimum: i64 = -9223372036854775808;
print(minimum % -1);
let ys: Array<i64> = [1, 2];
let zs = [ys[0], 2];
print(zs);
let unused = 1;
unused;
1 + 2;
len(data);
`
	tree, err := parser.Parse(source, "parity.ast")
	if err != nil {
		t.Fatal(err)
	}
	if err = checker.Check(tree); err != nil {
		t.Fatal(err)
	}
	bc, err := bytecode.Compile(tree)
	if err != nil {
		t.Fatal(err)
	}
	var vmOutput bytes.Buffer
	if err = vm.Run(bc, &vmOutput); err != nil {
		t.Fatal(err)
	}

	goSource, err := codegen.Generate(tree)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "aster-native")
	if err = toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin)
	var nativeOutput bytes.Buffer
	cmd.Stdout = &nativeOutput
	cmd.Stderr = &bytes.Buffer{}
	if err = cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if vmOutput.String() != nativeOutput.String() {
		t.Fatalf("backend output differs: VM=%q native=%q", vmOutput.String(), nativeOutput.String())
	}
	if vmOutput.String() != "28\nAster\n4\n3\n[2 4 6]\n[2 4 6]\n0\n[1 2]\n" {
		t.Fatalf("unexpected output: %q", vmOutput.String())
	}
}

func TestArrayIndexBoundsMatchBackends(t *testing.T) {
	tests := []struct {
		name  string
		index string
	}{
		{name: "negative_index", index: "-1"},
		{name: "length_index", index: "2"},
		{name: "large_positive_index", index: "4294967296"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := "let xs: Array<i64> = [7, 9]; print(xs[" + tt.index + "]);"
			tree, err := parser.Parse(source, "array-index-bounds.ast")
			if err != nil {
				t.Fatal(err)
			}
			if err = checker.Check(tree); err != nil {
				t.Fatal(err)
			}
			bc, err := bytecode.Compile(tree)
			if err != nil {
				t.Fatal(err)
			}
			var vmOutput bytes.Buffer
			vmErr := vm.Run(bc, &vmOutput)
			if vmErr == nil || !bytes.Contains([]byte(vmErr.Error()), []byte("out of bounds")) {
				t.Fatalf("VM should reject index %s, got output=%q error=%v", tt.index, vmOutput.String(), vmErr)
			}

			goSource, err := codegen.Generate(tree)
			if err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(t.TempDir(), "aster-index-bounds")
			if err = toolchain.BuildNative(goSource, bin); err != nil {
				t.Fatal(err)
			}
			output, nativeErr := exec.Command(bin).CombinedOutput()
			if nativeErr == nil {
				t.Fatalf("native backend should reject index %s, output=%q", tt.index, output)
			}
			if !bytes.Contains(output, []byte("out of bounds")) {
				t.Fatalf("native backend should report index %s out of bounds, got %q", tt.index, output)
			}
		})
	}
}

func TestCompiledBytecodeRoundTrip(t *testing.T) {
	tree, err := parser.Parse("print(42);", "roundtrip.ast")
	if err != nil {
		t.Fatal(err)
	}
	if err = checker.Check(tree); err != nil {
		t.Fatal(err)
	}
	bc, err := bytecode.Compile(tree)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "roundtrip.abc.json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = bytecode.Write(f, bc); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	f, err = os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := bytecode.Read(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = vm.Run(loaded, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "42\n" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestNativeBackendChecksIntegerOverflow(t *testing.T) {
	source := `let maximum: i64 = 9223372036854775807; print(maximum + 1);`
	tree, err := parser.Parse(source, "overflow.ast")
	if err != nil {
		t.Fatal(err)
	}
	if err = checker.Check(tree); err != nil {
		t.Fatal(err)
	}
	goSource, err := codegen.Generate(tree)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "aster-overflow")
	if err = toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected native runtime to reject signed integer overflow")
	}
	if !bytes.Contains(output, []byte("overflow")) {
		t.Fatalf("expected overflow diagnostic, got %q", output)
	}
}

func TestAnnotatedArrayElementOverflowMatchesBackends(t *testing.T) {
	source := `let xs: Array<i64> = [9223372036854775807, 1]; print(xs[0] + xs[1]);`
	tree, err := parser.Parse(source, "annotated-array-overflow.ast")
	if err != nil {
		t.Fatal(err)
	}
	if err = checker.Check(tree); err != nil {
		t.Fatal(err)
	}

	bc, err := bytecode.Compile(tree)
	if err != nil {
		t.Fatal(err)
	}
	var vmOutput bytes.Buffer
	vmErr := vm.Run(bc, &vmOutput)
	if vmErr == nil || !bytes.Contains([]byte(vmErr.Error()), []byte("overflow")) {
		t.Fatalf("VM should reject annotated-array arithmetic overflow, got output=%q error=%v", vmOutput.String(), vmErr)
	}

	goSource, err := codegen.Generate(tree)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "aster-array-overflow")
	if err = toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	output, nativeErr := exec.Command(bin).CombinedOutput()
	if nativeErr == nil {
		t.Fatalf("native backend should reject overflow, output=%q", output)
	}
	if !bytes.Contains(output, []byte("overflow")) {
		t.Fatalf("native output should explain overflow, got %q", output)
	}
}

func TestEveryExampleCompilesToValidatedBytecode(t *testing.T) {
	examples, err := filepath.Glob(filepath.Join("..", "examples", "*.ast"))
	if err != nil {
		t.Fatal(err)
	}
	if len(examples) == 0 {
		t.Fatal("no example .ast files found")
	}
	for _, path := range examples {
		t.Run(filepath.Base(path), func(t *testing.T) {
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			tree, err := parser.Parse(string(source), path)
			if err != nil {
				t.Fatal(err)
			}
			if err := checker.Check(tree); err != nil {
				t.Fatal(err)
			}
			program, err := bytecode.Compile(tree)
			if err != nil {
				t.Fatal(err)
			}
			if err := program.Validate(); err != nil {
				t.Fatalf("compiled bytecode failed validation: %v", err)
			}
		})
	}
}

func sourceMarkerPosition(t *testing.T, source, marker string, offset int) (int, int) {
	t.Helper()
	index := strings.Index(source, marker)
	if index < 0 {
		t.Fatalf("source marker %q not found in %q", marker, source)
	}
	index += offset
	if index < 0 || index > len(source) {
		t.Fatalf("source marker offset %d is outside source", index)
	}
	prefix := source[:index]
	line := strings.Count(prefix, "\n") + 1
	column := len(prefix) - strings.LastIndex(prefix, "\n")
	return line, column
}

func TestRuntimeErrorClassesMatchBackends(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		diagnostic string
		marker     string
		offset     int
	}{
		{name: "division_by_zero", source: "print(10 / 0);", diagnostic: "division by zero", marker: "/"},
		{name: "integer_overflow", source: "let maximum: i64 = 9223372036854775807; print(maximum + 1);", diagnostic: "overflow", marker: "+"},
		{name: "assertion_failure", source: "assert(false);", diagnostic: "assertion failed", marker: "assert(", offset: len("assert")},
		{name: "float_division_by_zero", source: "print(1.0 / 0.0);", diagnostic: "division by zero", marker: "/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, column := sourceMarkerPosition(t, tt.source, tt.marker, tt.offset)
			expectedPrefix := fmt.Sprintf("%d:%d: runtime error:", line, column)
			tree, err := parser.Parse(tt.source, tt.name+".ast")
			if err != nil {
				t.Fatal(err)
			}
			if err = checker.Check(tree); err != nil {
				t.Fatal(err)
			}
			program, err := bytecode.Compile(tree)
			if err != nil {
				t.Fatal(err)
			}
			var vmOutput bytes.Buffer
			vmErr := vm.Run(program, &vmOutput)
			if vmErr == nil || !strings.Contains(vmErr.Error(), tt.diagnostic) {
				t.Fatalf("VM should report %q, got output=%q error=%v", tt.diagnostic, vmOutput.String(), vmErr)
			}
			if !strings.HasPrefix(vmErr.Error(), expectedPrefix) {
				t.Fatalf("VM error should start with %q, got %q", expectedPrefix, vmErr.Error())
			}

			goSource, err := codegen.Generate(tree)
			if err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(t.TempDir(), "aster-runtime-error")
			if err = toolchain.BuildNative(goSource, bin); err != nil {
				t.Fatal(err)
			}
			nativeOutput, nativeErr := exec.Command(bin).CombinedOutput()
			if nativeErr == nil {
				t.Fatalf("native backend should fail with %q, output=%q", tt.diagnostic, nativeOutput)
			}
			if !strings.Contains(string(nativeOutput), tt.diagnostic) {
				t.Fatalf("native backend should report %q, got %q", tt.diagnostic, nativeOutput)
			}
			if !strings.HasPrefix(strings.TrimSpace(string(nativeOutput)), expectedPrefix) {
				t.Fatalf("native error should start with %q, got %q", expectedPrefix, nativeOutput)
			}
			if got := strings.TrimSpace(string(nativeOutput)); got != vmErr.Error() {
				t.Fatalf("backend diagnostics differ: VM=%q native=%q", vmErr.Error(), got)
			}
		})
	}
}
