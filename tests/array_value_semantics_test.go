package tests

import "testing"

// TestArrayValueSemanticsMatchBackends covers independent array values across
// binding copies, rebinding, function calls, nested updates, and slices. Keep
// this as one cross-backend program so all cases exercise the same VM/native
// execution paths and the expected output is an executable statement of ADR 0003.
func TestArrayValueSemanticsMatchBackends(t *testing.T) {
	source := `fn changeArgument(values: Array<i64>) -> Array<i64> {
    let mut local: Array<i64> = values;
    local[0] = 70;
    return local;
}

let mut current: Array<i64> = [1, 2, 3];
let earlier = current;
current[1] = 20;
print(current);
print(earlier);

let previous = current;
current = [4, 5];
current[0] = 40;
print(current);
print(previous);

let original: Array<i64> = [6, 7, 8];
let returned = changeArgument(original);
print(original);
print(returned);
let mut returnedCopy = returned;
returnedCopy[1] = 77;
print(returned);
print(returnedCopy);

let mut matrix: Array<Array<i64>> = [[1, 2], [3, 4]];
let nestedSnapshot = matrix;
matrix[0][1] = 99;
print(matrix);
print(nestedSnapshot);

let mut holder: Array<Array<i64>> = [[0, 0], [0, 0]];
let row: Array<i64> = [5, 6];
holder[1] = row;
holder[1][0] = 55;
print(row);
print(holder);

let source: Array<i64> = [10, 20, 30, 40];
let mut slice: Array<i64> = source[1:3];
slice[0] = 99;
print(source);
print(slice);

let matrixForSlice: Array<Array<i64>> = [[7, 8, 9], [10, 11, 12]];
let mut nestedSlice: Array<i64> = matrixForSlice[1][0:2];
nestedSlice[0] = 77;
print(matrixForSlice);
print(nestedSlice);
print(source[2:2]);`

	vmOutput, vmErr, nativeOutput, nativeErr := runTypedOnBothBackends(t, source)
	if vmErr != nil {
		t.Fatalf("VM execution failed: %v\noutput: %q", vmErr, vmOutput)
	}
	if nativeErr != nil {
		t.Fatalf("native execution failed: %v\noutput: %q", nativeErr, nativeOutput)
	}

	const want = "[1 20 3]\n" +
		"[1 2 3]\n" +
		"[40 5]\n" +
		"[1 20 3]\n" +
		"[6 7 8]\n" +
		"[70 7 8]\n" +
		"[70 7 8]\n" +
		"[70 77 8]\n" +
		"[[1 99] [3 4]]\n" +
		"[[1 2] [3 4]]\n" +
		"[5 6]\n" +
		"[[0 0] [55 6]]\n" +
		"[10 20 30 40]\n" +
		"[99 30]\n" +
		"[[7 8 9] [10 11 12]]\n" +
		"[77 11]\n" +
		"[]\n"

	if vmOutput != want {
		t.Fatalf("unexpected VM output:\n got: %q\nwant: %q", vmOutput, want)
	}
	if nativeOutput != want {
		t.Fatalf("unexpected native output:\n got: %q\nwant: %q", nativeOutput, want)
	}
	if nativeOutput != vmOutput {
		t.Fatalf("backend output differs:\nVM:     %q\nNative: %q", vmOutput, nativeOutput)
	}
}
