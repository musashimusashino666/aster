package toolchain

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/musashimusashino666/aster/internal/diagnostic"
	"github.com/musashimusashino666/aster/internal/resource"
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

func TestBuildNativeContextHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	output := filepath.Join(t.TempDir(), "aster-canceled")
	err := BuildNativeContext(ctx, "package main\nfunc main() {}\n", output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BuildNativeContext() error = %v, want context.Canceled", err)
	}
	if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
		t.Fatalf("canceled build created output: stat error = %v", statErr)
	}
}

func TestNativeRunFailurePrefersExpiredContextOverCPUTimeLimit(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	err := nativeRunFailure(ctx, resource.ErrCPUTimeLimitExceeded, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("nativeRunFailure() error = %v, want context.DeadlineExceeded over CPU limit", err)
	}
	wrapped := diagnostic.WrapToolchain(err)
	if !strings.Contains(wrapped.Error(), "[TOOL002]") {
		t.Fatalf("expected TOOL002 for expired context, got %v", wrapped)
	}
	if strings.Contains(wrapped.Error(), "[RUN019]") {
		t.Fatalf("expired context was misclassified as RUN019: %v", wrapped)
	}
}

func TestRunNativeContextStopsLongRunningProgram(t *testing.T) {
	requireNativeToolchain(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := RunNativeContext(ctx, "package main\nfunc main() { for {} }\n")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RunNativeContext() error = %v, want context.DeadlineExceeded", err)
	}
	if !strings.Contains(err.Error(), "native program timed out") || !strings.Contains(err.Error(), "[TOOL002]") {
		t.Fatalf("expected coded native program timeout diagnostic, got %v", err)
	}
}

func TestRunNativeConstrainedFailsClosedWithoutCgroupV2(t *testing.T) {
	requireNativeToolchain(t)
	t.Setenv("ASTER_CGROUP_ROOT", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	const source = "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"must not execute\") }\n"
	err := RunNativeConstrainedContext(ctx, source, resource.Limits{MemoryMaxBytes: 64 << 20})
	if err == nil {
		t.Fatal("RunNativeConstrainedContext() succeeded without cgroup v2 controls")
	}
	if !strings.Contains(err.Error(), "[TOOL008]") {
		t.Fatalf("expected TOOL008 fail-closed diagnostic, got %v", err)
	}
}
