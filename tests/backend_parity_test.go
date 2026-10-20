package tests

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
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

func TestGenericFunctionVMAndNativeBackendParity(t *testing.T) {
	source := `fn identity<T>(value: T) -> T { return value; }
fn select<T>(left: T, right: T, choose_left: bool) -> T {
    if choose_left { return left; }
    return right;
}
print(identity(42));
print(identity("aster"));
print(select(7, 9, false));
`
	tree, err := parser.Parse(source, "generic-function-parity.ast")
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
	if err := program.Validate(); err != nil {
		t.Fatalf("generic bytecode failed validation: %v", err)
	}
	var vmOutput bytes.Buffer
	if err := vm.Run(program, &vmOutput); err != nil {
		t.Fatal(err)
	}

	goSource, err := codegen.GenerateTyped(typed)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "aster-generic-parity")
	if err := toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	nativeOutput, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("Native generic program failed: %v\n%s", err, nativeOutput)
	}
	if got, want := vmOutput.String(), "42\naster\n9\n"; got != want {
		t.Fatalf("unexpected generic VM output: got %q want %q", got, want)
	}
	if got := string(nativeOutput); got != vmOutput.String() {
		t.Fatalf("generic backend output differs: VM=%q Native=%q", vmOutput.String(), got)
	}
}

func TestGenericStructVMAndNativeBackendParity(t *testing.T) {
	source := `struct Box<T> { value: T }
struct Pair<K, V> { key: K, value: V }
fn unwrap<T>(box: Box<T>) -> T { return box.value; }
let box = Box([1, 2, 3]);
let pair: Pair<string, Array<i64>> = Pair::<string, Array<i64>>("items", box.value);
print(unwrap(Box(42)));
print(pair.key);
print(pair.value[1]);
let mut original: Array<i64> = [4, 5];
let wrapped = Box(original);
original[0] = 99;
print(wrapped.value[0]);
`
	tree, err := parser.Parse(source, "generic-struct-parity.ast")
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
	if err := program.Validate(); err != nil {
		t.Fatalf("generic struct bytecode failed validation: %v", err)
	}
	var vmOutput bytes.Buffer
	if err := vm.Run(program, &vmOutput); err != nil {
		t.Fatal(err)
	}
	goSource, err := codegen.GenerateTyped(typed)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "aster-generic-struct-parity")
	if err := toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	nativeOutput, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("Native generic struct program failed: %v\n%s", err, nativeOutput)
	}
	if got, want := vmOutput.String(), "42\nitems\n2\n4\n"; got != want {
		t.Fatalf("unexpected generic struct VM output: got %q want %q", got, want)
	}
	if got := string(nativeOutput); got != vmOutput.String() {
		t.Fatalf("generic struct backend output differs: VM=%q Native=%q", vmOutput.String(), got)
	}
}

