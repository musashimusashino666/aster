package codegen

import _ "embed"

//go:embed preamble/runtime.go
var nativePreamble string
