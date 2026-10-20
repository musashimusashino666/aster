package tests

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// seededCOWAliasSequence generates a deterministic sequence of operations on
// nested arrays. It intentionally creates and rebinds aliases, slices outer
// arrays, extracts and mutates row aliases, replaces rows, and snapshots old
// values. Every generated index is valid, so failures indicate a backend or
// copy-on-write discrepancy rather than an intentional runtime trap.
func seededCOWAliasSequence(seed int64, prefix string) string {
	rng := rand.New(rand.NewSource(seed))
	const rowCount, rowLength = 3, 4

	rows := make([]string, rowCount)
	for i := range rows {
		rows[i] = seededI64Row(rng, rowLength)
	}

	matrix := prefix + "matrix"
	var source strings.Builder
	fmt.Fprintf(&source, "let mut %s: Array<Array<i64>> = [%s];\n", matrix, strings.Join(rows, ", "))

	roots := []string{matrix}
	rootLengths := map[string]int{matrix: rowCount}
	rowAliases := make([]string, 0, 8)
	snapshots := make([]string, 0, 8)
	innerSlices := make([]string, 0, 8)

	// Fixed operation count keeps CI cost predictable while seeds vary both
	// the operation order and which existing alias each operation targets.
	for step := 0; step < 28; step++ {
		switch rng.Intn(8) {
		case 0: // Create another root alias from an arbitrary current root.
			name := fmt.Sprintf("%salias%d", prefix, step)
			parent := roots[rng.Intn(len(roots))]
			fmt.Fprintf(&source, "let mut %s: Array<Array<i64>> = %s;\n", name, parent)
			roots = append(roots, name)
			rootLengths[name] = rootLengths[parent]

		case 1: // Rebind an existing root to another root, including a slice.
			target := roots[rng.Intn(len(roots))]
			parent := roots[rng.Intn(len(roots))]
			fmt.Fprintf(&source, "%s = %s;\n", target, parent)
			rootLengths[target] = rootLengths[parent]

		case 2: // Mutate a nested element through one of the aliases.
			target := roots[rng.Intn(len(roots))]
			row := rng.Intn(rootLengths[target])
			column := rng.Intn(rowLength)
			fmt.Fprintf(&source, "%s[%d][%d] = %s;\n", target, row, column, seededSafeI64Expr(rng, 1))

		case 3: // Extract a row value, then mutate the detached row variable.
			parent := roots[rng.Intn(len(roots))]
			row := rng.Intn(rootLengths[parent])
			name := fmt.Sprintf("%srow%d", prefix, step)
			fmt.Fprintf(&source, "let mut %s: Array<i64> = %s[%d];\n", name, parent, row)
			rowAliases = append(rowAliases, name)
			column := rng.Intn(rowLength)
			fmt.Fprintf(&source, "%s[%d] = %s;\n", name, column, seededSafeI64Expr(rng, 1))

		case 4: // Slice a root and make the slice participate in later aliases.
			parent := roots[rng.Intn(len(roots))]
			parentLength := rootLengths[parent]
			start := rng.Intn(parentLength)
			end := start + 1 + rng.Intn(parentLength-start)
			name := fmt.Sprintf("%souterSlice%d", prefix, step)
			fmt.Fprintf(&source, "let mut %s: Array<Array<i64>> = %s[%d:%d];\n", name, parent, start, end)
			roots = append(roots, name)
			rootLengths[name] = end - start

		case 5: // Replace a row with either a previously copied row or fresh data.
			target := roots[rng.Intn(len(roots))]
			row := rng.Intn(rootLengths[target])
			if len(rowAliases) > 0 && rng.Intn(2) == 0 {
				value := rowAliases[rng.Intn(len(rowAliases))]
				fmt.Fprintf(&source, "%s[%d] = %s;\n", target, row, value)
			} else {
				fmt.Fprintf(&source, "%s[%d] = %s;\n", target, row, seededI64Row(rng, rowLength))
			}

		case 6: // Slice an inner row, mutate the slice, and retain it for checks.
			parent := roots[rng.Intn(len(roots))]
			row := rng.Intn(rootLengths[parent])
			start := rng.Intn(rowLength)
			end := start + 1 + rng.Intn(rowLength-start)
			name := fmt.Sprintf("%sinnerSlice%d", prefix, step)
			fmt.Fprintf(&source, "let mut %s: Array<i64> = %s[%d][%d:%d];\n", name, parent, row, start, end)
			innerSlices = append(innerSlices, name)
			column := rng.Intn(end - start)
			fmt.Fprintf(&source, "%s[%d] = %s;\n", name, column, seededSafeI64Expr(rng, 1))

		case 7: // Preserve a snapshot before later mutations or rebindings.
			parent := roots[rng.Intn(len(roots))]
			name := fmt.Sprintf("%ssnapshot%d", prefix, step)
			fmt.Fprintf(&source, "let %s: Array<Array<i64>> = %s;\n", name, parent)
			snapshots = append(snapshots, name)
		}

		// Observe every live root after each transition. This makes transient
		// alias corruption visible even if a later operation overwrites it.
		for _, root := range roots {
			fmt.Fprintf(&source, "print(%s);\n", root)
		}
	}

	// Read out detached row/slice values and immutable snapshots as well.
	for _, name := range rowAliases {
		fmt.Fprintf(&source, "print(%s);\n", name)
	}
	for _, name := range innerSlices {
		fmt.Fprintf(&source, "print(%s);\n", name)
	}
	for _, name := range snapshots {
		fmt.Fprintf(&source, "print(%s);\n", name)
	}
	return source.String()
}

func TestSeededCOWAliasSequencesMatchBackends(t *testing.T) {
	seeds := []int64{0xC0A, 0xA11A5, 0x5EEDC0, 20261020, 0xBADC0DE, 0xD1FF, 0xA57E, 0xF00D}
	var combined strings.Builder
	for i, seed := range seeds {
		prefix := fmt.Sprintf("s%02d_", i)
		source := seededCOWAliasSequence(seed, prefix)
		if repeated := seededCOWAliasSequence(seed, prefix); repeated != source {
			t.Fatalf("COW alias generator is not deterministic for seed %#x", seed)
		}
		combined.WriteString(source)
	}

	source := combined.String()
	result := runSeededProgramOnBothBackends(t, source, "seeded-cow-alias-sequences.ast")
	if result.vmErr != nil {
		t.Fatalf("VM failed for generated COW alias sequences: %v\noutput: %q\nsource:\n%s", result.vmErr, result.vmOutput, source)
	}
	if result.nativeErr != nil || result.nativeExitCode != 0 {
		t.Fatalf("Native failed for generated COW alias sequences: error=%v exit=%d\noutput:\n%s\nsource:\n%s", result.nativeErr, result.nativeExitCode, result.nativeOutput, source)
	}
	if result.vmOutput != result.nativeOutput {
		t.Fatalf("backend output differs for generated COW alias sequences\nVM:\n%s\nNative:\n%s\nsource:\n%s", result.vmOutput, result.nativeOutput, source)
	}
}
