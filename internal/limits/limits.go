// Package limits defines value-size limits shared by Aster execution backends.
package limits

// String values are limited by UTF-8 byte length, not Unicode code points.
const MaxStringBytes = 16 << 20

// MaxArrayLength bounds the number of elements in each array value.
const MaxArrayLength = 1 << 18
