package bytecode_test

import (
	"bytes"
	"testing"

	"github.com/musashimusashino666/aster/internal/bytecode"
	"github.com/musashimusashino666/aster/internal/checker"
	"github.com/musashimusashino666/aster/internal/parser"
	"github.com/musashimusashino666/aster/internal/vm"
)

func TestCompileAndRunExplicitGenericFunctionInstances(t *testing.T) {
	source := `fn identity<T>(value: T) -> T { return value; }
print(identity::<i64>(7));
print(identity::<string>("ok")); `
	tree, err := parser.Parse(source, "generic-instance.ast")
	if err != nil {
		t.Fatal(err)
	}
	program, err := bytecode.Compile(tree)
	if err != nil {
		t.Fatal(err)
	}
	if err := program.Validate(); err != nil {
		t.Fatalf("generic-instance bytecode failed validation: %v", err)
	}
	var output bytes.Buffer
	if err := vm.Run(program, &output); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "7\nok\n"; got != want {
		t.Fatalf("VM output = %q, want %q", got, want)
	}
}

func TestCompileTypedConsumesNormalizedIR(t *testing.T) {
	source := "fn increment(value: i64) -> i64 { return value + 1; }\n" +
		"let mut values: Array<i64> = [2, 3, 4];\n" +
		"let mut total: i64 = 0;\n" +
		"let mut i: i64 = 0;\n" +
		"while i < len(values) { total = total + values[i]; i = i + 1; }\n" +
		"if true && (total == 9) { print(increment(total)); } else { assert(false); }\n" +
		"print(false && (1 / 0 == 0));\n" +
		"print(true || (1 / 0 == 0));\n"

	tree, err := parser.Parse(source, "typed-ir-bytecode.ast")
	if err != nil {
		t.Fatal(err)
	}
	typed, err := checker.Analyze(tree)
	if err != nil {
		t.Fatal(err)
	}
	if typed.IR == nil {
		t.Fatal("checker did not produce normalized IR")
	}
	typed.Tree = nil

	program, err := bytecode.CompileTyped(typed)
	if err != nil {
		t.Fatal(err)
	}
	if program.Source != "typed-ir-bytecode.ast" {
		t.Fatalf("bytecode source = %q", program.Source)
	}
	if err := program.Validate(); err != nil {
		t.Fatalf("IR-compiled bytecode failed validation: %v", err)
	}

	var output bytes.Buffer
	if err := vm.Run(program, &output); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "10\nfalse\ntrue\n"; got != want {
		t.Fatalf("VM output = %q, want %q", got, want)
	}
}

func TestCompileCompatibilityAPIUsesCheckedIR(t *testing.T) {
	tree, err := parser.Parse("print(6 * 7);", "compile-compat.ast")
	if err != nil {
		t.Fatal(err)
	}
	program, err := bytecode.Compile(tree)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := vm.Run(program, &output); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "42\n"; got != want {
		t.Fatalf("VM output = %q, want %q", got, want)
	}
}
