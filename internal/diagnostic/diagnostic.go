// Package diagnostic defines stable identifiers for source-front-end errors.
package diagnostic

import (
	"fmt"

	"github.com/aster-lang/aster/internal/token"
)

// Diagnostic is a source-front-end error with a stable code and source position.
// Error preserves the traditional message prefix and appends the code in brackets.
type Diagnostic struct {
	Code     string
	Phase    string
	Message  string
	Position token.Position
}

// New constructs a diagnostic with a stable code, phase, position, and message.
func New(code, phase string, position token.Position, message string) *Diagnostic {
	return &Diagnostic{Code: code, Phase: phase, Position: position, Message: message}
}

func (d *Diagnostic) Error() string {
	prefix := fmt.Sprintf("%s error: %s", d.Phase, d.Message)
	if d.Position.Line > 0 && d.Position.Column > 0 {
		prefix = fmt.Sprintf("%d:%d: %s", d.Position.Line, d.Position.Column, prefix)
	}
	if d.Code == "" {
		return prefix
	}
	return fmt.Sprintf("%s [%s]", prefix, d.Code)
}

const (
	LexerGeneric                  = "LEX000"
	LexerInvalidUTF8              = "LEX001"
	LexerIntegerOutOfRange        = "LEX002"
	LexerInvalidFloat             = "LEX003"
	LexerNewlineInString          = "LEX004"
	LexerUnsupportedEscape        = "LEX005"
	LexerUnterminatedString       = "LEX006"
	LexerUnexpectedCharacter      = "LEX007"
	LexerUnterminatedBlockComment = "LEX008"

	ParserGeneric                    = "PAR000"
	ParserSyntaxNesting              = "PAR001"
	ParserExpressionNesting          = "PAR002"
	ParserNoProgress                 = "PAR003"
	ParserReservedInternalName       = "PAR004"
	ParserReservedMainName           = "PAR005"
	ParserTypeNesting                = "PAR006"
	ParserArrayAssignmentUnsupported = "PAR007"
	ParserIntegerOutOfRange          = "PAR008"
	ParserInvalidFloat               = "PAR009"
	ParserExpectedExpression         = "PAR010"

	TypeGeneric                 = "TYP000"
	TypeDuplicateFunction       = "TYP001"
	TypeReservedParameter       = "TYP002"
	TypeInvalidTypeAnnotation   = "TYP003"
	TypeUnitParameter           = "TYP004"
	TypeReservedFunction        = "TYP005"
	TypeDuplicateParameter      = "TYP006"
	TypeMissingReturn           = "TYP007"
	TypeUnreachableStatement    = "TYP008"
	TypeReservedBinding         = "TYP009"
	TypeDuplicateBinding        = "TYP010"
	TypeUnitBinding             = "TYP011"
	TypeBindingTypeMismatch     = "TYP012"
	TypeUnitValueBinding        = "TYP013"
	TypeUndefinedVariable       = "TYP014"
	TypeImmutableAssignment     = "TYP015"
	TypeAssignmentTypeMismatch  = "TYP016"
	TypeIfCondition             = "TYP017"
	TypeWhileCondition          = "TYP018"
	TypeReturnOutsideFunction   = "TYP019"
	TypeReturnMismatch          = "TYP020"
	TypeUnsupportedStatement    = "TYP021"
	TypeEmptyArrayInference     = "TYP022"
	TypeUnitArrayElement        = "TYP023"
	TypeHeterogeneousArray      = "TYP024"
	TypeNestingLimit            = "TYP025"
	TypeIndexNonArray           = "TYP026"
	TypeIndexType               = "TYP027"
	TypeUnaryMinusOperand       = "TYP028"
	TypeNotOperand              = "TYP029"
	TypePlusOperands            = "TYP030"
	TypeNumericOperands         = "TYP031"
	TypeRemainderOperands       = "TYP032"
	TypeOperatorOperands        = "TYP033"
	TypeComparisonOperands      = "TYP034"
	TypeLogicalOperands         = "TYP035"
	TypeCallTarget              = "TYP036"
	TypePrintArity              = "TYP037"
	TypePrintUnit               = "TYP038"
	TypeLenArity                = "TYP039"
	TypeLenArgument             = "TYP040"
	TypeAssertArity             = "TYP041"
	TypeAssertArgument          = "TYP042"
	TypeUndefinedFunction       = "TYP043"
	TypeFunctionArity           = "TYP044"
	TypeArgumentMismatch        = "TYP045"
	TypeUnsupportedExpression   = "TYP046"
	TypeIndexAssignmentMismatch = "TYP047"
	TypeSliceNonArray           = "TYP048"
	TypeSliceBoundType          = "TYP049"
)
