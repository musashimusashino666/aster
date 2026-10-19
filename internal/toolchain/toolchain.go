// Package toolchain handles invoking the host Go toolchain for Aster's native backend.
package toolchain

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func BuildNative(goSource, output string) error {
	abs, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	dir, err := os.MkdirTemp("", "aster-native-build-*")
	if err != nil {
		return fmt.Errorf("create build directory: %w", err)
	}
	defer os.RemoveAll(dir)
	if err = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module aster-native-build\n\ngo 1.23\n"), 0o644); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "main.go"), []byte(goSource), 0o644); err != nil {
		return err
	}
	fmtcmd := exec.Command("gofmt", "-w", "main.go")
	fmtcmd.Dir = dir
	if out, err := fmtcmd.CombinedOutput(); err != nil {
		return fmt.Errorf("format generated code: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", abs, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("native build failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
func RunNative(goSource string) error {
	dir, err := os.MkdirTemp("", "aster-run-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	bin := filepath.Join(dir, "aster-program")
	if os.PathSeparator == '\\' {
		bin += ".exe"
	}
	if err = BuildNative(goSource, bin); err != nil {
		return err
	}
	cmd := exec.Command(bin)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("native program failed: %w", err)
	}
	return nil
}
