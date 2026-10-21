// Package toolchain handles invoking the host Go toolchain for Aster's native backend.
package toolchain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/musashimusashino666/aster/internal/diagnostic"
	"github.com/musashimusashino666/aster/internal/resource"
	"github.com/musashimusashino666/aster/internal/token"
)

const (
	// NativeBuildTimeout bounds native source generation and host-toolchain work
	// for a CLI build invocation.
	NativeBuildTimeout = 2 * time.Minute
	// NativeRunTimeout bounds native source generation, build, and execution
	// for a CLI native-run invocation.
	NativeRunTimeout = 5 * time.Minute

	nativeBuildTimeout = NativeBuildTimeout
	nativeRunTimeout   = NativeRunTimeout
)

// BuildNative formats and compiles goSource, atomically replacing output on
// success. The host Go toolchain is bounded by nativeBuildTimeout.
func BuildNative(goSource, output string) error {
	ctx, cancel := context.WithTimeout(context.Background(), nativeBuildTimeout)
	defer cancel()
	return BuildNativeContext(ctx, goSource, output)
}

// BuildNativeContext performs a native build using ctx to bound gofmt and go
// build. A canceled or expired build leaves the previous output untouched.
func BuildNativeContext(ctx context.Context, goSource, output string) (resultErr error) {
	defer func() { resultErr = diagnostic.WrapToolchain(resultErr) }()
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
func RunNativeContext(ctx context.Context, goSource string) (resultErr error) {
	defer func() { resultErr = diagnostic.WrapToolchain(resultErr) }()
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
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Env = environmentSliceWith(environmentWith("ASTER_RUNTIME_DIAGNOSTIC_PROTOCOL", "1"), "ASTER_RESOURCE_GATE", "")
	if err = cmd.Run(); err != nil {
		if ctx.Err() == nil {
			if runtimeDiagnostic := parseNativeRuntimeDiagnostic(stderr.Bytes()); runtimeDiagnostic != nil {
				return runtimeDiagnostic
			}
		}
		if stderr.Len() > 0 {
			_, _ = os.Stderr.Write(stderr.Bytes())
		}
		return commandFailure(ctx, "native program", err, nil)
	}
	if stderr.Len() > 0 {
		_, _ = os.Stderr.Write(stderr.Bytes())
	}
	return nil
}

// RunNativeConstrainedContext builds and runs a Native program inside an
// explicitly requested OS resource boundary. Unsupported platforms or missing
// cgroup v2 delegation fail before Aster code is released.
func RunNativeConstrainedContext(ctx context.Context, goSource string, limits resource.Limits) (resultErr error) {
	defer func() { resultErr = diagnostic.WrapToolchain(resultErr) }()
	if ctx == nil {
		return fmt.Errorf("native constrained run requires a non-nil context")
	}
	if err := limits.Validate(); err != nil {
		return fmt.Errorf("resource isolation unavailable: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("native run canceled: %w", err)
	}
	dir, err := os.MkdirTemp("", "aster-constrained-run-*")
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

	gatePath := filepath.Join(dir, "resource-release")
	launcherPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate Aster resource launcher: %w", err)
	}
	launcherPath, err = filepath.Abs(launcherPath)
	if err != nil {
		return fmt.Errorf("resolve Aster resource launcher: %w", err)
	}
	cmd := exec.Command(launcherPath, "__resource-launcher", bin)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Env = environmentSliceWith(environmentWith("ASTER_RUNTIME_DIAGNOSTIC_PROTOCOL", "1"), "ASTER_RESOURCE_GATE", gatePath)
	err = resource.Run(ctx, cmd, gatePath, limits)
	if err != nil {
		return nativeRunFailure(ctx, err, stderr.Bytes())
	}
	if stderr.Len() > 0 {
		_, _ = os.Stderr.Write(stderr.Bytes())
	}
	return nil
}

const nativeRuntimeDiagnosticPrefix = "\x1eASTER-RUNTIME-DIAGNOSTIC:"

type nativeRuntimeDiagnosticRecord struct {
	Kind             string `json:"kind"`
	Code             string `json:"code"`
	Source           string `json:"source"`
	Message          string `json:"message"`
	Offset           int    `json:"offset"`
	Line             int    `json:"line"`
	Column           int    `json:"column"`
	RangeStartOffset int    `json:"range_start_offset"`
	RangeStartLine   int    `json:"range_start_line"`
	RangeStartColumn int    `json:"range_start_column"`
	EndOffset        int    `json:"end_offset"`
	EndLine          int    `json:"end_line"`
	EndColumn        int    `json:"end_column"`
	HasSpan          bool   `json:"has_span"`
}

func parseNativeRuntimeDiagnostic(stderr []byte) error {
	for _, line := range strings.Split(string(stderr), "\n") {
		line = strings.TrimSuffix(line, "\r")
		payload, ok := strings.CutPrefix(line, nativeRuntimeDiagnosticPrefix)
		if !ok {
			continue
		}
		var record nativeRuntimeDiagnosticRecord
		if err := json.Unmarshal([]byte(payload), &record); err != nil ||
			record.Kind != "aster-runtime-diagnostic" || record.Code == "" {
			return nil
		}
		start := token.Position{Offset: record.Offset, Line: record.Line, Column: record.Column}
		if record.HasSpan {
			if record.Line <= 0 || record.Column <= 0 || record.EndLine <= 0 ||
				record.EndColumn <= 0 || record.Offset < 0 || record.EndOffset < record.Offset ||
				record.EndLine < record.Line ||
				(record.EndLine == record.Line && record.EndColumn < record.Column) {
				return nil
			}
			rangeStart := token.Position{
				Offset: record.RangeStartOffset, Line: record.RangeStartLine, Column: record.RangeStartColumn,
			}
			// Older structured records do not carry a distinct range start.
			if record.RangeStartLine == 0 && record.RangeStartColumn == 0 && record.RangeStartOffset == 0 {
				rangeStart = start
			}
			if rangeStart.Offset < 0 || rangeStart.Line <= 0 || rangeStart.Column <= 0 ||
				rangeStart.Offset > record.Offset || record.Offset > record.EndOffset ||
				(record.EndLine < rangeStart.Line ||
					(record.EndLine == rangeStart.Line && record.EndColumn < rangeStart.Column)) {
				return nil
			}
			end := token.Position{Offset: record.EndOffset, Line: record.EndLine, Column: record.EndColumn}
			return diagnostic.NewAtSourceRange(record.Code, "runtime", record.Source, start, rangeStart, end, record.Message)
		}
		return diagnostic.NewAtSource(record.Code, "runtime", record.Source, start, record.Message)
	}
	return nil
}

func environmentWith(key, value string) []string {
	return environmentSliceWith(os.Environ(), key, value)
}

func environmentSliceWith(base []string, key, value string) []string {
	prefix := key + "="
	env := make([]string, 0, len(base)+1)
	for _, entry := range base {
		if !strings.HasPrefix(entry, prefix) {
			env = append(env, entry)
		}
	}
	return append(env, prefix+value)
}

// nativeRunFailure prioritizes an expired external context over resource-limit
// classifications so RUN019 does not mask a concurrent TOOL002 timeout.
func nativeRunFailure(ctx context.Context, err error, stderr []byte) error {
	writeStderr := func() {
		if len(stderr) > 0 {
			_, _ = os.Stderr.Write(stderr)
		}
	}
	contextFailure := func() error {
		writeStderr()
		return commandFailure(ctx, "native program", err, nil)
	}

	if ctx.Err() != nil {
		return contextFailure()
	}
	if errors.Is(err, resource.ErrMemoryLimitExceeded) {
		if ctx.Err() != nil {
			return contextFailure()
		}
		return diagnostic.New(diagnostic.RuntimeProcessMemoryLimit, "runtime", token.Position{}, err.Error())
	}
	if errors.Is(err, resource.ErrCPUTimeLimitExceeded) {
		if ctx.Err() != nil {
			return contextFailure()
		}
		return diagnostic.New(diagnostic.RuntimeCPUTimeLimit, "runtime", token.Position{}, err.Error())
	}
	if ctx.Err() == nil {
		if runtimeDiagnostic := parseNativeRuntimeDiagnostic(stderr); runtimeDiagnostic != nil {
			return runtimeDiagnostic
		}
	}
	writeStderr()
	if ctx.Err() != nil {
		return commandFailure(ctx, "native program", err, nil)
	}
	return commandFailure(ctx, "native program", err, nil)
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
