package tests

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/musashimusashino666/aster/internal/bytecode"
	"github.com/musashimusashino666/aster/internal/checker"
	"github.com/musashimusashino666/aster/internal/codegen"
	"github.com/musashimusashino666/aster/internal/parser"
	"github.com/musashimusashino666/aster/internal/toolchain"
	"github.com/musashimusashino666/aster/internal/vm"
)

func TestNegativeZeroParity(t *testing.T) {
	source := `print(-0.0); print(-1.0);`
	tree, err := parser.Parse(source, "negative-zero.ast")
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
	bin := filepath.Join(t.TempDir(), "aster-negative-zero")
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

	const want = "-0\n-1\n"
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
