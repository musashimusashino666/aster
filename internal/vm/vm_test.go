package vm

import (
	"bytes"
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/bytecode"
	"github.com/aster-lang/aster/internal/checker"
	"github.com/aster-lang/aster/internal/parser"
)

func runSource(t *testing.T, src string) (string, error) {
	t.Helper()
	p, err := parser.Parse(src, "test.ast")
	if err != nil {
		return "", err
	}
	if err = checker.Check(p); err != nil {
		return "", err
	}
	bc, err := bytecode.Compile(p)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	err = Run(bc, &out)
	return out.String(), err
}
func TestVMRecursionAndLoops(t *testing.T) {
	out, err := runSource(t, `fn fib(n: i64) -> i64 { if n <= 1 { return n; } else { return fib(n - 1) + fib(n - 2); } } let mut i: i64 = 0; let mut s: i64 = 0; while i < 6 { s = s + fib(i); i = i + 1; } print(s);`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "12\n" {
		t.Fatalf("output = %q", out)
	}
}
func TestVMShortCircuit(t *testing.T) {
	out, err := runSource(t, `fn fail() -> bool { assert(false); return true; } let ok = true || fail(); print(ok);`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "true\n" {
		t.Fatalf("output = %q", out)
	}
}
func TestVMRejectsIntegerOverflow(t *testing.T) {
	_, err := runSource(t, `let x: i64 = 9223372036854775807; print(x + 1);`)
	if err == nil || !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("expected overflow error, got %v", err)
	}
}
func TestVMRejectsDivisionByZero(t *testing.T) {
	_, err := runSource(t, `print(10 / 0);`)
	if err == nil || !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("expected division error, got %v", err)
	}
}
func TestVMRejectsMalformedProgram(t *testing.T) {
	p := &bytecode.Program{Format: "aster-bytecode", Version: bytecode.FormatVersion, Constants: []bytecode.Value{}, Entry: 0, Functions: []bytecode.Function{{Name: "main", Return: "unit", Code: []bytecode.Instruction{{Op: bytecode.OpConst, A: 99}, {Op: bytecode.OpHalt}}}}}
	if err := Run(p, &bytes.Buffer{}); err == nil {
		t.Fatal("expected malformed program rejection")
	}
}

