package tests

import (
	"strings"
	"testing"

	"github.com/aster-lang/aster/internal/bytecode"
	"github.com/aster-lang/aster/internal/codegen"
	"github.com/aster-lang/aster/internal/parser"
)

func nativeDiagnosticPayload(text string) string {
	if index := strings.Index(text, "runtime error:"); index >= 0 {
		return text[index:]
	}
	return strings.TrimSpace(text)
}

func TestBoundaryScalarProgramsMatchBackends(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{
			name: "signed_integer_edges",
			source: `let maximum: i64 = 9223372036854775807;
let minimum: i64 = -9223372036854775808;
print(maximum);
print(minimum);
print(maximum - 1);
print(minimum + 1);
print(maximum / 3);
print(minimum / 3);
`,
		},
		{
			name: "float_to_integer_edges",
			source: `print(i64(-9223372036854775808.0));
print(i64(-0.9));
print(i64(0.9));
print(i64(1.999999));
`,
		},
		{
			name: "nested_array_copy_boundaries",
			source: `let mut matrix: Array<Array<Array<i64>>> = [[[1, 2], [3, 4]], [[5, 6], [7, 8]]];
let snapshot = matrix;
let mut row: Array<i64> = matrix[1][0];
row[1] = 60;
matrix[0][1][0] = 30;
matrix[1][1] = row;
matrix[1][1][0] = 70;
print(matrix);
print(snapshot);
print(row);
`,
		},
		{
			name: "empty_and_full_slices",
			source: `let values: Array<i64> = [4, 5, 6];
print(values[2:2]);
print(values[0:3]);
`,
		},
		{
			name: "nested_control_flow_and_short_circuit",
			source: `fn score(limit: i64) -> i64 {
    let mut i: i64 = 0;
    let mut total: i64 = 0;
    while i <= limit {
        if ((i % 2) == 0) && (i >= 0) {
            total = total + i;
        } else {
            total = total - 1;
        }
        i = i + 1;
    }
    return total;
}
print(score(0));
print(score(1));
print(score(8));
print((false && (1 / 0 == 0)) || (true && (score(2) == 0)));
`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result := runSeededProgramOnBothBackends(t, tc.source, "boundary-"+tc.name+".ast")
			if result.vmErr != nil {
				t.Fatalf("VM failed: %v\noutput: %q\nsource:\n%s", result.vmErr, result.vmOutput, tc.source)
			}
			if result.nativeErr != nil || result.nativeExitCode != 0 {
				t.Fatalf("Native failed: error=%v exit=%d\noutput:\n%s\nsource:\n%s", result.nativeErr, result.nativeExitCode, result.nativeOutput, tc.source)
			}
			if result.vmOutput != result.nativeOutput {
				t.Fatalf("backend output differs\nVM:\n%s\nNative:\n%s\nsource:\n%s", result.vmOutput, result.nativeOutput, tc.source)
			}
		})
	}
}

func TestAdditionalRuntimeBoundaryFailuresMatchBackends(t *testing.T) {
	cases := []struct {
		name       string
		source     string
		diagnostic string
	}{
		{
			name:       "float_to_i64_upper_boundary",
			source:     "print(i64(9223372036854775808.0));\n",
			diagnostic: "out of range for i64 conversion",
		},
		{
			name:       "negate_minimum_i64",
			source:     "let minimum: i64 = -9223372036854775808;\nprint(-minimum);\n",
			diagnostic: "overflow",
		},
		{
			name:       "nested_array_bounds",
			source:     "let matrix: Array<Array<i64>> = [[1], [2]];\nprint(matrix[1][1]);\n",
			diagnostic: "out of bounds",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result := runSeededProgramOnBothBackends(t, tc.source, "boundary-runtime-failure.ast")
			if result.vmErr == nil || !strings.Contains(result.vmErr.Error(), tc.diagnostic) {
				t.Fatalf("VM should report %q, got error=%v output=%q\nsource:\n%s", tc.diagnostic, result.vmErr, result.vmOutput, tc.source)
			}
			if result.vmOutput != "" {
				t.Fatalf("VM emitted output before failing: %q", result.vmOutput)
			}
			if result.nativeErr == nil || result.nativeExitCode == 0 {
				t.Fatalf("Native should exit unsuccessfully, got error=%v exit=%d output=%q\nsource:\n%s", result.nativeErr, result.nativeExitCode, result.nativeOutput, tc.source)
			}
			if got := nativeDiagnosticPayload(strings.TrimSpace(result.nativeOutput)); got != nativeDiagnosticPayload(result.vmErr.Error()) {
				t.Fatalf("backend diagnostic payloads differ: VM=%q Native=%q\nsource:\n%s", nativeDiagnosticPayload(result.vmErr.Error()), got, tc.source)
			}
		})
	}
}

func TestInvalidTypedProgramsAreRejectedBeforeEitherBackend(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{name: "annotated_scalar_mismatch", source: "let value: i64 = true;"},
		{name: "nested_array_element_mismatch", source: "let values: Array<Array<i64>> = [[1], [true]];"},
		{name: "invalid_branch_assignment", source: "let mut value: i64 = 1; if true { value = false; }"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tree, err := parser.Parse(tc.source, "invalid-typed-boundary.ast")
			if err != nil {
				t.Fatalf("test input should parse so that type rejection is exercised: %v", err)
			}
			if _, err := bytecode.Compile(tree); err == nil {
				t.Fatal("bytecode compilation should reject ill-typed program")
			}
			if _, err := codegen.Generate(tree); err == nil {
				t.Fatal("Native code generation should reject ill-typed program")
			}
		})
	}
}