func TestExtendedVMAndNativeBackendParity(t *testing.T) {
	source := `print(1.5 + 2.25); print(false && (1 / 0 == 0)); print(true || (1 / 0 == 0)); print("alpha" == "alpha"); print("alpha" < "beta");`
	tree, err := parser.Parse(source, "extended-parity.ast")
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
	bin := filepath.Join(t.TempDir(), "aster-extended-parity")
	if err := toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	nativeOutput, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("native backend failed: %v\n%s", err, nativeOutput)
	}
	if got, want := vmOutput.String(), "3.75\nfalse\ntrue\ntrue\ntrue\n"; got != want {
		t.Fatalf("unexpected VM output: got %q want %q", got, want)
	}
	if got := string(nativeOutput); got != vmOutput.String() {
		t.Fatalf("backend output differs: VM=%q native=%q", vmOutput.String(), got)
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

func TestStringGrowthLimitMatchesBackends(t *testing.T) {
	source := `let mut s: string = "x"; while len(s) < 16777216 { s=s+s; } s = s + s;`
	line, column := sourceMarkerPosition(t, source, "s = s + s;", len("s = s "))
	expected := fmt.Sprintf("string-growth-limit.ast:%d:%d: runtime error: string length limit exceeded (maximum 16777216 UTF-8 bytes) [RUN009]", line, column)

	tree, err := parser.Parse(source, "string-growth-limit.ast")
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
	if vmErr == nil || vmErr.Error() != expected {
		t.Fatalf("VM error = %v, want %q", vmErr, expected)
	}

	goSource, err := codegen.Generate(tree)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "aster-string-growth-limit")
	if err = toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	nativeOutput, nativeErr := exec.Command(bin).CombinedOutput()
	if nativeErr == nil {
		t.Fatalf("native backend should reject oversized concatenation, output=%q", nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("native error = %q, want %q", got, expected)
	}
}

func TestPrintedArrayOutputLimitMatchesBackends(t *testing.T) {
	source := `let mut s: string = "x"; while len(s) < 8388608 { s=s+s; } let xs = [s, s]; print(xs);`
	line, column := sourceMarkerPosition(t, source, "print(xs)", len("print"))
	expected := fmt.Sprintf("printed-array-limit.ast:%d:%d: runtime error: printed value exceeds 16777216-byte output limit [RUN010]", line, column)

	tree, err := parser.Parse(source, "printed-array-limit.ast")
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
	if vmErr == nil || vmErr.Error() != expected {
		t.Fatalf("VM error = %v, want %q", vmErr, expected)
	}
	if vmOutput.Len() != 0 {
		t.Fatalf("VM must not partially print an oversized value, got %d bytes", vmOutput.Len())
	}

	goSource, err := codegen.Generate(tree)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "aster-printed-array-limit")
	if err = toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	nativeOutput, nativeErr := exec.Command(bin).CombinedOutput()
	if nativeErr == nil {
		t.Fatalf("native backend should reject oversized rendered output")
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("native error = %q, want %q", got, expected)
	}
}

func TestCallDepthLimitMatchesBackends(t *testing.T) {
	source := `fn recurse(n: i64) -> i64 { return recurse(n + 1); } print(recurse(0));`
	line, column := sourceMarkerPosition(t, source, "recurse(n + 1)", len("recurse"))
	expected := fmt.Sprintf("call-depth-limit.ast:%d:%d: runtime error: call depth limit exceeded (maximum 4096) [RUN012]", line, column)

	tree, err := parser.Parse(source, "call-depth-limit.ast")
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
	if vmErr == nil || vmErr.Error() != expected {
		t.Fatalf("VM error = %v, want %q", vmErr, expected)
	}

	goSource, err := codegen.Generate(tree)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "aster-call-depth-limit")
	if err = toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	nativeOutput, nativeErr := exec.Command(bin).CombinedOutput()
	if nativeErr == nil {
		t.Fatalf("native backend should reject excessive recursion, output=%q", nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("native error = %q, want %q", got, expected)
	}
}

