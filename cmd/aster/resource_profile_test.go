package main

import (
	"strings"
	"testing"
)

func TestCommandRunRequiresExplicitConstrainedProfile(t *testing.T) {
	err := commandRun([]string{"--memory-limit", "64MiB", "examples/hello.ast"})
	if err == nil || !strings.Contains(err.Error(), "require --resource-profile constrained") {
		t.Fatalf("commandRun() error = %v, want explicit-profile error", err)
	}
}

func TestCommandRunConstrainedProfileRequiresNativeEngine(t *testing.T) {
	err := commandRun([]string{"--resource-profile", "constrained", "--memory-limit", "64MiB", "examples/hello.ast"})
	if err == nil || !strings.Contains(err.Error(), "requires --engine native") {
		t.Fatalf("commandRun() error = %v, want native-engine error", err)
	}
}

func TestCommandRunRejectsInvalidResourceLimitsBeforeReadingSource(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "invalid memory limit",
			args: []string{"--engine", "native", "--resource-profile", "constrained", "--memory-limit", "0MiB", "missing.ast"},
			want: "invalid memory limit",
		},
		{
			name: "zero CPU time",
			args: []string{"--engine", "native", "--resource-profile", "constrained", "--memory-limit", "64MiB", "--cpu-time-limit", "0s", "missing.ast"},
			want: "--cpu-time-limit must be greater than zero",
		},
		{
			name: "unknown profile",
			args: []string{"--resource-profile", "unlimited", "missing.ast"},
			want: "unknown resource profile",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := commandRun(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("commandRun() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}
