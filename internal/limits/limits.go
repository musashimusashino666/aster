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

// MaxEnumTypes bounds the number of nominal enum declarations in one program.
const MaxEnumTypes = 1 << 16

// MaxEnumVariants bounds the total number of variants across all enums.
const MaxEnumVariants = 1 << 16

// MaxEnumPayloadFields bounds the total number of enum payload fields in one program.
const MaxEnumPayloadFields = 1 << 16

// MaxMatchTablesPerFunction bounds match dispatch tables attached to one function.
const MaxMatchTablesPerFunction = 1 << 16

// MaxTotalMatchTables bounds match dispatch tables across a bytecode program.
const MaxTotalMatchTables = 1 << 20

// MaxMatchTableTargets bounds the total number of dispatch targets across a bytecode program.
const MaxMatchTableTargets = 1 << 20

// MaxGenericTypeParameters bounds type parameters on one generic function declaration.
const MaxGenericTypeParameters = 32

// MaxGenericFunctionInstantiations bounds distinct monomorphized generic functions per program.
const MaxGenericFunctionInstantiations = 1 << 12

// MaxGenericStructInstantiations bounds distinct monomorphized generic structs per program.
const MaxGenericStructInstantiations = 1 << 12

// MaxGenericEnumInstantiations bounds distinct monomorphized generic enums per program.
const MaxGenericEnumInstantiations = 1 << 12

// MaxGenericExpansionDepth bounds chains of expanding generic instantiations.
const MaxGenericExpansionDepth = 256