func TestRuntimeErrorClassesMatchBackends(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		diagnostic string
		code       string
		marker     string
		offset     int
	}{
		{name: "division_by_zero", source: "print(10 / 0);", code: "RUN003", diagnostic: "division by zero", marker: "/"},
		{name: "integer_overflow", source: "let maximum: i64 = 9223372036854775807; print(maximum + 1);", code: "RUN002", diagnostic: "overflow", marker: "+"},
		{name: "integer_subtraction_overflow", source: "let minimum: i64 = -9223372036854775808; print(minimum - 1);", code: "RUN002", diagnostic: "overflow", marker: "minimum - 1", offset: len("minimum ")},
		{name: "integer_multiplication_overflow", source: "let maximum: i64 = 9223372036854775807; print(maximum * 2);", code: "RUN002", diagnostic: "overflow", marker: "maximum * 2", offset: len("maximum ")},
		{name: "integer_division_overflow", source: "let minimum: i64 = -9223372036854775808; print(minimum / -1);", code: "RUN002", diagnostic: "overflow", marker: "minimum / -1", offset: len("minimum ")},
		{name: "integer_negation_overflow", source: "let minimum: i64 = -9223372036854775808; print(-minimum);", code: "RUN002", diagnostic: "integer overflow in unary '-'", marker: "print(-minimum)", offset: len("print(")},
		{name: "remainder_by_zero", source: "print(10 % 0);", code: "RUN004", diagnostic: "remainder by zero", marker: "%"},
		{name: "assertion_failure", source: "assert(false);", code: "RUN001", diagnostic: "assertion failed", marker: "assert(", offset: len("assert")},
		{name: "float_division_by_zero", source: "print(1.0 / 0.0);", code: "RUN003", diagnostic: "division by zero", marker: "/"},
		{name: "float_non_finite", source: "fn grow(n: i64) -> f64 { if n == 0 { return 1.0; } else { return grow(n - 1) * 10.0; } } print(grow(400));", code: "RUN005", diagnostic: "non-finite floating-point result", marker: "*"},
		{name: "array_index_out_of_bounds", source: "let xs: Array<i64> = [1]; print(xs[1]);", code: "RUN006", diagnostic: "out of bounds", marker: "xs[1]", offset: len("xs")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, column := sourceMarkerPosition(t, tt.source, tt.marker, tt.offset)
			expectedPrefix := fmt.Sprintf("%s:%d:%d: runtime error:", tt.name+".ast", line, column)
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
			if !strings.HasPrefix(vmErr.Error(), expectedPrefix) || !strings.HasSuffix(vmErr.Error(), "["+tt.code+"]") {
				t.Fatalf("VM error should include source location %q and code %q, got %q", expectedPrefix, tt.code, vmErr.Error())
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
			if !strings.HasPrefix(strings.TrimSpace(string(nativeOutput)), expectedPrefix) ||
				!strings.HasSuffix(strings.TrimSpace(string(nativeOutput)), "["+tt.code+"]") {
				t.Fatalf("native error should include source location %q and code %q, got %q", expectedPrefix, tt.code, nativeOutput)
			}
			if got := strings.TrimSpace(string(nativeOutput)); got != vmErr.Error() {
				t.Fatalf("backend diagnostics differ: VM=%q native=%q", vmErr.Error(), got)
			}
		})
	}
}

func generatedIntExpr(rng *rand.Rand, depth int) string {
	literal := func() string { return strconv.Itoa(rng.Intn(19) - 9) }
	if depth <= 0 {
		return literal()
	}
	switch rng.Intn(7) {
	case 0:
		return literal()
	case 1:
		return "(-" + generatedIntExpr(rng, depth-1) + ")"
	case 2, 3, 4:
		ops := []string{"+", "-", "*"}
		op := ops[rng.Intn(len(ops))]
		return "(" + generatedIntExpr(rng, depth-1) + " " + op + " " + generatedIntExpr(rng, depth-1) + ")"
	case 5:
		return "(" + generatedIntExpr(rng, depth-1) + " / " + strconv.Itoa(rng.Intn(9)+1) + ")"
	default:
		return "(" + generatedIntExpr(rng, depth-1) + " % " + strconv.Itoa(rng.Intn(9)+1) + ")"
	}
}

func generatedStringExpr(rng *rand.Rand, depth int) string {
	if depth <= 0 || rng.Intn(3) == 0 {
		literals := []string{"", "Aster", "vm", "native", "xyz", "012"}
		return strconv.Quote(literals[rng.Intn(len(literals))])
	}
	return "(" + generatedStringExpr(rng, depth-1) + " + " + generatedStringExpr(rng, depth-1) + ")"
}

func generatedBoolExpr(rng *rand.Rand, depth int) string {
	leaf := func() string {
		switch rng.Intn(4) {
		case 0:
			return "true"
		case 1:
			return "false"
		case 2:
			return "(" + generatedIntExpr(rng, 2) + " < " + generatedIntExpr(rng, 2) + ")"
		default:
			return "(" + generatedStringExpr(rng, 1) + " == " + generatedStringExpr(rng, 1) + ")"
		}
	}
	if depth <= 0 {
		return leaf()
	}
	switch rng.Intn(6) {
	case 0:
		return leaf()
	case 1:
		return "(!" + generatedBoolExpr(rng, depth-1) + ")"
	case 2:
		return "(" + generatedBoolExpr(rng, depth-1) + " && " + generatedBoolExpr(rng, depth-1) + ")"
	case 3:
		return "(" + generatedBoolExpr(rng, depth-1) + " || " + generatedBoolExpr(rng, depth-1) + ")"
	case 4:
		return "(" + generatedIntExpr(rng, 2) + " != " + generatedIntExpr(rng, 2) + ")"
	default:
		return "(" + generatedStringExpr(rng, 1) + " < " + generatedStringExpr(rng, 1) + ")"
	}
}

