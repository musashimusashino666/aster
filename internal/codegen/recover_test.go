package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/parser"
)

func runGeneratedNativeFailure(t *testing.T, source string) string {
	t.Helper()
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("native toolchain executable %q is unavailable: %v", "go", err)
	}

	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(goPath, "run", path)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("generated program succeeded; expected a failure, output=%q", output)
	}
	return string(output)
}

func TestNativeRuntimeFailureIsReportedAsLanguageDiagnostic(t *testing.T) {
	tree, err := parser.Parse("print(1 / 0);", "runtime-error.ast")
	if err != nil {
		t.Fatal(err)
	}
	source, err := Generate(tree)
	if err != nil {
		t.Fatal(err)
	}

	output := runGeneratedNativeFailure(t, source)
	if !strings.Contains(output, "runtime error: division by zero") {
		t.Fatalf("missing language-level runtime diagnostic: %q", output)
	}
	if strings.Contains(output, "panic:") || strings.Contains(output, "goroutine ") {
		t.Fatalf("language-level runtime error leaked a Go panic trace: %q", output)
	}
}

func TestNativeInternalPanicIsNotRewrittenAsRuntimeError(t *testing.T) {
	tree, err := parser.Parse("print(7);", "internal-panic.ast")
	if err != nil {
		t.Fatal(err)
	}
	source, err := Generate(tree)
	if err != nil {
		t.Fatal(err)
	}

	const marker = "\t_ = asterPrint(int64(7), "
	if !strings.Contains(source, marker) {
		t.Fatalf("could not find generated print statement to inject an internal panic:\n%s", source)
	}
	source = strings.Replace(source, marker, "\tpanic(\"internal panic sentinel\")\n"+marker, 1)

	output := runGeneratedNativeFailure(t, source)
	if !strings.Contains(output, "panic: internal panic sentinel") {
		t.Fatalf("internal panic was not propagated as a Go panic: %q", output)
	}
	if strings.Contains(output, "runtime error:") {
		t.Fatalf("internal panic was incorrectly relabeled as a language runtime error: %q", output)
	}
	if !strings.Contains(output, "goroutine ") {
		t.Fatalf("internal panic did not retain its diagnostic stack trace: %q", output)
	}
}
