package vm

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

var cowFuzzOuterNames = [...]string{"a", "b", "c", "d"}
var cowFuzzRowNames = [...]string{"r0", "r1", "r2"}

func cowFuzzCloneRow(values []int64) []int64 {
	cloned := make([]int64, len(values))
	copy(cloned, values)
	return cloned
}

func cowFuzzCloneMatrix(values [][]int64) [][]int64 {
	cloned := make([][]int64, len(values))
	for i := range values {
		cloned[i] = cowFuzzCloneRow(values[i])
	}
	return cloned
}

func cowFuzzFormatRow(values []int64) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strconv.FormatInt(value, 10)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func cowFuzzFormatMatrix(values [][]int64) string {
	parts := make([]string, len(values))
	for i, row := range values {
		parts[i] = cowFuzzFormatRow(row)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// FuzzVMArrayCOWMatchesReferenceModel generates bounded, well-typed array
// programs and compares the VM output against a deep-copy reference model.
// The model intentionally has no shared-storage optimization: each Aster
// value-copy boundary is represented by a recursive copy.
func FuzzVMArrayCOWMatchesReferenceModel(f *testing.F) {
	// Ensure that ordinary CI exercises every operation family deterministically.
	allOperations := make([]byte, 0, 13*6)
	for operation := 0; operation < 13; operation++ {
		allOperations = append(allOperations,
			byte(operation), byte(operation%4), byte((operation+1)%4),
			byte(operation+1), byte(operation+2), byte(operation+20))
	}
	f.Add(allOperations)
	f.Add([]byte{1, 0, 0, 1, 2, 99, 0, 1, 0, 0, 0, 0, 11, 1, 2, 0, 0, 17})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		inputByte := func(index int) byte {
			if len(data) == 0 {
				return 0
			}
			return data[index%len(data)]
		}

		outer := [4][][]int64{
			{{1, 2, 3}, {4, 5, 6}},
			{{7, 8, 9}, {10, 11, 12}},
			{{13, 14, 15}, {16, 17, 18}},
			{{19, 20, 21}, {22, 23, 24}},
		}
		rows := [3][]int64{
			{25, 26, 27},
			{28, 29, 30},
			{31, 32, 33},
		}

		var source strings.Builder
		source.WriteString(`fn returnOuter(values: Array<Array<i64>>) -> Array<Array<i64>> {
    return values;
}
fn returnRow(values: Array<i64>) -> Array<i64> {
    return values;
}
fn mutateOuter(values: Array<Array<i64>>) -> Array<Array<i64>> {
    let mut local: Array<Array<i64>> = values;
    if len(local) > 0 {
        if len(local[0]) > 0 {
            local[0][0] = 123;
        }
    }
    return local;
}
fn mutateRow(values: Array<i64>) -> Array<i64> {
    let mut local: Array<i64> = values;
    if len(local) > 0 {
        local[0] = 123;
    }
    return local;
}

let mut a: Array<Array<i64>> = [[1, 2, 3], [4, 5, 6]];
let mut b: Array<Array<i64>> = [[7, 8, 9], [10, 11, 12]];
let mut c: Array<Array<i64>> = [[13, 14, 15], [16, 17, 18]];
let mut d: Array<Array<i64>> = [[19, 20, 21], [22, 23, 24]];
let mut r0: Array<i64> = [25, 26, 27];
let mut r1: Array<i64> = [28, 29, 30];
let mut r2: Array<i64> = [31, 32, 33];
`)

		// Each six-byte record selects an operation and supplies bounded indexes.
		// Limit generated programs so fuzzing remains fast and resource-predictable.
		operationCount := (len(data) + 5) / 6
		if operationCount == 0 {
			operationCount = 1
		}
		if operationCount > 32 {
			operationCount = 32
		}

		for operation := 0; operation < operationCount; operation++ {
			offset := operation * 6
			choice := int(inputByte(offset)) % 13
			dst := int(inputByte(offset+1)) % len(cowFuzzOuterNames)
			src := int(inputByte(offset+2)) % len(cowFuzzOuterNames)
			first := int(inputByte(offset + 3))
			second := int(inputByte(offset + 4))
			scalar := int64(int(inputByte(offset+5)%101) - 50)

			switch choice {
			case 0: // Array binding/rebinding copy.
				fmt.Fprintf(&source, "%s = %s;\n", cowFuzzOuterNames[dst], cowFuzzOuterNames[src])
				outer[dst] = cowFuzzCloneMatrix(outer[src])

			case 1: // Nested scalar mutation.
				if len(outer[dst]) > 0 {
					rowIndex := first % len(outer[dst])
					if len(outer[dst][rowIndex]) > 0 {
						columnIndex := second % len(outer[dst][rowIndex])
						fmt.Fprintf(&source, "%s[%d][%d] = %d;\n", cowFuzzOuterNames[dst], rowIndex, columnIndex, scalar)
						outer[dst][rowIndex][columnIndex] = scalar
					}
				}

			case 2: // Replace an outer-array element with another nested row.
				if len(outer[dst]) > 0 && len(outer[src]) > 0 {
					dstIndex := first % len(outer[dst])
					srcIndex := second % len(outer[src])
					fmt.Fprintf(&source, "%s[%d] = %s[%d];\n", cowFuzzOuterNames[dst], dstIndex, cowFuzzOuterNames[src], srcIndex)
					outer[dst][dstIndex] = cowFuzzCloneRow(outer[src][srcIndex])
				}

			case 3: // Insert a row variable into an outer array.
				if len(outer[dst]) > 0 {
					dstIndex := first % len(outer[dst])
					rowIndex := int(inputByte(offset+2)) % len(cowFuzzRowNames)
					fmt.Fprintf(&source, "%s[%d] = %s;\n", cowFuzzOuterNames[dst], dstIndex, cowFuzzRowNames[rowIndex])
					outer[dst][dstIndex] = cowFuzzCloneRow(rows[rowIndex])
				}

			case 4: // Extract a nested row into an independently mutable variable.
				if len(outer[src]) > 0 {
					rowIndex := dst % len(cowFuzzRowNames)
					matrixIndex := first % len(outer[src])
					fmt.Fprintf(&source, "%s = %s[%d];\n", cowFuzzRowNames[rowIndex], cowFuzzOuterNames[src], matrixIndex)
					rows[rowIndex] = cowFuzzCloneRow(outer[src][matrixIndex])
				}

			case 5: // Copy/rebind a row variable.
				dstRow := dst % len(cowFuzzRowNames)
				srcRow := src % len(cowFuzzRowNames)
				fmt.Fprintf(&source, "%s = %s;\n", cowFuzzRowNames[dstRow], cowFuzzRowNames[srcRow])
				rows[dstRow] = cowFuzzCloneRow(rows[srcRow])

			case 6: // Scalar mutation through a row variable.
				dstRow := dst % len(cowFuzzRowNames)
				if len(rows[dstRow]) > 0 {
					index := first % len(rows[dstRow])
					fmt.Fprintf(&source, "%s[%d] = %d;\n", cowFuzzRowNames[dstRow], index, scalar)
					rows[dstRow][index] = scalar
				}

			case 7: // Slice an outer array, including an empty slice.
				length := len(outer[src])
				start := first % (length + 1)
				end := start + second%(length-start+1)
				fmt.Fprintf(&source, "%s = %s[%d:%d];\n", cowFuzzOuterNames[dst], cowFuzzOuterNames[src], start, end)
				outer[dst] = cowFuzzCloneMatrix(outer[src][start:end])

			case 8: // Function argument and return boundary for an outer array.
				fmt.Fprintf(&source, "%s = returnOuter(%s);\n", cowFuzzOuterNames[dst], cowFuzzOuterNames[src])
				outer[dst] = cowFuzzCloneMatrix(outer[src])

			case 9: // Function argument and return boundary for a row.
				dstRow := dst % len(cowFuzzRowNames)
				srcRow := src % len(cowFuzzRowNames)
				fmt.Fprintf(&source, "%s = returnRow(%s);\n", cowFuzzRowNames[dstRow], cowFuzzRowNames[srcRow])
				rows[dstRow] = cowFuzzCloneRow(rows[srcRow])

			case 10: // Slice an inner row and store the result as a separate value.
				if len(outer[src]) > 0 {
					dstRow := dst % len(cowFuzzRowNames)
					matrixIndex := first % len(outer[src])
					row := outer[src][matrixIndex]
					start := second % (len(row) + 1)
					end := start + int(inputByte(offset+5))%(len(row)-start+1)
					fmt.Fprintf(&source, "%s = %s[%d][%d:%d];\n", cowFuzzRowNames[dstRow], cowFuzzOuterNames[src], matrixIndex, start, end)
					rows[dstRow] = cowFuzzCloneRow(row[start:end])
				}

			case 11: // Mutate a local copy inside a function and return it.
				fmt.Fprintf(&source, "%s = mutateOuter(%s);\n", cowFuzzOuterNames[dst], cowFuzzOuterNames[src])
				outer[dst] = cowFuzzCloneMatrix(outer[src])
				if len(outer[dst]) > 0 && len(outer[dst][0]) > 0 {
					outer[dst][0][0] = 123
				}

			case 12: // Mutate a row local inside a function and return it.
				dstRow := dst % len(cowFuzzRowNames)
				srcRow := src % len(cowFuzzRowNames)
				fmt.Fprintf(&source, "%s = mutateRow(%s);\n", cowFuzzRowNames[dstRow], cowFuzzRowNames[srcRow])
				rows[dstRow] = cowFuzzCloneRow(rows[srcRow])
				if len(rows[dstRow]) > 0 {
					rows[dstRow][0] = 123
				}
			}
		}

		var expected strings.Builder
		for _, name := range cowFuzzOuterNames {
			fmt.Fprintf(&source, "print(%s);\n", name)
		}
		for _, name := range cowFuzzRowNames {
			fmt.Fprintf(&source, "print(%s);\n", name)
		}
		for _, matrix := range outer {
			fmt.Fprintf(&expected, "%s\n", cowFuzzFormatMatrix(matrix))
		}
		for _, row := range rows {
			fmt.Fprintf(&expected, "%s\n", cowFuzzFormatRow(row))
		}

		got, err := runSource(t, source.String())
		if err != nil {
			t.Fatalf("generated VM program failed: %v\ninput: %v\nsource:\n%s", err, data, source.String())
		}
		if want := expected.String(); got != want {
			t.Fatalf("COW reference-model mismatch\ninput: %v\nsource:\n%s\n got: %q\nwant: %q", data, source.String(), got, want)
		}
	})
}
