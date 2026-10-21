//go:build linux

package resource

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	cgroup2SuperMagic      = 0x63677270
	defaultCPUCgroupPeriod = 100000
	cpuPollInterval        = 10 * time.Millisecond
	cgroupDrainTimeout     = 2 * time.Second
	gateEnvironment        = "ASTER_RESOURCE_GATE"
)

// Run starts a gated internal launcher, attaches it to a dedicated cgroup v2,
// and only then releases it to exec the Native program. releasePath must
// name a private, initially absent file in a directory owned by the caller.
func Run(ctx context.Context, cmd *exec.Cmd, releasePath string, limits Limits) error {
	if ctx == nil {
		return fmt.Errorf("resource supervisor requires a non-nil context")
	}
	if cmd == nil {
		return fmt.Errorf("resource supervisor requires a command")
	}
	if err := limits.Validate(); err != nil {
		return err
	}
	cgroup, err := createCgroup(limits)
	if err != nil {
		return err
	}

	if _, err := os.Lstat(releasePath); err == nil {
		_ = cgroup.cleanup()
		return fmt.Errorf("%w: release gate already exists", ErrUnavailable)
	} else if !errors.Is(err, os.ErrNotExist) {
		_ = cgroup.cleanup()
		return fmt.Errorf("%w: inspect release gate: %v", ErrUnavailable, err)
	}
	cmd.Env = replaceEnv(cmd.Env, gateEnvironment, releasePath)
	if err := ctx.Err(); err != nil {
		_ = cgroup.cleanup()
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = cgroup.cleanup()
		return err
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	if err := cgroup.attach(cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		<-waitCh
		_ = cgroup.cleanup()
		return err
	}
	if err := ctx.Err(); err != nil {
		_ = cgroup.kill()
		_ = cmd.Process.Kill()
		<-waitCh
		_ = cgroup.cleanup()
		return err
	}
	if err := os.WriteFile(releasePath, []byte("release\n"), 0o600); err != nil {
		_ = cgroup.kill()
		_ = cmd.Process.Kill()
		<-waitCh
		_ = cgroup.cleanup()
		return fmt.Errorf("%w: release startup gate: %v", ErrUnavailable, err)
	}

	result := cgroup.wait(ctx, waitCh, limits.CPUTimeLimit)
	cleanupErr := cgroup.cleanup()
	if cleanupErr != nil {
		return fmt.Errorf("%w: cgroup cleanup failed: %v (workload result: %v)", ErrUnavailable, cleanupErr, result)
	}
	return result
}

type workloadCgroup struct {
	path            string
	limits          Limits
	initialOOM      uint64
	initialOOMKills uint64
}

type cpuTimeLimitError struct {
	budget   time.Duration
	observed time.Duration
	overshoot time.Duration
	sample   string
}

func newCPUTimeLimitError(budget time.Duration, observedUsec uint64, sample string) *cpuTimeLimitError {
	// cpu.stat reports aggregate usage in whole microseconds. Normalize the
	// budget to the same unit before calculating and reporting the overshoot.
	budget = time.Duration(budget.Microseconds()) * time.Microsecond
	observed := time.Duration(observedUsec) * time.Microsecond
	overshoot := observed - budget
	if overshoot < 0 {
		overshoot = 0
	}
	return &cpuTimeLimitError{
		budget: budget, observed: observed, overshoot: overshoot, sample: sample,
	}
}

func (err *cpuTimeLimitError) Error() string {
	return fmt.Sprintf("%s (budget_usec=%d observed_usec=%d overshoot_usec=%d sample=%s)",
		ErrCPUTimeLimitExceeded, err.budget.Microseconds(), err.observed.Microseconds(),
		err.overshoot.Microseconds(), err.sample)
}

func (err *cpuTimeLimitError) Unwrap() error {
	return ErrCPUTimeLimitExceeded
}

func createCgroup(limits Limits) (*workloadCgroup, error) {
	base, err := delegatedCgroupRoot()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(base, &stat); err != nil || uint64(stat.Type) != cgroup2SuperMagic {
		return nil, fmt.Errorf("%w: %s is not a cgroup v2 filesystem", ErrUnavailable, base)
	}
	available, err := readWordSet(filepath.Join(base, "cgroup.controllers"))
	if err != nil {
		return nil, fmt.Errorf("%w: read available controllers: %v", ErrUnavailable, err)
	}
	active, err := readWordSet(filepath.Join(base, "cgroup.subtree_control"))
	if err != nil {
		return nil, fmt.Errorf("%w: read enabled controllers: %v", ErrUnavailable, err)
	}
	if limits.MemoryMaxBytes > 0 {
		if !available["memory"] || !active["memory"] {
			return nil, fmt.Errorf("%w: cgroup v2 memory controller is not enabled for child cgroups", ErrUnavailable)
		}
	}
	if limits.CPUQuota > 0 || limits.CPUTimeLimit > 0 {
		if !available["cpu"] || !active["cpu"] {
			return nil, fmt.Errorf("%w: cgroup v2 CPU controller is not enabled for child cgroups", ErrUnavailable)
		}
	}

	path, err := os.MkdirTemp(base, "aster-run-")
	if err != nil {
		return nil, fmt.Errorf("%w: create delegated cgroup below %s: %v", ErrUnavailable, base, err)
	}
	group := &workloadCgroup{path: path, limits: limits}
	failed := true
	defer func() {
		if failed {
			_ = group.cleanup()
		}
	}()

	killControl := filepath.Join(path, "cgroup.kill")
	if _, err := os.Stat(killControl); err != nil {
		return nil, fmt.Errorf("%w: cgroup.kill is required to terminate the complete process group", ErrUnavailable)
	}
	if err := writeControl(killControl, "1"); err != nil {
		return nil, fmt.Errorf("%w: cgroup.kill is not writable: %v", ErrUnavailable, err)
	}
	if limits.MemoryMaxBytes > 0 {
		if err := writeControl(filepath.Join(path, "memory.max"), strconv.FormatInt(limits.MemoryMaxBytes, 10)); err != nil {
			return nil, fmt.Errorf("%w: configure memory.max: %v", ErrUnavailable, err)
		}
		if err := writeControl(filepath.Join(path, "memory.swap.max"), "0"); err != nil {
			return nil, fmt.Errorf("%w: configure memory.swap.max to disable swap for the workload: %v", ErrUnavailable, err)
		}
		if err := writeControl(filepath.Join(path, "memory.oom.group"), "1"); err != nil {
			return nil, fmt.Errorf("%w: configure memory.oom.group: %v", ErrUnavailable, err)
		}
		group.initialOOM, err = readOOMCount(path)
		if err != nil {
			return nil, fmt.Errorf("%w: read memory.events oom counter: %v", ErrUnavailable, err)
		}
		group.initialOOMKills, err = readOOMKillCount(path)
		if err != nil {
			return nil, fmt.Errorf("%w: read memory.events oom_kill counter: %v", ErrUnavailable, err)
		}
	}
	if limits.CPUQuota > 0 {
		quota := int64(limits.CPUQuota * defaultCPUCgroupPeriod)
		if quota < 1000 {
			quota = 1000
		}
		value := fmt.Sprintf("%d %d", quota, defaultCPUCgroupPeriod)
		if err := writeControl(filepath.Join(path, "cpu.max"), value); err != nil {
			return nil, fmt.Errorf("%w: configure cpu.max: %v", ErrUnavailable, err)
		}
	}
	if limits.CPUTimeLimit > 0 {
		if _, err := readCPUUsage(path); err != nil {
			return nil, fmt.Errorf("%w: read cpu.stat: %v", ErrUnavailable, err)
		}
	}
	failed = false
	return group, nil
}

func delegatedCgroupRoot() (string, error) {
	if override := os.Getenv("ASTER_CGROUP_ROOT"); override != "" {
		if !filepath.IsAbs(override) {
			return "", fmt.Errorf("ASTER_CGROUP_ROOT must be an absolute path")
		}
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			return "", fmt.Errorf("ASTER_CGROUP_ROOT is not an accessible directory")
		}
		return abs, nil
	}
	cgroupPath, err := currentUnifiedCgroupPath()
	if err != nil {
		return "", err
	}
	mount, mountRoot, err := cgroup2Mount()
	if err != nil {
		return "", err
	}
	relative := cgroupPath
	if mountRoot != "/" && strings.HasPrefix(cgroupPath, strings.TrimSuffix(mountRoot, "/")+"/") {
		relative = strings.TrimPrefix(cgroupPath, mountRoot)
	} else if cgroupPath == mountRoot {
		relative = "/"
	}
	root := filepath.Clean(filepath.Join(mount, strings.TrimPrefix(relative, "/")))
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("current cgroup directory %s is inaccessible", root)
	}
	return root, nil
}

