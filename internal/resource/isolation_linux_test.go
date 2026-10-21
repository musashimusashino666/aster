//go:build linux

package resource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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
	if err := os.WriteFile(filepath.Join(dir, "memory.events.local"), []byte("low 0\nhigh 0\nmax 2\noom 1\noom_kill 1\noom_group_kill 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := readOOMGroupKillCount(dir); err != nil || got != 1 {
		t.Fatalf("readOOMGroupKillCount() = (%d, %v), want (1, nil)", got, err)
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
	root := requireDelegatedCgroup(t, limits)
	before := snapshotAsterCgroups(t, root)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	gate := filepath.Join(t.TempDir(), "release")
	cmd := exec.Command("sh", "-c", `while [ ! -e "$ASTER_RESOURCE_GATE" ]; do :; done; while :; do :; done`)
	err := Run(ctx, cmd, gate, limits)
	if !errors.Is(err, ErrCPUTimeLimitExceeded) {
		t.Fatalf("Run() error = %v, want ErrCPUTimeLimitExceeded", err)
	}
	var limitErr *cpuTimeLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("Run() error = %T, want a measured cpuTimeLimitError", err)
	}
	if limitErr.observed < limitErr.budget || limitErr.overshoot != limitErr.observed-limitErr.budget {
		t.Fatalf("invalid CPU-time measurement: budget=%s observed=%s overshoot=%s", limitErr.budget, limitErr.observed, limitErr.overshoot)
	}
	if limitErr.sample != "post-termination" {
		t.Fatalf("CPU-time measurement sample = %q, want post-termination sample", limitErr.sample)
	}
	t.Logf("cpu_time_measurement budget_usec=%d observed_usec=%d overshoot_usec=%d sample=%s cpu_poll_interval=%s",
		limitErr.budget.Microseconds(), limitErr.observed.Microseconds(), limitErr.overshoot.Microseconds(),
		limitErr.sample, cpuPollInterval)
	assertNoNewAsterCgroups(t, root, before)
}

func TestRunEnforcesMemoryLimitWhenDelegatedCgroupIsAvailable(t *testing.T) {
	limits := Limits{MemoryMaxBytes: 64 << 20}
	root := requireDelegatedCgroup(t, limits)
	before := snapshotAsterCgroups(t, root)

	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	gate := filepath.Join(t.TempDir(), "release")
	script := "while [ ! -e \"$ASTER_RESOURCE_GATE\" ]; do :; done; unset ASTER_RESOURCE_GATE; exec \"$ASTER_RESOURCE_TEST_BINARY\" -test.run=^TestCgroupMemoryHelper$"
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = append(os.Environ(), "ASTER_RESOURCE_TEST_HELPER=memory", "ASTER_RESOURCE_TEST_BINARY="+binary)
	err = Run(ctx, cmd, gate, limits)
	if !errors.Is(err, ErrMemoryLimitExceeded) {
		t.Fatalf("Run() error = %v, want ErrMemoryLimitExceeded", err)
	}
	assertNoNewAsterCgroups(t, root, before)
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
	files := map[string]string{
		"cpu.stat":      "usage_usec 2000\nuser_usec 2000\nsystem_usec 0\n",
		"cgroup.kill":   "0",
		"cgroup.events": "populated 0\nfrozen 0\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	waitCh := make(chan error, 1)
	waitCh <- nil
	group := &workloadCgroup{path: dir}
	err := group.wait(context.Background(), waitCh, time.Millisecond)
	if !errors.Is(err, ErrCPUTimeLimitExceeded) {
		t.Fatalf("wait() error = %v, want ErrCPUTimeLimitExceeded when completed workload used 2ms against a 1ms budget", err)
	}
	var limitErr *cpuTimeLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("wait() error = %T, want a measured cpuTimeLimitError", err)
	}
	if limitErr.budget != time.Millisecond || limitErr.observed != 2*time.Millisecond ||
		limitErr.overshoot != time.Millisecond || limitErr.sample != "post-termination" {
		t.Fatalf("CPU-time measurement = %+v, want budget=1ms observed=2ms overshoot=1ms sample=post-termination", limitErr)
	}
}

