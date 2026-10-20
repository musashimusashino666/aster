package formatter

import (
	"strings"
	"testing"
)

func TestFormatCanonicalizesWhitespaceAndPreservesStringEscapes(t *testing.T) {
	input := "fn  f (x:i64)->i64{let s=\"a\\n\\\"b\";return x+1;}"
	want := "fn f(x: i64) -> i64 {\n    let s = \"a\\n\\\"b\";\n    return x + 1;\n}\n"
	got, err := Format(input)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}
}

func TestFormatPreservesLineAndNestedBlockComments(t *testing.T) {
	input := "// file header\nfn  f(){let x=1; // trailing note\n /* outer /* nested */ tail */ return x;}"
	got, err := Format(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"// file header", "// trailing note", "/* outer /* nested */ tail */"} {
		if !strings.Contains(got, want) {
			t.Errorf("formatted source does not contain comment %q: %s", want, got)
		}
	}
	again, err := Format(got)
	if err != nil {
		t.Fatal(err)
	}
	if again != got {
		t.Fatalf("format is not idempotent:\nfirst:  %q\nsecond: %q", got, again)
	}
}

func TestFormatRejectsInvalidSyntax(t *testing.T) {
	if _, err := Format("let x = ;"); err == nil {
		t.Fatal("expected syntax error")
	}
}

func TestFormatEmptySource(t *testing.T) {
	got, err := Format("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("Format empty source = %q, want empty string", got)
	}
}
