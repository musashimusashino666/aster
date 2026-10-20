package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFlagsBeforePositionals(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		flags []string
		want  []string
	}{
		{
			name:  "run flag after source",
			args:  []string{"examples/hello.ast", "--engine", "native"},
			flags: []string{"--engine"},
			want:  []string{"--engine", "native", "examples/hello.ast"},
		},
		{
			name:  "equals syntax after source",
			args:  []string{"examples/hello.ast", "--engine=native"},
			flags: []string{"--engine", "-engine"},
			want:  []string{"--engine=native", "examples/hello.ast"},
		},
		{
			name:  "single dash flag after source",
			args:  []string{"examples/hello.ast", "-engine", "native"},
			flags: []string{"--engine", "-engine"},
			want:  []string{"-engine", "native", "examples/hello.ast"},
		},
		{
			name:  "multiple build flags after source",
			args:  []string{"main.ast", "-o", "app", "--target", "native"},
			flags: []string{"-o", "--target"},
			want:  []string{"-o", "app", "--target", "native", "main.ast"},
		},
		{
			name:  "unrecognized arguments remain positional",
			args:  []string{"first.ast", "second.ast"},
			flags: []string{"--engine"},
			want:  []string{"first.ast", "second.ast"},
		},
		{
			name:  "delimiter preserves dash-prefixed filename",
			args:  []string{"--engine", "native", "--", "-draft.ast"},
			flags: []string{"--engine"},
			want:  []string{"--engine", "native", "--", "-draft.ast"},
		},
		{
			name:  "delimiter prevents reordering later flag-like arguments",
			args:  []string{"--", "-engine", "native"},
			flags: []string{"--engine", "-engine"},
			want:  []string{"--", "-engine", "native"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := flagsBeforePositionals(tt.args, tt.flags...); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("flagsBeforePositionals() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestWriteOutputAtomicallyReplacesSymlinkItself(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("symbolic link creation may require elevated permissions on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	output := filepath.Join(dir, "output.txt")
	if err := os.WriteFile(target, []byte("target contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, output); err != nil {
		t.Skipf("cannot create symbolic link in this environment: %v", err)
	}

	err := writeOutputAtomically(output, func(w io.Writer) error {
		_, err := io.WriteString(w, "new output")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(output)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("atomic output path is still a symbolic link")
	}
	gotOutput, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotOutput) != "new output" {
		t.Fatalf("output contents = %q, want %q", gotOutput, "new output")
	}
	gotTarget, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotTarget) != "target contents" {
		t.Fatalf("symlink target was changed: %q", gotTarget)
	}
}