func TestDeterministicGeneratedProgramsMatchBackends(t *testing.T) {
	// A fixed seed makes failures reproducible while exploring many typed expressions.
	rng := rand.New(rand.NewSource(0xA57E))
	var source strings.Builder
	for i := 0; i < 64; i++ {
		switch i % 4 {
		case 0:
			fmt.Fprintf(&source, "print(%s);\n", generatedIntExpr(rng, 3))
		case 1:
			fmt.Fprintf(&source, "print(%s);\n", generatedBoolExpr(rng, 3))
		case 2:
			fmt.Fprintf(&source, "print(%s);\n", generatedStringExpr(rng, 3))
		default:
			fmt.Fprintf(&source, "print([%s, %s, %s]);\n", generatedIntExpr(rng, 2), generatedIntExpr(rng, 2), generatedIntExpr(rng, 2))
		}
	}
	programSource := source.String()
	tree, err := parser.Parse(programSource, "generated-differential.ast")
	if err != nil {
		t.Fatalf("generated source did not parse: %v\nsource:\n%s", err, programSource)
	}
	typed, err := checker.Analyze(tree)
	if err != nil {
		t.Fatalf("generated source did not type-check: %v\nsource:\n%s", err, programSource)
	}
	bytecodeProgram, err := bytecode.Compile(typed.Tree)
	if err != nil {
		t.Fatalf("generated source did not compile to bytecode: %v\nsource:\n%s", err, programSource)
	}
	var vmOutput bytes.Buffer
	if err := vm.Run(bytecodeProgram, &vmOutput); err != nil {
		t.Fatalf("generated VM program failed: %v\nsource:\n%s", err, programSource)
	}
	goSource, err := codegen.GenerateTyped(typed)
	if err != nil {
		t.Fatalf("generated source did not generate Go: %v\nsource:\n%s", err, programSource)
	}
	bin := filepath.Join(t.TempDir(), "aster-generated-differential")
	if err := toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatalf("generated Go did not build: %v\nsource:\n%s", err, programSource)
	}
	nativeOutput, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("generated native program failed: %v\noutput:\n%s\nsource:\n%s", err, nativeOutput, programSource)
	}
	if vmOutput.String() != string(nativeOutput) {
		t.Fatalf("generated backend output differs\nVM:\n%s\nnative:\n%s\nsource:\n%s", vmOutput.String(), nativeOutput, programSource)
	}
}

func TestNativePrintWriteFailureMatchesVM(t *testing.T) {
	const source = "print(42);"
	const fullDevice = "/dev/full"
	full, err := os.OpenFile(fullDevice, os.O_WRONLY, 0)
	if err != nil {
		t.Skipf("write-failure device %s is unavailable: %v", fullDevice, err)
	}

	tree, err := parser.Parse(source, "print-write-error.ast")
	if err != nil {
		full.Close()
		t.Fatal(err)
	}
	if err = checker.Check(tree); err != nil {
		full.Close()
		t.Fatal(err)
	}
	program, err := bytecode.Compile(tree)
	if err != nil {
		full.Close()
		t.Fatal(err)
	}
	vmErr := vm.Run(program, full)
	closeErr := full.Close()
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if vmErr == nil || !strings.Contains(vmErr.Error(), "write output:") {
		t.Fatalf("VM should report stdout write failure, got %v", vmErr)
	}

	goSource, err := codegen.Generate(tree)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "aster-print-write-error")
	if err = toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	full, err = os.OpenFile(fullDevice, os.O_WRONLY, 0)
	if err != nil {
		t.Skipf("write-failure device %s became unavailable: %v", fullDevice, err)
	}
	cmd := exec.Command(bin)
	cmd.Stdout = full
	var nativeStderr bytes.Buffer
	cmd.Stderr = &nativeStderr
	nativeErr := cmd.Run()
	closeErr = full.Close()
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if nativeErr == nil {
		t.Fatal("native backend should fail when stdout cannot be written")
	}
	nativeDiagnostic := strings.TrimSpace(nativeStderr.String())
	line, column := sourceMarkerPosition(t, source, "print(", len("print"))
	expectedPrefix := fmt.Sprintf("print-write-error.ast:%d:%d: runtime error: write output:", line, column)
	if !strings.HasPrefix(vmErr.Error(), expectedPrefix) {
		t.Fatalf("VM diagnostic should start with %q, got %q", expectedPrefix, vmErr.Error())
	}
	if !strings.HasPrefix(nativeDiagnostic, expectedPrefix) || !strings.HasSuffix(nativeDiagnostic, "[RUN016]") {
		t.Fatalf("native diagnostic should include source location %q and code RUN016, got %q", expectedPrefix, nativeDiagnostic)
	}
	if !strings.HasSuffix(vmErr.Error(), "[RUN016]") {
		t.Fatalf("VM diagnostic should include code RUN016, got %q", vmErr.Error())
	}
	if !strings.Contains(vmErr.Error(), "no space left on device") || !strings.Contains(nativeDiagnostic, "no space left on device") {
		t.Fatalf("both backends should report the failed write reason: VM=%q native=%q", vmErr.Error(), nativeDiagnostic)
	}
}