func currentUnifiedCgroupPath() (string, error) {
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "0::") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "0::"))
			if filepath.IsAbs(path) {
				return filepath.Clean(path), nil
			}
		}
	}
	return "", fmt.Errorf("current process has no unified cgroup v2 entry")
}

func cgroup2Mount() (mountpoint, root string, resultErr error) {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return "", "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		separator := strings.Index(line, " - cgroup2 ")
		if separator < 0 {
			continue
		}
		fields := strings.Fields(line[:separator])
		if len(fields) < 5 {
			continue
		}
		return unescapeMountField(fields[4]), unescapeMountField(fields[3]), nil
	}
	return "", "", fmt.Errorf("no cgroup v2 mount was found in /proc/self/mountinfo")
}

func unescapeMountField(value string) string {
	return strings.NewReplacer("\\040", " ", "\\011", "\t", "\\012", "\n", "\\134", "\\").Replace(value)
}

func readWordSet(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	words := make(map[string]bool)
	for _, word := range strings.Fields(string(data)) {
		words[strings.TrimPrefix(word, "+")] = true
	}
	return words, nil
}

func writeControl(path, value string) error {
	return os.WriteFile(path, []byte(value), 0)
}

func (group *workloadCgroup) attach(pid int) error {
	if err := writeControl(filepath.Join(group.path, "cgroup.procs"), strconv.Itoa(pid)); err != nil {
		return fmt.Errorf("%w: assign native process to cgroup: %v", ErrUnavailable, err)
	}
	return nil
}

