// Package resource implements opt-in operating-system resource enforcement.
package resource

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrUnavailable means the requested OS-backed guarantee cannot be installed.
	ErrUnavailable = errors.New("resource isolation unavailable")
	// ErrMemoryLimitExceeded means the operating system confirmed a cgroup memory OOM event.
	ErrMemoryLimitExceeded = errors.New("process memory limit exceeded")
	// ErrCPUTimeLimitExceeded means measured aggregate cgroup CPU time reached the configured budget.
	ErrCPUTimeLimitExceeded = errors.New("cumulative CPU-time budget exceeded")
)

// Limits contains opt-in runtime controls. CPUQuota is measured in logical CPUs:
// 0.5 permits half of one logical CPU, and 2 permits two logical CPUs.
type Limits struct {
	MemoryMaxBytes int64
	CPUQuota       float64
	CPUTimeLimit   time.Duration
}

// Validate rejects empty, negative, non-finite, or impractical resource settings.
func (limits Limits) Validate() error {
	if limits.MemoryMaxBytes < 0 {
		return fmt.Errorf("memory limit must be positive")
	}
	if limits.CPUQuota < 0 || math.IsNaN(limits.CPUQuota) || math.IsInf(limits.CPUQuota, 0) {
		return fmt.Errorf("CPU rate must be a finite positive number")
	}
	if limits.CPUQuota > 0 && (limits.CPUQuota < 0.01 || limits.CPUQuota > 256) {
		return fmt.Errorf("CPU rate must be between 0.01 and 256 logical CPUs")
	}
	if limits.CPUTimeLimit < 0 {
		return fmt.Errorf("CPU-time limit must be positive")
	}
	if limits.MemoryMaxBytes == 0 && limits.CPUQuota == 0 && limits.CPUTimeLimit == 0 {
		return fmt.Errorf("constrained execution requires at least one resource limit")
	}
	return nil
}

// ParseMemoryBytes parses a byte count or an IEC size such as 256MiB.
// Accepted suffixes are B, KiB, MiB, and GiB; a bare integer means bytes.
func ParseMemoryBytes(value string) (int64, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0, fmt.Errorf("memory limit is empty")
	}
	unit := int64(1)
	number := text
	for suffix, multiplier := range map[string]int64{
		"GiB": 1 << 30,
		"MiB": 1 << 20,
		"KiB": 1 << 10,
		"B":   1,
	} {
		if strings.HasSuffix(strings.ToLower(text), strings.ToLower(suffix)) {
			number = strings.TrimSpace(text[:len(text)-len(suffix)])
			unit = multiplier
			break
		}
	}
	parsed, err := strconv.ParseInt(number, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("invalid memory limit %q: use a positive byte count or B/KiB/MiB/GiB suffix", value)
	}
	if parsed > math.MaxInt64/unit {
		return 0, fmt.Errorf("memory limit %q overflows int64 bytes", value)
	}
	return parsed * unit, nil
}