func TestCPUStatReadDoesNotMaskExpiredContext(t *testing.T) {
	dir := t.TempDir()
	killPath := filepath.Join(dir, "cgroup.kill")
	if err := os.WriteFile(killPath, []byte("0"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	group := &workloadCgroup{
		path: dir,
		cpuUsageReader: func(string) (uint64, error) {
			cancel()
			return 1_000, nil
		},
	}
	err := group.wait(ctx, make(chan error), time.Millisecond)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wait() error = %v, want context.Canceled when context expires during cpu.stat read", err)
	}
	if errors.Is(err, ErrCPUTimeLimitExceeded) {
		t.Fatalf("wait() misclassified read-time cancellation as a CPU limit: %v", err)
	}
	got, err := os.ReadFile(killPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "1" {
		t.Fatalf("cgroup.kill = %q, want %q after canceled context", got, "1")
	}
}

func TestCPUTimeLimitErrorReportsPostTerminationOvershoot(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"cgroup.kill":   "0",
		"cgroup.events": "populated 0\nfrozen 0\n",
		"cpu.stat":      "usage_usec 63000\nuser_usec 62000\nsystem_usec 1000\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	group := &workloadCgroup{path: dir}
	err := group.stopForCPUTimeLimit(context.Background(), 50*time.Millisecond, 52_000)
	if !errors.Is(err, ErrCPUTimeLimitExceeded) {
		t.Fatalf("stopForCPUTimeLimit() error = %v, want ErrCPUTimeLimitExceeded", err)
	}
	var limitErr *cpuTimeLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("stopForCPUTimeLimit() error = %T, want *cpuTimeLimitError", err)
	}
	if limitErr.budget != 50*time.Millisecond ||
		limitErr.observed != 63*time.Millisecond ||
		limitErr.overshoot != 13*time.Millisecond ||
		limitErr.sample != "post-termination" {
		t.Fatalf("measurement = %+v, want budget=50ms observed=63ms overshoot=13ms sample=post-termination", limitErr)
	}
	for _, expected := range []string{
		"budget_usec=50000",
		"observed_usec=63000",
		"overshoot_usec=13000",
		"sample=post-termination",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("error %q does not contain %q", err, expected)
		}
	}
}

func TestWorkloadExitDoesNotMaskExpiredContext(t *testing.T) {
	dir := t.TempDir()
	killPath := filepath.Join(dir, "cgroup.kill")
	if err := os.WriteFile(killPath, []byte("0"), 0o600); err != nil {
		t.Fatal(err)
	}

	group := &workloadCgroup{path: dir}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	waitCh := make(chan error, 1)
	waitCh <- nil

	// Both select cases are ready. Whichever case wins, a completed workload
	// must not mask an already-expired wrapper deadline.
	err := group.wait(ctx, waitCh, 0)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait() error = %v, want context.DeadlineExceeded", err)
	}
	got, err := os.ReadFile(killPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "1" {
		t.Fatalf("cgroup.kill = %q, want %q after expired context", got, "1")
	}
}