func TestConstantConditionFlowBackendParity(t *testing.T) {
	source := "fn truth() -> i64 { if true { return 11; } }\n" +
		"fn falsity() -> i64 { if false { return 99; } else { return 22; } }\n" +
		"fn skip() -> i64 { while false { return 99; } return 33; }\n" +
		"fn repeat() -> i64 { while true { return 44; } }\n" +
		"fn diverge() -> i64 { while true { } }\n" +
		"print(truth()); print(falsity()); print(skip()); print(repeat());"
	tree, err := parser.Parse(source, "constant-flow.ast")
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
	bin := filepath.Join(t.TempDir(), "aster-constant-flow")
	if err := toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin)
	var nativeOutput bytes.Buffer
	cmd.Stdout = &nativeOutput
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if vmOutput.String() != nativeOutput.String() {
		t.Fatalf("backend output differs: VM=%q native=%q", vmOutput.String(), nativeOutput.String())
	}
	if got, want := vmOutput.String(), "11\n22\n33\n44\n"; got != want {
		t.Fatalf("execution output = %q, want %q", got, want)
	}
}

func TestIndexedArrayAssignmentBackendParityAndValueSemantics(t *testing.T) {
	source := `fn replaceFirst(values: Array<i64>) -> Array<i64> {
    let mut copy: Array<i64> = values;
    copy[0] = 9;
    return copy;
}
let mut matrix: Array<Array<i64>> = [[1, 2], [3, 4]];
let mut alias: Array<Array<i64>> = matrix;
alias[0][1] = 9;
print(alias);
print(matrix);
let mut row: Array<i64> = [7, 8];
alias[1] = row;
alias[1][0] = 6;
print(alias);
print(row);
print(replaceFirst([4, 5]));`
	tree, err := parser.Parse(source, "indexed-assignment-parity.ast")
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
	bin := filepath.Join(t.TempDir(), "aster-indexed-assignment")
	if err := toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin)
	var nativeOutput bytes.Buffer
	cmd.Stdout = &nativeOutput
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if vmOutput.String() != nativeOutput.String() {
		t.Fatalf("backend output differs: VM=%q native=%q", vmOutput.String(), nativeOutput.String())
	}
	want := "[[1 9] [3 4]]\n[[1 2] [3 4]]\n[[1 9] [6 8]]\n[7 8]\n[9 5]\n"
	if got := vmOutput.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestArraySliceBackendParityAndIndependentValues(t *testing.T) {
	source := `fn middle(values: Array<i64>) -> Array<i64> {
    return values[1:3];
}
let original = [1, 2, 3, 4];
let mut part = middle(original);
part[0] = 99;
print(part);
print(original);
let matrix = [[5, 6, 7], [8, 9, 10]];
print(matrix[1][0:2]);
print(original[2:2]);`
	tree, err := parser.Parse(source, "slice-parity.ast")
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
	bin := filepath.Join(t.TempDir(), "aster-slice-parity")
	if err := toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin)
	var nativeOutput bytes.Buffer
	cmd.Stdout = &nativeOutput
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if vmOutput.String() != nativeOutput.String() {
		t.Fatalf("backend output differs: VM=%q native=%q", vmOutput.String(), nativeOutput.String())
	}
	want := "[99 3]\n[1 2 3 4]\n[8 9]\n[]\n"
	if got := vmOutput.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func runTypedOnBothBackends(t *testing.T, source string) (string, error, string, error) {
	t.Helper()
	tree, err := parser.Parse(source, "numeric-cast-parity.ast")
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
	vmErr := vm.Run(program, &vmOutput)

	goSource, err := codegen.GenerateTyped(typed)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "aster-numeric-cast-parity")
	if err = toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	nativeOutput, nativeErr := exec.Command(bin).CombinedOutput()
	return vmOutput.String(), vmErr, string(nativeOutput), nativeErr
}

