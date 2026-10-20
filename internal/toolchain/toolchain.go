// Package toolchain handles invoking the host Go toolchain for Aster's native backend.
package toolchain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const nativeBuildTimeout = 2 * time.Minute
const nativeRunTimeout = 5 * time.Minute

// BuildNative formats and compiles goSource, atomically replacing output on
// success. The host Go toolchain is bounded by nativeBuildTimeout.
func BuildNative(goSource, output string) error {
	ctx, cancel := context.WithTimeout(context.Background(), nativeBuildTimeout)
	defer cancel()
	return BuildNativeContext(ctx, goSource, output)
}

// BuildNativeContext performs a native build using ctx to bound gofmt and go
// build. A canceled or expired build leaves the previous output untouched.
func BuildNativeContext(ctx context.Context, goSource, output string) error {
	if ctx == nil {
		return fmt.Errorf("native build requires a non-nil context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("native build canceled: %w", err)
	}
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
	fmtcmd := exec.CommandContext(ctx, gofmtPath, "-w", "main.go")
	fmtcmd.Dir = dir
	if out, err := fmtcmd.CombinedOutput(); err != nil {
		return commandFailure(ctx, "format generated code", err, out)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("native build canceled: %w", err)
	}
	outputDir, err := os.MkdirTemp(filepath.Dir(abs), "."+filepath.Base(abs)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary native output directory: %w", err)
	}
	defer os.RemoveAll(outputDir)
	tempOutput := filepath.Join(outputDir, filepath.Base(abs))

	cmd := exec.CommandContext(ctx, goPath, "build", "-trimpath", "-ldflags=-s -w", "-o", tempOutput, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		return commandFailure(ctx, "native build", err, out)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("native build canceled: %w", err)
	}
	if err := os.Rename(tempOutput, abs); err != nil {
		return fmt.Errorf("replace native output %s: %w", abs, err)
	}
	return nil
}

// RunNative builds and executes goSource with a five-minute total deadline.
// Native programs execute with host privileges and are not sandboxed.
func RunNative(goSource string) error {
	ctx, cancel := context.WithTimeout(context.Background(), nativeRunTimeout)
	defer cancel()
	return RunNativeContext(ctx, goSource)
}

// RunNativeContext bounds both the native build and executable run with ctx.
func RunNativeContext(ctx context.Context, goSource string) error {
	if ctx == nil {
		return fmt.Errorf("native run requires a non-nil context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("native run canceled: %w", err)
	}
	dir, err := os.MkdirTemp("", "aster-run-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	bin := filepath.Join(dir, "aster-program")
	if os.PathSeparator == '\\' {
		bin += ".exe"
	}
	if err = BuildNativeContext(ctx, goSource, bin); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		return commandFailure(ctx, "native program", err, nil)
	}
	return nil
}

func commandFailure(ctx context.Context, operation string, err error, output []byte) error {
	message := strings.TrimSpace(string(output))
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		if message == "" {
			return fmt.Errorf("%s timed out: %w", operation, ctx.Err())
		}
		return fmt.Errorf("%s timed out: %w\n%s", operation, ctx.Err(), message)
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		if message == "" {
			return fmt.Errorf("%s canceled: %w", operation, ctx.Err())
		}
		return fmt.Errorf("%s canceled: %w\n%s", operation, ctx.Err(), message)
	}
	if message == "" {
		return fmt.Errorf("%s failed: %w", operation, err)
	}
	return fmt.Errorf("%s failed: %w\n%s", operation, err, message)
}
