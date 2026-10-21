//go:build linux

package resource

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestRunFailsClosedWhenCgroupRootIsNotCgroupV2(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ASTER_CGROUP_ROOT", root)
	marker := filepath.Join(t.TempDir(), "child-started")
	release := filepath.Join(t.TempDir(), "release")
	cmd := exec.Command("sh", "-c", "printf started > "+marker)
	err := Run(context.Background(), cmd, release, Limits{MemoryMaxBytes: 64 << 20})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Run() error = %v, want ErrUnavailable", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("child ran despite unavailable cgroup controls: stat err = %v", statErr)
	}
	if _, statErr := os.Stat(release); !os.IsNotExist(statErr) {
		t.Fatalf("release gate was created despite unavailable controls: stat err = %v", statErr)
	}
}

func TestCurrentCgroupPathIsAbsolute(t *testing.T) {
	path, err := currentUnifiedCgroupPath()
	if err != nil {
		t.Skipf("unified cgroup v2 path unavailable on this runner: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("current cgroup path %q is not absolute", path)
	}
}

func TestCgroupMetricParsers(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cpu.stat"), []byte("usage_usec 12345\nuser_usec 10000\nsystem_usec 2345\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := readCPUUsage(dir); err != nil || got != 12345 {
		t.Fatalf("readCPUUsage() = (%d, %v), want (12345, nil)", got, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "memory.events"), []byte("low 0\nhigh 0\nmax 2\noom 1\noom_kill 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := readOOMCount(dir); err != nil || got != 1 {
		t.Fatalf("readOOMCount() = (%d, %v), want (1, nil)", got, err)
	}
	if got, err := readOOMKillCount(dir); err != nil || got != 1 {
		t.Fatalf("readOOMKillCount() = (%d, %v), want (1, nil)", got, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cgroup.events"), []byte("populated 1\nfrozen 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := cgroupPopulated(dir); err != nil || !got {
		t.Fatalf("cgroupPopulated() = (%v, %v), want (true, nil)", got, err)
	}
}

func TestRunEnforcesCumulativeCPUTimeWhenDelegatedCgroupIsAvailable(t *testing.T) {
	limits := Limits{CPUTimeLimit: 50 * time.Millisecond}
	probe, err := createCgroup(limits)
	if err != nil {
		t.Skipf("Linux cgroup v2 CPU controller is not delegated to this test runner: %v", err)
	}
	if err := probe.cleanup(); err != nil {
		t.Fatalf("cleanup cgroup capability probe: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	gate := filepath.Join(t.TempDir(), "release")
	cmd := exec.Command("sh", "-c", `while [ ! -e "$ASTER_RESOURCE_GATE" ]; do :; done; while :; do :; done`)
	err = Run(ctx, cmd, gate, limits)
	if !errors.Is(err, ErrCPUTimeLimitExceeded) {
		t.Fatalf("Run() error = %v, want ErrCPUTimeLimitExceeded", err)
	}
}

func TestRunEnforcesMemoryLimitWhenDelegatedCgroupIsAvailable(t *testing.T) {
	limits := Limits{MemoryMaxBytes: 64 << 20}
	probe, err := createCgroup(limits)
	if err != nil {
		t.Skipf("Linux cgroup v2 memory controller is not delegated to this test runner: %v", err)
	}
	if err := probe.cleanup(); err != nil {
		t.Fatalf("cleanup cgroup capability probe: %v", err)
	}

	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	gate := filepath.Join(t.TempDir(), "release")
	script := `while [ ! -e "$ASTER_RESOURCE_GATE" ]; do :; done; exec "$ASTER_RESOURCE_TEST_BINARY" -test.run=^TestCgroupMemoryHelper$`
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = append(os.Environ(), "ASTER_RESOURCE_TEST_HELPER=memory", "ASTER_RESOURCE_TEST_BINARY="+binary)
	err = Run(ctx, cmd, gate, limits)
	if !errors.Is(err, ErrMemoryLimitExceeded) {
		t.Fatalf("Run() error = %v, want ErrMemoryLimitExceeded", err)
	}
}

// TestCgroupMemoryHelper is a child-process helper for the conditional cgroup
// integration test. Ordinary go test runs return immediately without the
// explicit helper environment variable.
func TestCgroupMemoryHelper(t *testing.T) {
	if os.Getenv("ASTER_RESOURCE_TEST_HELPER") != "memory" {
		return
	}
	const size = 128 << 20
	payload := make([]byte, size)
	var checksum uint64
	for offset := 0; offset < len(payload); offset += 4096 {
		payload[offset] = 1
		checksum += uint64(payload[offset])
	}
	if len(payload) != size || checksum == 0 {
		t.Fatal("unexpected memory test payload")
	}
	time.Sleep(5 * time.Second)
	if payload[0] != 1 {
		t.Fatal("memory test payload was not retained")
	}
}

func TestRunChecksCPUTimeBudgetWhenWorkloadExitsBetweenPolls(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cpu.stat"), []byte("usage_usec 2000\nuser_usec 2000\nsystem_usec 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	waitCh := make(chan error, 1)
	waitCh <- nil
	group := &workloadCgroup{path: dir}
	err := group.wait(context.Background(), waitCh, time.Millisecond)
	if !errors.Is(err, ErrCPUTimeLimitExceeded) {
		t.Fatalf("wait() error = %v, want ErrCPUTimeLimitExceeded when completed workload used 2ms against a 1ms budget", err)
	}
}
