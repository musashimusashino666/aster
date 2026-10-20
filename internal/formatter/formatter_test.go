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

func TestFormatGenericNominalTypesWithoutOperatorSpacing(t *testing.T) {
	input := "struct Box<T>{value:T,} " +
		"struct Pair<K,V>{first:K,second:V,} " +
		"enum Option<T>{None,Some(T),} " +
		"let b:Box<i64>=Box::<i64>(42); " +
		"let pair:Pair<string,i64>=Pair::<string,i64>(\"x\",42); " +
		"let none:Option<i64>=Option::<i64>::None; " +
		"let a:i64=1;let z:i64=2;print(a<z);"
	got, err := Format(input)
	if err != nil {
		t.Fatalf("Format() failed: %v", err)
	}
	for _, want := range []string{
		"struct Box<T>",
		"struct Pair<K, V>",
		"enum Option<T>",
		"Box<i64>",
		"Pair<string, i64>",
		"Option<i64>",
		"Box::<i64>",
		"Pair::<string, i64>",
		"Option::<i64>::None",
		"print(a < z);",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("formatted source does not contain %q:\n%s", want, got)
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

func TestFormatDistinguishesArrayTypeArgumentsFromComparisons(t *testing.T) {
	input := "fn identity(values:Array<i64>)->Array<i64>{return values;}\n" +
		"let Array: i64 = 4; let n: i64 = 5; print(Array<n && n>Array);"
	want := "fn identity(values: Array<i64>) -> Array<i64> {\n" +
		"    return values;\n" +
		"}\n" +
		"let Array: i64 = 4;\n" +
		"let n: i64 = 5;\n" +
		"print(Array < n && n > Array);\n"

	got, err := Format(input)
	if err != nil {
		t.Fatalf("Format() failed: %v", err)
	}
	if got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}
	again, err := Format(got)
	if err != nil {
		t.Fatal(err)
	}
	if again != got {
		t.Fatalf("format is not idempotent:\nfirst:  %q\nsecond: %q", got, again)
	}
}
