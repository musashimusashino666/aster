package parser

import (
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/langtypes"
)

func nestedArrayType(depth int) string {
	return strings.Repeat("Array<", depth) + "i64" + strings.Repeat(">", depth)
}

func TestTypeNestingLimitIsEnforcedByParser(t *testing.T) {
	valid := "fn f(value: " + nestedArrayType(langtypes.MaxNestingDepth) + ") {}"
	if _, err := Parse(valid, "type-depth.ast"); err != nil {
		t.Fatalf("maximum supported nesting should parse: %v", err)
	}

	tooDeep := "fn f(value: " + nestedArrayType(langtypes.MaxNestingDepth+1) + ") {}"
	if _, err := Parse(tooDeep, "type-depth.ast"); err == nil || !strings.Contains(err.Error(), "type nesting exceeds limit") {
		t.Fatalf("expected source type nesting limit error, got %v", err)
	}
}
