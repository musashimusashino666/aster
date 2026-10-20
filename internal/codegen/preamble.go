package codegen

import _ "embed"

//go:embed preamble/runtime.go.tmpl
var nativePreamble string
