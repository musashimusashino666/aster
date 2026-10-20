package tests

import "testing"

// TestNestedArraySlicesAndReturnedRowsPreserveValueSemantics exercises sharing
// boundaries that are not covered by scalar slices alone. Mutating a row
// obtained from a slice or a function return must not modify the source matrix.
func TestNestedArraySlicesAndReturnedRowsPreserveValueSemantics(t *testing.T) {
	source := `fn firstRow(values: Array<Array<i64>>) -> Array<i64> {
    return values[0];
}

let mut matrix: Array<Array<i64>> = [[1, 2], [3, 4]];
let mut sliced: Array<Array<i64>> = matrix[0:1];
sliced[0][0] = 90;
print(matrix);
print(sliced);

let mut sourceMatrix: Array<Array<i64>> = [[5, 6], [7, 8]];
let mut returned: Array<i64> = firstRow(sourceMatrix);
returned[1] = 60;
print(sourceMatrix);
print(returned);`

	vmOutput, vmErr, nativeOutput, nativeErr := runTypedOnBothBackends(t, source)
	if vmErr != nil {
		t.Fatalf("VM execution failed: %v\noutput: %q", vmErr, vmOutput)
	}
	if nativeErr != nil {
		t.Fatalf("native execution failed: %v\noutput: %q", nativeErr, nativeOutput)
	}

	const want = "[[1 2] [3 4]]\n" +
		"[[90 2]]\n" +
		"[[5 6] [7 8]]\n" +
		"[5 60]\n"
	if vmOutput != want {
		t.Errorf("unexpected VM output:\n got: %q\nwant: %q", vmOutput, want)
	}
	if nativeOutput != want {
		t.Errorf("unexpected native output:\n got: %q\nwant: %q", nativeOutput, want)
	}
	if nativeOutput != vmOutput {
		t.Errorf("backend output differs:\nVM:     %q\nNative: %q", vmOutput, nativeOutput)
	}
}

// TestIndexedArrayReadsShareOnlyAtValueCopyBoundaries verifies that an array
// row read into a local or assigned to an existing local remains independent,
// while temporary reads in expressions need not create a new semantic owner.
func TestIndexedArrayReadsShareOnlyAtValueCopyBoundaries(t *testing.T) {
	source := `let mut matrix: Array<Array<i64>> = [[1, 2], [3, 4]];

let mut copied: Array<i64> = matrix[0];
copied[1] = 20;
print(matrix);
print(copied);

let mut rebound: Array<i64> = [8, 9];
rebound = matrix[1];
rebound[0] = 30;
print(matrix);
print(rebound);

matrix[0][1] = matrix[0][0] + 10;
print(matrix);`

	vmOutput, vmErr, nativeOutput, nativeErr := runTypedOnBothBackends(t, source)
	if vmErr != nil {
		t.Fatalf("VM execution failed: %v\noutput: %q", vmErr, vmOutput)
	}
	if nativeErr != nil {
		t.Fatalf("native execution failed: %v\noutput: %q", nativeErr, nativeOutput)
	}

	const want = "[[1 2] [3 4]]\n" +
		"[1 20]\n" +
		"[[1 2] [3 4]]\n" +
		"[30 4]\n" +
		"[[1 11] [3 4]]\n"
	if vmOutput != want {
		t.Errorf("unexpected VM output:\n got: %q\nwant: %q", vmOutput, want)
	}
	if nativeOutput != want {
		t.Errorf("unexpected native output:\n got: %q\nwant: %q", nativeOutput, want)
	}
	if nativeOutput != vmOutput {
		t.Errorf("backend output differs:\nVM: %q\nNative: %q", vmOutput, nativeOutput)
	}
}
