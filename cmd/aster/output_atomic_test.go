package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/aster-lang/aster/internal/bytecode"
)

func TestWriteOutputAtomicallyPreservesExistingFileOnWriteFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "program.abc.json")
	const original = "known-good output"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	writeErr := errors.New("simulated encoder failure")
	err := writeOutputAtomically(path, func(w io.Writer) error {
		if _, err := io.WriteString(w, "partial replacement"); err != nil {
			t.Fatal(err)
		}
		return writeErr
	})
	if !errors.Is(err, writeErr) {
		t.Fatalf("writeOutputAtomically() error = %v, want %v", err, writeErr)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("existing output changed after failed write: got %q, want %q", got, original)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("temporary artifacts remain after failure: %v", entries)
	}
}

func TestWriteOutputAtomicallyReplacesCompletedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "program.abc.json")
	if err := os.WriteFile(path, []byte("old output"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := writeOutputAtomically(path, func(w io.Writer) error {
		_, err := io.WriteString(w, "complete output")
		return err
	})
	if err != nil {
		t.Fatalf("writeOutputAtomically() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "complete output" {
		t.Fatalf("output = %q, want %q", got, "complete output")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("temporary artifacts remain after success: %v", entries)
	}
}

func TestWriteBytecodeAtomicallyPreservesExistingFileOnValidationFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "program.abc.json")
	const original = "known-good bytecode"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	err := writeBytecodeAtomically(path, &bytecode.Program{})
	if err == nil {
		t.Fatal("expected invalid bytecode program to fail validation")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != original {
		t.Fatalf("existing bytecode changed after validation failure: got %q, want %q", got, original)
	}
}
