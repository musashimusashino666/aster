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
	goPath, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("native backend requires Go 1.23 or newer; executable %q was not found in PATH", "go")
	}
	gofmtPath, err := exec.LookPath("gofmt")
	if err != nil {
		return fmt.Errorf("native backend requires gofmt (normally installed with Go 1.23 or newer)")
	}
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
	fmtcmd := exec.Command(gofmtPath, "-w", "main.go")
	fmtcmd.Dir = dir
	if out, err := fmtcmd.CombinedOutput(); err != nil {
		return fmt.Errorf("format generated code: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	outputDir, err := os.MkdirTemp(filepath.Dir(abs), "."+filepath.Base(abs)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary native output directory: %w", err)
	}
	defer os.RemoveAll(outputDir)
	tempOutput := filepath.Join(outputDir, filepath.Base(abs))

	cmd := exec.Command(goPath, "build", "-trimpath", "-ldflags=-s -w", "-o", tempOutput, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("native build failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	if err := os.Rename(tempOutput, abs); err != nil {
		return fmt.Errorf("replace native output %s: %w", abs, err)
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
