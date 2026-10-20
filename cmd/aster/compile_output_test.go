package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommandCompileCreatesMissingOutputDirectories(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "program.ast")
	if err := os.WriteFile(source, []byte("print(1);\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "missing", "nested", "program.abc.json")

	if err := commandCompile([]string{"-o", output, source}); err != nil {
		t.Fatalf("compile to a missing output directory: %v", err)
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatalf("stat compiled output: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("compiled output is not a regular file: %v", info.Mode())
	}
}
