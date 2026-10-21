package tests

import "testing"

// TestEnumMatchPayloadArrayAliasesRemainIndependent exercises array-value
// ownership across enum construction, enum copies, function arguments/returns,
// match payload extraction, and nested indexed mutation. The original arrays
// and the arrays reachable through the enum values must remain unchanged when
// a payload extracted by match is mutated.
func TestEnumMatchPayloadArrayAliasesRemainIndependent(t *testing.T) {
	source := `enum Payload {
    Flat(Array<i64>),
    Matrix(Array<Array<i64>>),
}

fn roundTrip(value: Payload) -> Payload {
    return match value {
        Payload::Flat(items) => Payload::Flat(items),
        Payload::Matrix(rows) => Payload::Matrix(rows),
    };
}

let mut flat: Array<i64> = [1, 2, 3];
let flatBox = Payload::Flat(flat);
let flatBoxCopy = flatBox;
let flatRoundTrip = roundTrip(flatBoxCopy);
let mut extractedFlat: Array<i64> = match flatRoundTrip {
    Payload::Flat(items) => items,
    Payload::Matrix(rows) => rows[0],
};
extractedFlat[0] = 9;
print(flat);
print(match flatBox { Payload::Flat(items) => items, Payload::Matrix(rows) => rows[0] });
print(match flatRoundTrip { Payload::Flat(items) => items, Payload::Matrix(rows) => rows[0] });
print(extractedFlat);

let mut matrix: Array<Array<i64>> = [[4, 5], [6, 7]];
let matrixBox = Payload::Matrix(matrix);
let matrixBoxCopy = matrixBox;
let matrixRoundTrip = roundTrip(matrixBoxCopy);
let mut extractedMatrix: Array<Array<i64>> = match matrixRoundTrip {
    Payload::Flat(items) => [items],
    Payload::Matrix(rows) => rows,
};
extractedMatrix[0][1] = 99;
print(matrix);
print(match matrixBox { Payload::Flat(items) => [items], Payload::Matrix(rows) => rows });
print(match matrixRoundTrip { Payload::Flat(items) => [items], Payload::Matrix(rows) => rows });
print(extractedMatrix);`

	vmOutput, vmErr, nativeOutput, nativeErr := runTypedOnBothBackends(t, source)
	if vmErr != nil {
		t.Fatalf("VM execution failed: %v\noutput: %q", vmErr, vmOutput)
	}
	if nativeErr != nil {
		t.Fatalf("Native execution failed: %v\noutput: %q", nativeErr, nativeOutput)
	}

	const want = "[1 2 3]\n" +
		"[1 2 3]\n" +
		"[1 2 3]\n" +
		"[9 2 3]\n" +
		"[[4 5] [6 7]]\n" +
		"[[4 5] [6 7]]\n" +
		"[[4 5] [6 7]]\n" +
		"[[4 99] [6 7]]\n"
	if vmOutput != want {
		t.Errorf("unexpected VM output:\n got: %q\nwant: %q", vmOutput, want)
	}
	if nativeOutput != want {
		t.Errorf("unexpected Native output:\n got: %q\nwant: %q", nativeOutput, want)
	}
	if nativeOutput != vmOutput {
		t.Errorf("backend output differs:\nVM: %q\nNative: %q", vmOutput, nativeOutput)
	}
}
