package checker

import (
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/parser"
)

func checkSource(t *testing.T, src string) error {
	t.Helper()
	p, err := parser.Parse(src, "test.ast")
	if err != nil {
		return err
	}
	return Check(p)
}
func TestAcceptsRecursionAndTypeInference(t *testing.T) {
	src := `fn fact(n: i64) -> i64 { if n <= 1 { return 1; } else { return n * fact(n - 1); } } let mut result = fact(5); result = result + 1; print(result);`
	if err := checkSource(t, src); err != nil {
		t.Fatal(err)
	}
}
func TestRejectsImmutableAssignment(t *testing.T) {
	err := checkSource(t, "let x = 1; x = 2;")
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("unexpected error: %v", err)
	}
}
func TestRejectsTypeMismatch(t *testing.T) {
	err := checkSource(t, `let x: i64 = "bad";`)
	if err == nil || !strings.Contains(err.Error(), "cannot assign string") {
		t.Fatalf("unexpected error: %v", err)
	}
}
func TestRequiresReturnOnEveryPath(t *testing.T) {
	err := checkSource(t, `fn f(n: i64) -> i64 { if n > 0 { return n; } }`)
	if err == nil || !strings.Contains(err.Error(), "must return i64") {
		t.Fatalf("unexpected error: %v", err)
	}
}
func TestRejectsBadCallArity(t *testing.T) {
	err := checkSource(t, `fn f(x: i64) -> i64 { return x; } print(f());`)
	if err == nil || !strings.Contains(err.Error(), "expects 1 arguments") {
		t.Fatalf("unexpected error: %v", err)
	}
}
func TestRejectsUnreachableCode(t *testing.T) {
	err := checkSource(t, `fn f() { return; print(1); }`)
	if err == nil || !strings.Contains(err.Error(), "unreachable statement") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestArraysAreHomogeneousAndTyped(t *testing.T) {
	good := `fn sum(items: Array<i64>) -> i64 { let mut i: i64 = 0; let mut s: i64 = 0; while i < len(items) { s = s + items[i]; i = i + 1; } return s; } print(sum([1, 2, 3]));`
	if err := checkSource(t, good); err != nil {
		t.Fatal(err)
	}
	bad := `let items = [1, "two"];`
	if err := checkSource(t, bad); err == nil || !strings.Contains(err.Error(), "array elements must have one type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRejectsUnitValueInInferredBinding(t *testing.T) {
	err := checkSource(t, "let x = print(1);")
	if err == nil || !strings.Contains(err.Error(), "variables cannot hold unit values") {
		t.Fatalf("expected unit binding to be rejected, got %v", err)
	}
}

func TestRejectsPrintingUnitValue(t *testing.T) {
	err := checkSource(t, "print(print(1));")
	if err == nil || !strings.Contains(err.Error(), "print cannot display a unit value") {
		t.Fatalf("expected unit print argument to be rejected, got %v", err)
	}
}
