//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const resourceGateEnv = "ASTER_RESOURCE_GATE"

// commandResourceLauncher is an internal command. The resource supervisor
// moves this already-started launcher into its cgroup, then creates the gate
// file. The launcher execs the Native artifact in the same PID so its Go
// runtime starts only after cgroup membership is established.
func commandResourceLauncher(args []string) error {
	if len(args) != 1 || args[0] == "" {
		return fmt.Errorf("invalid internal resource launcher arguments")
	}
	gate := os.Getenv(resourceGateEnv)
	if gate == "" {
		return fmt.Errorf("resource launcher requires a supervisor gate")
	}
	target, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("resolve Native artifact path: %w", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		_, err := os.Stat(gate)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("inspect resource supervisor gate: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("resource supervisor gate timed out")
		}
		time.Sleep(time.Millisecond)
	}

	env := make([]string, 0, len(os.Environ()))
	prefix := resourceGateEnv + "="
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, prefix) {
			env = append(env, entry)
		}
	}
	return syscall.Exec(target, []string{target}, env)
}
