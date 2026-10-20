// Package langtypes defines type-name constraints shared by the parser,
// checker, bytecode verifier, and code generators.
package langtypes

import "strings"

func validIdentifier(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_') {
				return false
			}
			continue
		}
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}

// MaxNestingDepth limits syntactic nesting of type names such as
// Array<Array<i64>>. It is distinct from limits.MaxArrayNestingDepth, which
// bounds runtime array-value traversal and display. Keeping this in a
// dependency-light package avoids different acceptance rules in the source
// frontend and bytecode verifier.
const MaxNestingDepth = 256

// MaxTypeNameBytes is the longest valid spelling of a maximally nested type.
const MaxTypeNameBytes = MaxNestingDepth*7 + len("string")

// ValidName reports whether s is a supported source-level type name.
func ValidName(s string) bool {
	if len(s) == 0 || len(s) > MaxTypeNameBytes {
		return false
	}
	depth := 0
	for strings.HasPrefix(s, "Array<") && strings.HasSuffix(s, ">") {
		depth++
		if depth > MaxNestingDepth {
			return false
		}
		s = s[len("Array<") : len(s)-1]
	}
	if depth > 0 && s == "unit" {
		return false
	}
	switch s {
	case "i64", "f64", "bool", "string", "unit":
		return true
	default:
		// Nominal type names are validated against the declaration table by
		// the checker and bytecode verifier. This package validates spelling.
		return validIdentifier(s) && s != "Array"
	}
}