func TestWorkloadExitIncludesCPUUsedByDescendantsUntilGroupIsEmpty(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"cgroup.kill":   "0",
		"cgroup.events": "populated 1\nfrozen 0\n",
		"cpu.stat":      "usage_usec 49000\nuser_usec 48000\nsystem_usec 1000\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// Simulate a descendant consuming CPU after the leader exits and while the
	// supervisor is killing the cgroup. The final sample must be taken only
	// after cgroup.events reports populated=0.
	watcherDone := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			data, err := os.ReadFile(filepath.Join(dir, "cgroup.kill"))
			if err != nil {
				watcherDone <- err
				return
			}
			if strings.TrimSpace(string(data)) == "1" {
				if err := os.WriteFile(filepath.Join(dir, "cpu.stat"), []byte("usage_usec 60000\nuser_usec 59000\nsystem_usec 1000\n"), 0o600); err != nil {
					watcherDone <- err
					return
				}
				if err := os.WriteFile(filepath.Join(dir, "cgroup.events"), []byte("populated 0\nfrozen 0\n"), 0o600); err != nil {
					watcherDone <- err
					return
				}
				watcherDone <- nil
				return
			}
			time.Sleep(time.Millisecond)
		}
		watcherDone <- fmt.Errorf("timed out waiting for cgroup.kill")
	}()

	group := &workloadCgroup{path: dir}
	err := group.handleWorkloadExit(context.Background(), nil, 50*time.Millisecond)
	if watcherErr := <-watcherDone; watcherErr != nil {
		t.Fatal(watcherErr)
	}
	if !errors.Is(err, ErrCPUTimeLimitExceeded) {
		t.Fatalf("handleWorkloadExit() error = %v, want ErrCPUTimeLimitExceeded after descendant CPU use", err)
	}
	var limitErr *cpuTimeLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("handleWorkloadExit() error = %T, want *cpuTimeLimitError", err)
	}
	if limitErr.observed != 60*time.Millisecond || limitErr.overshoot != 10*time.Millisecond || limitErr.sample != "post-termination" {
		t.Fatalf("final CPU-time measurement = %+v, want 60ms observed, 10ms overshoot, post-termination sample", limitErr)
	}
}

func TestMemoryLimitClassificationRequiresLocalOOMGroupKillCounter(t *testing.T) {
	tests := []struct {
		name                 string
		initialGroupOOMKills uint64
		currentGroupOOMKills uint64
		currentOOM           uint64
		currentOOMKill       uint64
		wantMemoryLimit      bool
	}{
		{name: "local group OOM increases", initialGroupOOMKills: 4, currentGroupOOMKills: 5, currentOOM: 1, currentOOMKill: 1, wantMemoryLimit: true},
		{name: "oom and oom_kill increase without group OOM", initialGroupOOMKills: 4, currentGroupOOMKills: 4, currentOOM: 2, currentOOMKill: 1},
		{name: "oom only", initialGroupOOMKills: 4, currentGroupOOMKills: 4, currentOOM: 1, currentOOMKill: 0},
		{name: "oom_kill only", initialGroupOOMKills: 4, currentGroupOOMKills: 4, currentOOM: 0, currentOOMKill: 1},
		{name: "neither increases", initialGroupOOMKills: 4, currentGroupOOMKills: 4, currentOOM: 0, currentOOMKill: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			files := map[string]string{
				"cgroup.kill":         "0",
				"cgroup.events":       "populated 0\nfrozen 0\n",
				"memory.events":       fmt.Sprintf("low 0\nhigh 0\nmax 0\noom %d\noom_kill %d\n", tt.currentOOM, tt.currentOOMKill),
				"memory.events.local": fmt.Sprintf("low 0\nhigh 0\nmax 0\noom %d\noom_kill %d\noom_group_kill %d\n", tt.currentOOM, tt.currentOOMKill, tt.currentGroupOOMKills),
			}
			for name, content := range files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}
			group := &workloadCgroup{
				path:                 dir,
				limits:               Limits{MemoryMaxBytes: 64 << 20},
				initialOOMGroupKills: tt.initialGroupOOMKills,
			}
			err := group.handleWorkloadExit(context.Background(), nil, 0)
			gotMemoryLimit := errors.Is(err, ErrMemoryLimitExceeded)
			if gotMemoryLimit != tt.wantMemoryLimit {
				t.Fatalf("handleWorkloadExit() memory-limit classification = %v, want %v (error %v)", gotMemoryLimit, tt.wantMemoryLimit, err)
			}
		})
	}
}

func TestReadOOMGroupKillCountRequiresLocalCounter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.events.local")
	if err := os.WriteFile(path, []byte("low 0\nhigh 0\noom 0\noom_kill 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readOOMGroupKillCount(dir); err == nil || !strings.Contains(err.Error(), "oom_group_kill") {
		t.Fatalf("readOOMGroupKillCount() error = %v, want missing oom_group_kill counter error", err)
	}
}