func TestExplicitNumericCastParity(t *testing.T) {
	source := `let large: i64 = 9007199254740993;
print(i64(7));
print(f64(1.25));
print(f64(large) == 9007199254740992.0);
print(i64(3.9));
print(i64(-3.9));
print(i64(-0.9));
print(i64(-9223372036854775808.0));`
	vmOutput, vmErr, nativeOutput, nativeErr := runTypedOnBothBackends(t, source)
	if vmErr != nil {
		t.Fatalf("VM execution failed: %v", vmErr)
	}
	if nativeErr != nil {
		t.Fatalf("native execution failed: %v\n%s", nativeErr, nativeOutput)
	}
	if got, want := vmOutput, "7\n1.25\ntrue\n3\n-3\n0\n-9223372036854775808\n"; got != want {
		t.Fatalf("unexpected VM output: got %q want %q", got, want)
	}
	if nativeOutput != vmOutput {
		t.Fatalf("backend output differs: VM=%q native=%q", vmOutput, nativeOutput)
	}
}

func TestExplicitFloatToIntegerRangeMatchesBackends(t *testing.T) {
	source := "print(i64(9223372036854775808.0));"
	line, column := sourceMarkerPosition(t, source, "i64(", len("i64"))
	expected := fmt.Sprintf("numeric-cast-parity.ast:%d:%d: runtime error: f64 value out of range for i64 conversion [RUN008]", line, column)

	vmOutput, vmErr, nativeOutput, nativeErr := runTypedOnBothBackends(t, source)
	if vmErr == nil || vmErr.Error() != expected {
		t.Fatalf("VM error = %v, want %q (output=%q)", vmErr, expected, vmOutput)
	}
	if nativeErr == nil {
		t.Fatalf("native backend should reject out-of-range conversion, output=%q", nativeOutput)
	}
	if got := strings.TrimSpace(nativeOutput); got != expected {
		t.Fatalf("native error = %q, want %q", got, expected)
	}
}

func TestStructFieldBackendParityAndArrayValueSemantics(t *testing.T) {
	source := `struct Point { x: i64, y: i64 }
struct Box { point: Point, values: Array<i64> }
fn pass(value: Box) -> Box { return value; }
let mut original: Array<i64> = [1, 2];
let boxValue = Box(Point(3, 4), original);
let returned = pass(boxValue);
let mut extracted: Array<i64> = returned.values;
extracted[0] = 9;
let mut nestedOriginal: Array<i64> = [5, 6];
let boxes: Array<Box> = [Box(Point(7, 8), nestedOriginal)];
let mut nestedExtracted: Array<i64> = boxes[0].values;
nestedExtracted[0] = 99;
print(returned.point.x);
print(original);
print(boxValue.values);
print(returned.values);
print(extracted);
print(nestedOriginal);
print(boxes[0].values);
print(nestedExtracted);`
	tree, err := parser.Parse(source, "struct-parity.ast")
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
	bin := filepath.Join(t.TempDir(), "aster-struct-parity")
	if err := toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	nativeOutput, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("native execution failed: %v\n%s", err, nativeOutput)
	}
	if got, want := vmOutput.String(), "3\n[1 2]\n[1 2]\n[1 2]\n[9 2]\n[5 6]\n[5 6]\n[99 6]\n"; got != want {
		t.Fatalf("VM output = %q, want %q", got, want)
	}
	if got := string(nativeOutput); got != vmOutput.String() {
		t.Fatalf("backend output differs: VM=%q native=%q", vmOutput.String(), got)
	}
}

