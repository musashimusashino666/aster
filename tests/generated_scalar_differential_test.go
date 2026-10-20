package tests

import (
	"fmt"
	"math/rand"
	"testing"
)

// seededScalarProgram explores arithmetic, comparisons, short-circuit boolean
// operators, functions, branches, loops, and numeric casts. Inputs are bounded
// so the program should complete without overflow or division-by-zero traps.
func seededScalarProgram(seed int64) string {
	rng := rand.New(rand.NewSource(seed))
	a := rng.Intn(41) - 20
	b := 1 + rng.Intn(20)
	limit := 1 + rng.Intn(5)

	return fmt.Sprintf(`fn combine(left: i64, right: i64) -> i64 {
    let product = left * right;
    if left >= right {
        return product + left;
    } else {
        return product - right;
    }
}

fn accumulate(limit: i64, seed: i64) -> i64 {
    let mut cursor: i64 = 0;
    let mut total: i64 = seed;
    while cursor < limit {
        total = total + combine(cursor + seed, limit);
        cursor = cursor + 1;
    }
    return total;
}

let a: i64 = %d;
let b: i64 = %d;
let limit: i64 = %d;
print(combine(a, b));
print(a / b);
print(a %% b);
print((a < b) && !(a == b));
print((a > b) || (b == 1));
let as_float: f64 = f64(a);
let round_trip: i64 = i64(as_float);
print(round_trip);
print(accumulate(limit, a));
`, a, b, limit)
}

func TestSeededScalarProgramsMatchBackends(t *testing.T) {
	seeds := []int64{0xBADC0DE, 0xC0117, 20261020, 42}
	for _, seed := range seeds {
		seed := seed
		t.Run(fmt.Sprintf("seed_%x", seed), func(t *testing.T) {
			source := seededScalarProgram(seed)
			if repeated := seededScalarProgram(seed); repeated != source {
				t.Fatalf("generator is not deterministic for seed %#x", seed)
			}

			result := runSeededProgramOnBothBackends(t, source, "seeded-scalar-differential.ast")
			if result.vmErr != nil {
				t.Fatalf("VM execution failed for seed %#x: %v\noutput: %q\nsource:\n%s", seed, result.vmErr, result.vmOutput, source)
			}
			if result.nativeErr != nil || result.nativeExitCode != 0 {
				t.Fatalf("Native execution failed for seed %#x: error=%v exit=%d\noutput:\n%s\nsource:\n%s", seed, result.nativeErr, result.nativeExitCode, result.nativeOutput, source)
			}
			if result.vmOutput != result.nativeOutput {
				t.Fatalf("backend output differs for seed %#x\nVM:\n%s\nNative:\n%s\nsource:\n%s", seed, result.vmOutput, result.nativeOutput, source)
			}
		})
	}
}