const (
	resourceTestModeEnv   = "ASTER_RESOURCE_TEST_MODE"
	resourceTestReportEnv = "ASTER_RESOURCE_TEST_REPORT"
	resourceTestBinaryEnv = "ASTER_RESOURCE_TEST_BINARY"
	resourceTestNameEnv   = "ASTER_RESOURCE_TEST_NAME"
	resourceTestChildPID  = "ASTER_RESOURCE_TEST_CHILD_PID"
)

type cgroupIntegrationReport struct {
	CgroupPath   string `json:"cgroup_path"`
	CPUmax       string `json:"cpu_max,omitempty"`
	MemoryMax    string `json:"memory_max,omitempty"`
	SwapMax      string `json:"memory_swap_max,omitempty"`
	OOMGroup     string `json:"memory_oom_group,omitempty"`
	CPUStat      string `json:"cpu_stat,omitempty"`
	WallNanos    int64  `json:"wall_nanos,omitempty"`
	ProcessNanos int64  `json:"process_cpu_nanos,omitempty"`
}

// requireDelegatedCgroup skips with an explicit reason unless this runner can
// create, configure, and remove a real delegated cgroup for the requested
// controls. A skip is a capability result, not evidence of enforcement.
func requireDelegatedCgroup(t *testing.T, limits Limits) string {
	t.Helper()
	root, err := delegatedCgroupRoot()
	if err != nil {
		t.Skipf("delegated cgroup v2 root unavailable: %v", err)
	}
	t.Setenv("ASTER_CGROUP_ROOT", root)
	probe, err := createCgroup(limits)
	if err != nil {
		t.Skipf("required cgroup v2 controllers are not delegated: %v", err)
	}
	if err := probe.cleanup(); err != nil {
		t.Fatalf("clean up cgroup capability probe: %v", err)
	}
	kernel, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		t.Fatalf("read Linux kernel release: %v", err)
	}
	controllers, err := os.ReadFile(filepath.Join(root, "cgroup.controllers"))
	if err != nil {
		t.Fatalf("read delegated cgroup controllers: %v", err)
	}
	subtreeControl, err := os.ReadFile(filepath.Join(root, "cgroup.subtree_control"))
	if err != nil {
		t.Fatalf("read enabled child cgroup controllers: %v", err)
	}
	t.Logf("cgroup preflight: kernel=%q root=%q controllers=%q subtree_control=%q cpu_poll_interval=%s limits=%+v",
		strings.TrimSpace(string(kernel)), root, strings.TrimSpace(string(controllers)),
		strings.TrimSpace(string(subtreeControl)), cpuPollInterval, limits)
	return root
}

func snapshotAsterCgroups(t *testing.T, root string) map[string]struct{} {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read delegated cgroup root %s: %v", root, err)
	}
	found := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "aster-run-") {
			found[entry.Name()] = struct{}{}
		}
	}
	return found
}

func assertNoNewAsterCgroups(t *testing.T, root string, before map[string]struct{}) {
	t.Helper()
	after := snapshotAsterCgroups(t, root)
	for name := range after {
		if _, existed := before[name]; !existed {
			t.Errorf("temporary workload cgroup remains after Run: %s", filepath.Join(root, name))
		}
	}
}

func newGatedTestCommand(t *testing.T, testName, mode, reportPath string) *exec.Cmd {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Do not spawn helper children while waiting for the release gate. Once the
	// supervisor releases the gate, exec the Go test helper in the same PID.
	script := `while [ ! -e "$ASTER_RESOURCE_GATE" ]; do :; done; unset ASTER_RESOURCE_GATE; exec "$ASTER_RESOURCE_TEST_BINARY" -test.run="$ASTER_RESOURCE_TEST_NAME"`
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = append(os.Environ(),
		resourceTestModeEnv+"="+mode,
		resourceTestReportEnv+"="+reportPath,
		resourceTestBinaryEnv+"="+binary,
		resourceTestNameEnv+"="+testName,
	)
	return cmd
}

