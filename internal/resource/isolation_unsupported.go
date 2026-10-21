//go:build !linux

package resource

import (
	"context"
	"fmt"
	"os/exec"
)

// Run fails closed when the platform backend cannot install process-tree limits.
func Run(ctx context.Context, cmd *exec.Cmd, releasePath string, limits Limits) error {
	return fmt.Errorf("%w: process-tree resource enforcement currently requires Linux cgroup v2", ErrUnavailable)
}