func TestVMArrayLiteralIndexAndLength(t *testing.T) {
	out, err := runSource(t, `let values: Array<i64> = [2, 4, 6]; print(values[1]); print(len(values)); print(values);`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "4\n3\n[2 4 6]\n" {
		t.Fatalf("output = %q", out)
	}
}
func TestVMArrayBoundsCheck(t *testing.T) {
	_, err := runSource(t, `let values = [10, 20]; print(values[2]);`)
	if err == nil || !strings.Contains(err.Error(), "out of bounds") {
		t.Fatalf("expected bounds error, got %v", err)
	}
}

func TestVMRejectsStringGrowthBeyondLimit(t *testing.T) {
	_, err := runSource(t, `let mut s: string = "a"; while true { s = s + s; }`)
	if err == nil || !strings.Contains(err.Error(), "string length limit exceeded") {
		t.Fatalf("expected string length limit error, got %v", err)
	}
}

func TestVMPrintOutputLimitProtectsRepeatedArrayStrings(t *testing.T) {
	large := strings.Repeat("x", bytecode.MaxStringBytes/2)
	p := &bytecode.Program{
		Format:    "aster-bytecode",
		Version:   bytecode.FormatVersion,
		Constants: []bytecode.Value{bytecode.Array([]bytecode.Value{bytecode.Str(large), bytecode.Str(large)})},
		Functions: []bytecode.Function{{
			Name:   "main",
			Params: 0,
			Locals: 0,
			Return: "unit",
			Code: []bytecode.Instruction{
				{Op: bytecode.OpConst, A: 0},
				{Op: bytecode.OpBuiltin, A: 0, B: 1},
				{Op: bytecode.OpPop},
				{Op: bytecode.OpHalt},
			},
		}},
		Entry: 0,
	}
	err := Run(p, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "printed value exceeds") {
		t.Fatalf("expected bounded array display error, got %v", err)
	}
}

func TestVMRejectsOversizedArrayBuildInstruction(t *testing.T) {
	p := &bytecode.Program{
		Format:    "aster-bytecode",
		Version:   bytecode.FormatVersion,
		Constants: []bytecode.Value{},
		Functions: []bytecode.Function{{
			Name:   "main",
			Return: "unit",
			Code: []bytecode.Instruction{
				{Op: bytecode.OpBuildArray, A: bytecode.MaxArrayLength + 1},
				{Op: bytecode.OpHalt},
			},
		}},
		Entry: 0,
	}
	err := Run(p, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid array element count") {
		t.Fatalf("expected oversized array instruction rejection, got %v", err)
	}
}

func TestVMRejectsExcessiveActiveLocalSlots(t *testing.T) {
	// Each recursive frame allocates 70,000 slots. The individual function
	// passes bytecode verification, but the aggregate live-frame budget is
	// exceeded before the general call-depth limit.
	p := &bytecode.Program{
		Format:    "aster-bytecode",
		Version:   bytecode.FormatVersion,
		Constants: []bytecode.Value{},
		Functions: []bytecode.Function{
			{
				Name:   "recurse",
				Locals: 70000,
				Return: "unit",
				Code: []bytecode.Instruction{
					{Op: bytecode.OpCall, A: 0, B: 0},
					{Op: bytecode.OpReturn},
				},
			},
			{
				Name:   "__aster_main",
				Return: "unit",
				Code: []bytecode.Instruction{
					{Op: bytecode.OpCall, A: 0, B: 0},
					{Op: bytecode.OpPop},
					{Op: bytecode.OpHalt},
				},
			},
		},
		Entry: 1,
	}
	err := Run(p, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "active local slot limit exceeded") {
		t.Fatalf("expected cumulative frame allocation limit error, got %v", err)
	}
}

func TestIndexedArrayAssignmentUsesValueSemantics(t *testing.T) {
	source := `let mut original: Array<Array<i64>> = [[1, 2], [3, 4]];
let mut values: Array<Array<i64>> = original;
values[0][1] = 9;
print(values);
print(original);
let mut row: Array<i64> = [7, 8];
values[1] = row;
values[1][0] = 6;
print(values);
print(row);`
	out, err := runSource(t, source)
	if err != nil {
		t.Fatal(err)
	}
	want := "[[1 9] [3 4]]\n[[1 2] [3 4]]\n[[1 9] [6 8]]\n[7 8]\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestIndexedArrayAssignmentChecksBounds(t *testing.T) {
	_, err := runSource(t, "let mut values = [1, 2]; values[2] = 9;")
	if err == nil || !strings.Contains(err.Error(), "out of bounds") {
		t.Fatalf("expected out-of-bounds runtime error, got %v", err)
	}
}

func TestArraySlicesAreIndependentValues(t *testing.T) {
	source := `let original = [1, 2, 3, 4];
let mut part = original[1:3];
part[0] = 99;
print(part);
print(original);
print(original[2:2]);`
	out, err := runSource(t, source)
	if err != nil {
		t.Fatal(err)
	}
	want := "[99 3]\n[1 2 3 4]\n[]\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestArraySliceChecksBounds(t *testing.T) {
	for _, source := range []string{
		"let values = [1, 2, 3]; print(values[-1:2]);",
		"let values = [1, 2, 3]; print(values[2:1]);",
		"let values = [1, 2, 3]; print(values[0:4]);",
	} {
		_, err := runSource(t, source)
		if err == nil || !strings.Contains(err.Error(), "slice bounds") {
			t.Errorf("expected slice-bounds runtime error for %q, got %v", source, err)
		}
	}
}