func readCgroupControlForCurrentProcess(name string) (string, error) {
	procPath, err := currentUnifiedCgroupPath()
	if err != nil {
		return "", err
	}
	mount, mountRoot, err := cgroup2Mount()
	if err != nil {
		return "", err
	}
	relative := procPath
	if mountRoot != "/" && strings.HasPrefix(procPath, strings.TrimSuffix(mountRoot, "/")+"/") {
		relative = strings.TrimPrefix(procPath, mountRoot)
	} else if procPath == mountRoot {
		relative = "/"
	}
	dir := filepath.Clean(filepath.Join(mount, strings.TrimPrefix(relative, "/")))
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func writeCgroupIntegrationReport(report cgroupIntegrationReport) error {
	path := os.Getenv(resourceTestReportEnv)
	if path == "" {
		return fmt.Errorf("%s is not set", resourceTestReportEnv)
	}
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// TestCgroupIntegrationHelper is invoked only as a child process by the
// conditional integration tests. Ordinary go test runs return immediately.
func TestCgroupIntegrationHelper(t *testing.T) {
	mode := os.Getenv(resourceTestModeEnv)
	if mode == "" {
		return
	}
	procPath, err := currentUnifiedCgroupPath()
	if err != nil {
		t.Fatalf("read workload cgroup membership: %v", err)
	}
	report := cgroupIntegrationReport{CgroupPath: procPath}
	if report.CPUmax, err = readCgroupControlForCurrentProcess("cpu.max"); err != nil && mode != "memory-inspect" {
		t.Fatalf("read workload cpu.max: %v", err)
	}
	if mode == "cpu-inspect" {
		if err := writeCgroupIntegrationReport(report); err != nil {
			t.Fatal(err)
		}
		return
	}
	if mode == "memory-inspect" {
		if report.MemoryMax, err = readCgroupControlForCurrentProcess("memory.max"); err != nil {
			t.Fatalf("read workload memory.max: %v", err)
		}
		if report.SwapMax, err = readCgroupControlForCurrentProcess("memory.swap.max"); err != nil {
			t.Fatalf("read workload memory.swap.max: %v", err)
		}
		if report.OOMGroup, err = readCgroupControlForCurrentProcess("memory.oom.group"); err != nil {
			t.Fatalf("read workload memory.oom.group: %v", err)
		}
		if err := writeCgroupIntegrationReport(report); err != nil {
			t.Fatal(err)
		}
		return
	}
	if mode == "cpu-throttle" {
		start := time.Now()
		var work uint64
		for time.Since(start) < 400*time.Millisecond {
			work++
		}
		report.WallNanos = time.Since(start).Nanoseconds()
		var usage syscall.Rusage
		if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
			t.Fatalf("get process CPU usage: %v", err)
		}
		report.ProcessNanos = int64(usage.Utime.Sec+usage.Stime.Sec)*int64(time.Second) +
			int64(usage.Utime.Usec+usage.Stime.Usec)*int64(time.Microsecond)
		if report.CPUStat, err = readCgroupControlForCurrentProcess("cpu.stat"); err != nil {
			t.Fatalf("read workload cpu.stat: %v", err)
		}
		if work == 0 {
			t.Fatal("CPU workload did not execute")
		}
		if err := writeCgroupIntegrationReport(report); err != nil {
			t.Fatal(err)
		}
		return
	}
	if mode == "process-tree" {
		child := exec.Command("sh", "-c", "while :; do :; done")
		if err := child.Start(); err != nil {
			t.Fatalf("start descendant process: %v", err)
		}
		pidPath := os.Getenv(resourceTestChildPID)
		if pidPath == "" {
			t.Fatal("child PID report path is not set")
		}
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			t.Fatalf("record descendant PID: %v", err)
		}
		for {
			// Deliberately consume CPU until the supervisor kills the complete
			// cgroup or the wrapper context expires.
		}
	}
	t.Fatalf("unknown resource integration helper mode %q", mode)
}

func readIntegrationReport(t *testing.T, path string) cgroupIntegrationReport {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read integration report: %v", err)
	}
	var report cgroupIntegrationReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode integration report: %v", err)
	}
	return report
}

