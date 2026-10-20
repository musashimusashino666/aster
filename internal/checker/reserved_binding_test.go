package checker

import (
	"strings"
	"testing"
)

func TestRejectsBindingsThatShadowBuiltinsAndCasts(t *testing.T) {
	for _, name := range []string{"print", "len", "assert", "i64", "f64"} {
		t.Run(name, func(t *testing.T) {
			err := checkSource(t, "let "+name+" = 1;")
			if err == nil || !strings.Contains(err.Error(), "reserved") {
				t.Fatalf("expected reserved-binding error for %q, got %v", name, err)
			}
		})
	}
}
