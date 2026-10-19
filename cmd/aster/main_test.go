package main

import (
	"reflect"
	"testing"
)

func TestFlagsBeforePositionals(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		flags []string
		want  []string
	}{
		{
			name:  "run flag after source",
			args:  []string{"examples/hello.ast", "--engine", "native"},
			flags: []string{"--engine"},
			want:  []string{"--engine", "native", "examples/hello.ast"},
		},
		{
			name:  "equals syntax after source",
			args:  []string{"examples/hello.ast", "--engine=native"},
			flags: []string{"--engine"},
			want:  []string{"--engine=native", "examples/hello.ast"},
		},
		{
			name:  "multiple build flags after source",
			args:  []string{"main.ast", "-o", "app", "--target", "native"},
			flags: []string{"-o", "--target"},
			want:  []string{"-o", "app", "--target", "native", "main.ast"},
		},
		{
			name:  "unrecognized arguments remain positional",
			args:  []string{"first.ast", "second.ast"},
			flags: []string{"--engine"},
			want:  []string{"first.ast", "second.ast"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := flagsBeforePositionals(tt.args, tt.flags...); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("flagsBeforePositionals() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