func readCPUStatValue(t *testing.T, raw, key string) uint64 {
	t.Helper()
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == key {
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				t.Fatalf("parse cpu.stat %s value %q: %v", key, fields[1], err)
			}
			return value
		}
	}
	t.Fatalf("cpu.stat does not contain %q: %q", key, raw)
	return 0
}

func assertProcessNotRunning(t *testing.T, pid int) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatalf("inspect descendant process %d: %v", pid, err)
	}
	endComm := strings.LastIndex(string(data), ")")
	if endComm < 0 {
		t.Fatalf("malformed /proc/%d/stat: %q", pid, data)
	}
	fields := strings.Fields(string(data)[endComm+1:])
	if len(fields) == 0 {
		t.Fatalf("missing process state in /proc/%d/stat", pid)
	}
	if fields[0] != "Z" && fields[0] != "X" {
		t.Fatalf("descendant process %d is still running (state %s)", pid, fields[0])
	}
}

func TestRunConfiguresCPUQuotaBeforeWorkloadAndCleansUp(t *testing.T) {
	limits := Limits{CPUQuota: 0.5}
	root := requireDelegatedCgroup(t, limits)
	before := snapshotAsterCgroups(t, root)
	reportPath := filepath.Join(t.TempDir(), "report.json")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := newGatedTestCommand(t, "TestCgroupIntegrationHelper", "cpu-inspect", reportPath)
	// The helper reads /proc/self/cgroup and cpu.max only after the gate lets
	// it exec; membership must already be inside the temporary workload group.
	err := Run(ctx, cmd, filepath.Join(t.TempDir(), "release"), limits)
	if err != nil {
		t.Fatalf("Run() error = %v, want normal exit", err)
	}
	report := readIntegrationReport(t, reportPath)
	if filepath.Base(report.CgroupPath) == "" || !strings.HasPrefix(filepath.Base(report.CgroupPath), "aster-run-") {
		t.Fatalf("workload began outside its temporary cgroup: %q", report.CgroupPath)
	}
	if report.CPUmax != "50000 100000" {
		t.Fatalf("workload cpu.max = %q, want %q", report.CPUmax, "50000 100000")
	}
	assertNoNewAsterCgroups(t, root, before)
	t.Logf("membership=%s cpu.max=%s cleanup=clean", report.CgroupPath, report.CPUmax)
}

func TestRunCPUQuotaThrottlesAndReportsKernelMetrics(t *testing.T) {
	limits := Limits{CPUQuota: 0.1}
	root := requireDelegatedCgroup(t, limits)
	before := snapshotAsterCgroups(t, root)
	reportPath := filepath.Join(t.TempDir(), "cpu-report.json")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := newGatedTestCommand(t, "TestCgroupIntegrationHelper", "cpu-throttle", reportPath)
	if err := Run(ctx, cmd, filepath.Join(t.TempDir(), "release"), limits); err != nil {
		t.Fatalf("Run() error = %v, want normal completion of measured workload", err)
	}
	report := readIntegrationReport(t, reportPath)
	if report.CPUmax != "10000 100000" {
		t.Fatalf("workload cpu.max = %q, want %q", report.CPUmax, "10000 100000")
	}
	if report.WallNanos < int64(350*time.Millisecond) || report.ProcessNanos <= 0 {
		t.Fatalf("invalid CPU-throttle measurements: wall=%s process CPU=%s", time.Duration(report.WallNanos), time.Duration(report.ProcessNanos))
	}
	if report.ProcessNanos*10 >= report.WallNanos*7 {
		t.Fatalf("CPU quota did not sufficiently throttle workload: wall=%s process CPU=%s", time.Duration(report.WallNanos), time.Duration(report.ProcessNanos))
	}
	if got := readCPUStatValue(t, report.CPUStat, "nr_throttled"); got == 0 {
		t.Fatalf("cpu.stat nr_throttled = 0; expected kernel throttling, stats=%q", report.CPUStat)
	}
	if got := readCPUStatValue(t, report.CPUStat, "throttled_usec"); got == 0 {
		t.Fatalf("cpu.stat throttled_usec = 0; expected kernel throttling, stats=%q", report.CPUStat)
	}
	assertNoNewAsterCgroups(t, root, before)
	t.Logf("cpu.max=%s wall=%s process_cpu=%s cpu.stat=%q cleanup=clean", report.CPUmax, time.Duration(report.WallNanos), time.Duration(report.ProcessNanos), report.CPUStat)
}

