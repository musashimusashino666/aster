// Package limits defines value-size limits shared by Aster execution backends.
package limits

// String values are limited by UTF-8 byte length, not Unicode code points.
const MaxStringBytes = 16 << 20

// MaxStdoutBytes bounds cumulative UTF-8 output across all print calls, including newlines.
const MaxStdoutBytes = 64 << 20

// MaxArrayLength bounds the number of elements in each array value.
const MaxArrayLength = 1 << 18

// MaxArrayNestingDepth bounds recursive array traversal and display.
const MaxArrayNestingDepth = 256

// MaxCallDepth bounds active Aster function calls in every execution backend.
const MaxCallDepth = 4096
