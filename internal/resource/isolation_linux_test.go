//go:build linux

package resource

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