func TestRunConfiguresMemoryControlsAndCleansUp(t *testing.T) {
	limits := Limits{MemoryMaxBytes: 128 << 20}
	root := requireDelegatedCgroup(t, limits)
	before := snapshotAsterCgroups(t, root)
	reportPath := filepath.Join(t.TempDir(), "memory-report.json")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := newGatedTestCommand(t, "TestCgroupIntegrationHelper", "memory-inspect", reportPath)
	if err := Run(ctx, cmd, filepath.Join(t.TempDir(), "release"), limits); err != nil {
		t.Fatalf("Run() error = %v, want normal exit", err)
	}
	report := readIntegrationReport(t, reportPath)
	if !strings.HasPrefix(filepath.Base(report.CgroupPath), "aster-run-") {
		t.Fatalf("workload began outside its temporary cgroup: %q", report.CgroupPath)
	}
	if report.MemoryMax != strconv.FormatInt(limits.MemoryMaxBytes, 10) {
		t.Fatalf("memory.max = %q, want %d", report.MemoryMax, limits.MemoryMaxBytes)
	}
	if report.SwapMax != "0" {
		t.Fatalf("memory.swap.max = %q, want 0", report.SwapMax)
	}
	if report.OOMGroup != "1" {
		t.Fatalf("memory.oom.group = %q, want 1", report.OOMGroup)
	}
	assertNoNewAsterCgroups(t, root, before)
	t.Logf("membership=%s memory.max=%s memory.swap.max=%s memory.oom.group=%s cleanup=clean", report.CgroupPath, report.MemoryMax, report.SwapMax, report.OOMGroup)
}

func TestRunKillsDescendantsAndCleansUpOnCPUTimeLimit(t *testing.T) {
	limits := Limits{CPUTimeLimit: 100 * time.Millisecond}
	root := requireDelegatedCgroup(t, limits)
	before := snapshotAsterCgroups(t, root)
	reportPath := filepath.Join(t.TempDir(), "unused-report.json")
	pidPath := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := newGatedTestCommand(t, "TestCgroupIntegrationHelper", "process-tree", reportPath)
	cmd.Env = append(cmd.Env, resourceTestChildPID+"="+pidPath)
	err := Run(ctx, cmd, filepath.Join(t.TempDir(), "release"), limits)
	if !errors.Is(err, ErrCPUTimeLimitExceeded) {
		t.Fatalf("Run() error = %v, want ErrCPUTimeLimitExceeded", err)
	}
	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("read descendant PID (helper may not have started): %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse descendant PID %q: %v", data, err)
	}
	assertProcessNotRunning(t, pid)
	assertNoNewAsterCgroups(t, root, before)
	t.Logf("termination=CPU-time-limit child_pid=%d child_stopped=true cleanup=clean", pid)
}

func TestRunKillsDescendantsAndCleansUpOnContextDeadline(t *testing.T) {
	limits := Limits{CPUQuota: 1.0}
	root := requireDelegatedCgroup(t, limits)
	before := snapshotAsterCgroups(t, root)
	reportPath := filepath.Join(t.TempDir(), "unused-report.json")
	pidPath := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := newGatedTestCommand(t, "TestCgroupIntegrationHelper", "process-tree", reportPath)
	cmd.Env = append(cmd.Env, resourceTestChildPID+"="+pidPath)
	err := Run(ctx, cmd, filepath.Join(t.TempDir(), "release"), limits)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context.DeadlineExceeded", err)
	}
	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("read descendant PID (helper may not have started): %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse descendant PID %q: %v", data, err)
	}
	assertProcessNotRunning(t, pid)
	assertNoNewAsterCgroups(t, root, before)
	t.Logf("termination=context-deadline child_pid=%d child_stopped=true cleanup=clean", pid)
}