func TestEnumPayloadMatchVMNativeParity(t *testing.T) {
	source := `enum Packet { Empty, Data(Array<i64>), Pair(i64, i64), }
fn first(packet: Packet) -> i64 {
	return match packet {
		Packet::Empty => 0,
		Packet::Data(values) => values[0],
		Packet::Pair(left, right) => left + right,
	};
}
let mut values = [10, 20];
let packet = Packet::Data(values);
values[0] = 99;
print(first(packet));
print(first(Packet::Pair(2, 3)));
print(match Packet::Empty { Packet::Empty => 7, _ => 9, });
`
	tree, err := parser.Parse(source, "enum-parity.ast")
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
	if err = vm.Run(program, &vmOutput); err != nil {
		t.Fatal(err)
	}
	goSource, err := codegen.GenerateTyped(typed)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "aster-enum-parity")
	if err = toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	nativeOutput, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("native backend failed: %v\n%s", err, nativeOutput)
	}
	if got, want := vmOutput.String(), "10\n5\n7\n"; got != want {
		t.Fatalf("unexpected VM output: got %q want %q", got, want)
	}
	if got := string(nativeOutput); got != vmOutput.String() {
		t.Fatalf("backend output differs: VM=%q native=%q", vmOutput.String(), got)
	}
}

func TestGenericEnumVMAndNativeBackendParity(t *testing.T) {
	source := `enum Option<T> { None, Some(T), }
enum Result<T, E> { Ok(T), Err(E), }
enum Pair<T> { Item(Option<T>, T), }
fn identity<T>(value: T) -> T { return value; }
fn unwrap<T>(value: Option<T>) -> T {
	return match value { Option::Some(item) => item, Option::None => 0, };
}
fn errorCode(message: string) -> i64 {
	if message == "error" { return 1; }
	return 0;
}
let mut original: Array<i64> = [1, 2];
let wrapped: Option<Array<i64>> = Option::Some(original);
original[0] = 9;
print(match wrapped { Option::Some(values) => values[0], Option::None => 0, });
print(unwrap(Option::Some(42)));
let result: Result<i64, string> = Result::Ok(7);
print(match result { Result::Ok(value) => value, Result::Err(message) => errorCode(message), });
let failed: Result<i64, string> = Result::Err("error");
print(match failed { Result::Ok(value) => value, Result::Err(message) => errorCode(message), });
let empty: Option<i64> = Option::None;
let nested: Option<Option<i64>> = Option::Some(Option::None);
let peerInferred = Pair::Item(Option::None, 7);
let throughCall = Option::Some(identity(42));
print(match empty { Option::Some(value) => value, Option::None => 0, });
print(match nested { Option::Some(inner) => match inner { Option::Some(value) => value, Option::None => 0, }, Option::None => 0, });
print(match peerInferred { Pair::Item(inner, value) => value + match inner { Option::Some(item) => item, Option::None => 0, }, });
print(match throughCall { Option::Some(value) => value, Option::None => 0, });
`
	tree, err := parser.Parse(source, "generic-enum-parity.ast")
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
	if err := program.Validate(); err != nil {
		t.Fatalf("generic enum bytecode failed validation: %v", err)
	}
	var vmOutput bytes.Buffer
	if err := vm.Run(program, &vmOutput); err != nil {
		t.Fatal(err)
	}
	goSource, err := codegen.GenerateTyped(typed)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "aster-generic-enum-parity")
	if err := toolchain.BuildNative(goSource, bin); err != nil {
		t.Fatal(err)
	}
	nativeOutput, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("Native generic enum program failed: %v\n%s", err, nativeOutput)
	}
	want := "1\n42\n7\n1\n0\n0\n7\n42\n"
	if got := vmOutput.String(); got != want {
		t.Fatalf("unexpected generic enum VM output: got %q want %q", got, want)
	}
	if got := string(nativeOutput); got != vmOutput.String() {
		t.Fatalf("generic enum backend output differs: VM=%q Native=%q", vmOutput.String(), got)
	}
}