func (group *workloadCgroup) wait(ctx context.Context, waitCh <-chan error, cpuBudget time.Duration) error {
	var ticker *time.Ticker
	var tickerCh <-chan time.Time
	if cpuBudget > 0 {
		ticker = time.NewTicker(cpuPollInterval)
		tickerCh = ticker.C
		defer ticker.Stop()
	}
	for {
		select {
		case err := <-waitCh:
			return group.handleWorkloadExit(ctx, err, cpuBudget)
		case <-ctx.Done():
			_ = group.kill()
			return ctx.Err()
		case <-tickerCh:
			// select may choose the ticker even when the context deadline has
			// become ready. Prefer cancellation over a resource classification.
			if err := ctx.Err(); err != nil {
				_ = group.kill()
				return err
			}
			usage, err := readCPUUsage(group.path)
			if err != nil {
				_ = group.kill()
				return fmt.Errorf("%w: monitor cpu.stat: %v", ErrUnavailable, err)
			}
			if usage >= uint64(cpuBudget.Microseconds()) {
				return group.stopForCPUTimeLimit(cpuBudget, usage)
			}
		}
	}
}

// stopForCPUTimeLimit stops the whole workload and, when possible, takes a
// final cpu.stat sample after the cgroup has become empty. This captures CPU
// time consumed while the supervisor is delivering the kill request.
func (group *workloadCgroup) stopForCPUTimeLimit(budget time.Duration, usageAtDetection uint64) error {
	limitErr := newCPUTimeLimitError(budget, usageAtDetection, "threshold-detection")
	if err := group.kill(); err != nil {
		return fmt.Errorf("%w: cgroup.kill failed; final CPU usage unavailable: %v", limitErr, err)
	}
	if err := group.waitUntilEmpty(); err != nil {
		return fmt.Errorf("%w: final CPU usage unavailable after stop request: %v", limitErr, err)
	}
	finalUsage, err := readCPUUsage(group.path)
	if err != nil {
		return fmt.Errorf("%w: final CPU usage unavailable after workload termination: %v", limitErr, err)
	}
	return newCPUTimeLimitError(budget, finalUsage, "post-termination")
}

