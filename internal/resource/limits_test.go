package resource

import (
	"math"
	"testing"
	"time"
)

func TestParseMemoryBytes(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{"256MiB", 256 << 20, false},
		{"2GiB", 2 << 30, false},
		{"4KiB", 4 << 10, false},
		{"123B", 123, false},
		{"4096", 4096, false},
		{" 16MiB ", 16 << 20, false},
		{"", 0, true},
		{"0", 0, true},
		{"-1MiB", 0, true},
		{"1MB", 0, true},
		{"9223372036854775807GiB", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseMemoryBytes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseMemoryBytes(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("ParseMemoryBytes(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestLimitsValidate(t *testing.T) {
	tests := []struct {
		name    string
		limits  Limits
		wantErr bool
	}{
		{"empty", Limits{}, true},
		{"memory", Limits{MemoryMaxBytes: 256 << 20}, false},
		{"quota", Limits{CPUQuota: 0.5}, false},
		{"cpu time", Limits{CPUTimeLimit: time.Second}, false},
		{"negative memory", Limits{MemoryMaxBytes: -1}, true},
		{"negative quota", Limits{CPUQuota: -1}, true},
		{"nan quota", Limits{CPUQuota: math.NaN()}, true},
		{"infinite quota", Limits{CPUQuota: math.Inf(1)}, true},
		{"too small quota", Limits{CPUQuota: 0.001}, true},
		{"too large quota", Limits{CPUQuota: 257}, true},
		{"negative cpu time", Limits{CPUTimeLimit: -time.Second}, true},
		{"sub-microsecond cpu time", Limits{CPUTimeLimit: time.Nanosecond}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.limits.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
