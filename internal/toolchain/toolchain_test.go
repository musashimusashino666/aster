package toolchain

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func requireNativeToolchain(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"go", "gofmt"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("native toolchain executable %q is unavailable: %v", tool, err)
		}
	}
}

func TestBuildNativePreservesExistingOutputOnFailure(t *testing.T) {
	requireNativeToolchain(t)
	dir := t.TempDir()
	name := "aster-app"
	if os.PathSeparator == '\\' {
		name += ".exe"
	}
	output := filepath.Join(dir, name)
	const original = "pre-existing output"
	if err := os.WriteFile(output, []byte(original), 0o755); err != nil {
		t.Fatal(err)
	}

	err := BuildNative("package main\nfunc main( {\n", output)
	if err == nil {
		t.Fatal("expected invalid generated Go source to fail the build")
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("existing output changed after failed build: got %q, want %q", got, original)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(output) {
		t.Fatalf("temporary build artifacts remain: %v", entries)
	}
}

func TestBuildNativeReplacesOutputAfterSuccessfulBuild(t *testing.T) {
	requireNativeToolchain(t)
	dir := t.TempDir()
	name := "aster-app"
	if os.PathSeparator == '\\' {
		name += ".exe"
	}
	output := filepath.Join(dir, name)
	if err := os.WriteFile(output, []byte("old output"), 0o755); err != nil {
		t.Fatal(err)
	}

	const source = "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"new output\") }\n"
	if err := BuildNative(source, output); err != nil {
		t.Fatalf("BuildNative() error = %v", err)
	}
	cmd := exec.Command(output)
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("built executable failed: %v, output=%q", err, got)
	}
	if string(got) != "new output\n" {
		t.Fatalf("built executable output = %q, want %q", got, "new output\n")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(output) {
		t.Fatalf("temporary build artifacts remain: %v", entries)
	}
}