func (group *workloadCgroup) waitUntilEmpty() error {
	deadline := time.Now().Add(cgroupDrainTimeout)
	var lastReadErr error
	for {
		populated, err := cgroupPopulated(group.path)
		if err == nil && !populated {
			return nil
		}
		if err != nil {
			lastReadErr = err
		}
		if !time.Now().Before(deadline) {
			if lastReadErr != nil {
				return fmt.Errorf("cgroup did not become empty within %s (last cgroup.events error: %v)", cgroupDrainTimeout, lastReadErr)
			}
			return fmt.Errorf("cgroup did not become empty within %s", cgroupDrainTimeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// handleWorkloadExit resolves the race where process completion and context
// cancellation become ready together. Cancellation takes precedence over a
// successful exit, and the whole cgroup is asked to stop before returning.
func (group *workloadCgroup) handleWorkloadExit(ctx context.Context, waitErr error, cpuBudget time.Duration) error {
	if err := ctx.Err(); err != nil {
		_ = group.kill()
		return err
	}
	if group.limits.MemoryMaxBytes > 0 {
		oomCount, oomErr := readOOMCount(group.path)
		killCount, killErr := readOOMKillCount(group.path)
		if oomErr == nil && killErr == nil &&
			oomCount > group.initialOOM && killCount > group.initialOOMKills {
			return ErrMemoryLimitExceeded
		}
	}
	// The workload may finish between polling ticks after consuming more
	// than its CPU-time budget. Check the final cumulative metric before
	// accepting the process exit result.
	if cpuBudget > 0 {
		usage, usageErr := readCPUUsage(group.path)
		if usageErr != nil {
			return fmt.Errorf("%w: read cpu.stat after workload exit: %v", ErrUnavailable, usageErr)
		}
		if usage >= uint64(cpuBudget.Microseconds()) {
			return newCPUTimeLimitError(cpuBudget, usage, "workload-exit")
		}
	}
	return waitErr
}

func (group *workloadCgroup) kill() error {
	return writeControl(filepath.Join(group.path, "cgroup.kill"), "1")
}

func (group *workloadCgroup) cleanup() error {
	killErr := group.kill()
	if killErr != nil {
		_ = group.signalCgroupProcesses()
	}
	deadline := time.Now().Add(cgroupDrainTimeout)
	for time.Now().Before(deadline) {
		populated, err := cgroupPopulated(group.path)
		if err == nil && !populated {
			removeErr := os.Remove(group.path)
			if removeErr != nil {
				return removeErr
			}
			return killErr
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("workload cgroup %s did not become empty (kill error: %v)", group.path, killErr)
}

func (group *workloadCgroup) signalCgroupProcesses() error {
	data, err := os.ReadFile(filepath.Join(group.path, "cgroup.procs"))
	if err != nil {
		return err
	}
	var failures []string
	for _, item := range strings.Fields(string(data)) {
		pid, parseErr := strconv.Atoi(item)
		if parseErr != nil {
			failures = append(failures, parseErr.Error())
			continue
		}
		if killErr := syscall.Kill(pid, syscall.SIGKILL); killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
			failures = append(failures, killErr.Error())
		}
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func cgroupPopulated(path string) (bool, error) {
	file, err := os.Open(filepath.Join(path, "cgroup.events"))
	if err != nil {
		return false, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "populated" {
			return fields[1] != "0", nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, fmt.Errorf("cgroup.events has no populated field")
}

func readCPUUsage(path string) (uint64, error) {
	file, err := os.Open(filepath.Join(path, "cpu.stat"))
	if err != nil {
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "usage_usec" {
			return strconv.ParseUint(fields[1], 10, 64)
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("cpu.stat has no usage_usec field")
}

func readOOMCount(path string) (uint64, error) {
	file, err := os.Open(filepath.Join(path, "memory.events"))
	if err != nil {
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "oom" {
			return strconv.ParseUint(fields[1], 10, 64)
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("memory.events has no oom field")
}

func readOOMKillCount(path string) (uint64, error) {
	file, err := os.Open(filepath.Join(path, "memory.events"))
	if err != nil {
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "oom_kill" {
			return strconv.ParseUint(fields[1], 10, 64)
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("memory.events has no oom_kill field")
}

func replaceEnv(existing []string, key, value string) []string {
	prefix := key + "="
	base := existing
	if base == nil {
		base = os.Environ()
	}
	result := make([]string, 0, len(base)+1)
	for _, entry := range base {
		if !strings.HasPrefix(entry, prefix) {
			result = append(result, entry)
		}
	}
	return append(result, prefix+value)
}
