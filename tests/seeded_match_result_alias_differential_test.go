package tests

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// seededDirectLocalMatchCase exercises MatchExpr results whose arms directly
// return pre-existing array locals rather than payload bindings or fresh
// arrays. selector chooses which arm executes, while every arm remains a
// possible alias source for both an initializer and a later assignment.
func seededDirectLocalMatchCase(seed int64, prefix string, selector int) (string, []string) {
	rng := rand.New(rand.NewSource(seed))
	variants := []string{"Left", "Middle", "Right"}
	names := []string{prefix + "left", prefix + "middle", prefix + "right"}
	originals := make([][]int, len(names))
	var source strings.Builder

	for i, name := range names {
		originals[i] = seededAggregateValues(rng, 3)
		fmt.Fprintf(&source, "let mut %s: Array<i64> = %s;\n", name, seededAggregateArrayLiteral(originals[i]))
	}

	writeMatch := func(variant int) {
		fmt.Fprintf(&source, "match Pick::%s {\n", variants[variant])
		for arm, name := range names {
			fmt.Fprintf(&source, "    Pick::%s => %s,\n", variants[arm], name)
		}
		source.WriteString("}")
	}

	letIndex := rng.Intn(3)
	letValue := 50 + rng.Intn(50)
	fmt.Fprintf(&source, "let mut %sfromLet: Array<i64> = ", prefix)
	writeMatch(selector)
	source.WriteString(";\n")
	fmt.Fprintf(&source, "%sfromLet[%d] = %d;\n", prefix, letIndex, letValue)
	for _, name := range names {
		fmt.Fprintf(&source, "print(%s);\n", name)
	}
	fmt.Fprintf(&source, "print(%sfromLet);\n", prefix)

	assignSelector := (selector + 1) % len(variants)
	assignIndex := rng.Intn(3)
	assignValue := 100 + rng.Intn(50)
	fmt.Fprintf(&source, "let mut %sfromAssign: Array<i64> = [101, 102, 103];\n", prefix)
	fmt.Fprintf(&source, "%sfromAssign = ", prefix)
	writeMatch(assignSelector)
	source.WriteString(";\n")
	fmt.Fprintf(&source, "%sfromAssign[%d] = %d;\n", prefix, assignIndex, assignValue)
	for _, name := range names {
		fmt.Fprintf(&source, "print(%s);\n", name)
	}
	fmt.Fprintf(&source, "print(%sfromAssign);\n", prefix)

	expected := make([]string, 0, 8)
	for _, values := range originals {
		expected = append(expected, seededAggregateArrayDisplay(values))
	}
	fromLet := append([]int(nil), originals[selector]...)
	fromLet[letIndex] = letValue
	expected = append(expected, seededAggregateArrayDisplay(fromLet))

	for _, values := range originals {
		expected = append(expected, seededAggregateArrayDisplay(values))
	}
	fromAssign := append([]int(nil), originals[assignSelector]...)
	fromAssign[assignIndex] = assignValue
	expected = append(expected, seededAggregateArrayDisplay(fromAssign))

	return source.String(), expected
}

// TestSeededMatchResultLocalArraysMatchBackends expands the COW differential
// coverage beyond payload extraction: every enum arm directly returns an
// existing local array, and both declaration and reassignment boundaries are
// mutated afterward. Fixed seeds preserve reproducibility, and the selector
// schedule guarantees that each match arm is executed.
func TestSeededMatchResultLocalArraysMatchBackends(t *testing.T) {
	seeds := []int64{
		0xC0A, 0xA11A5, 0x5EEDC0,
		20261020, 0xBADC0DE, 0xD1FF,
		0xA57E, 0xF00D, 0x1A2B3C,
	}

	var source strings.Builder
	source.WriteString("enum Pick { Left, Middle, Right, }\n")
	expected := make([]string, 0, len(seeds)*8)
	for i, seed := range seeds {
		prefix := fmt.Sprintf("matchSeed%02d_", i)
		selector := i % 3
		generated, want := seededDirectLocalMatchCase(seed, prefix, selector)
		repeated, repeatedWant := seededDirectLocalMatchCase(seed, prefix, selector)
		if repeated != generated || strings.Join(repeatedWant, "\n") != strings.Join(want, "\n") {
			t.Fatalf("direct-local match generator is not deterministic for seed %#x", seed)
		}
		source.WriteString(generated)
		expected = append(expected, want...)
	}

	program := source.String()
	result := runSeededProgramOnBothBackends(t, program, "seeded-match-result-alias.ast")
	want := strings.Join(expected, "\n") + "\n"
	if result.vmErr != nil {
		t.Fatalf("VM failed for seeded direct-local match aliases: %v\noutput: %q\nsource:\n%s", result.vmErr, result.vmOutput, program)
	}
	if result.nativeErr != nil || result.nativeExitCode != 0 {
		t.Fatalf("Native failed for seeded direct-local match aliases: error=%v exit=%d\noutput:\n%s\nsource:\n%s",
			result.nativeErr, result.nativeExitCode, result.nativeOutput, program)
	}
	if result.vmOutput != want {
		t.Fatalf("unexpected VM output for seeded direct-local match aliases:\n got: %q\nwant: %q\nsource:\n%s",
			result.vmOutput, want, program)
	}
	if result.nativeOutput != want {
		t.Fatalf("unexpected Native output for seeded direct-local match aliases:\n got: %q\nwant: %q\nsource:\n%s",
			result.nativeOutput, want, program)
	}
	if result.vmOutput != result.nativeOutput {
		t.Fatalf("backend output differs for seeded direct-local match aliases\nVM:\n%s\nNative:\n%s\nsource:\n%s",
			result.vmOutput, result.nativeOutput, program)
	}
}
