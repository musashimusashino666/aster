// Package diagnostic defines stable identifiers for source, runtime, bytecode, and toolchain errors.
package diagnostic

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aster-lang/aster/internal/token"
)

// Diagnostic is an error with a stable code and source position. When
// HasEndPosition is true, EndPosition is an exclusive source position.
// Error preserves the traditional message prefix and appends the code in brackets.
type Diagnostic struct {
	Code           string
	Phase          string
	Message        string
	Position       token.Position
	EndPosition    token.Position
	HasEndPosition bool
	Source         string
	Cause          error
}

// New constructs a diagnostic with a stable code, phase, position, and message.
func New(code, phase string, position token.Position, message string) *Diagnostic {
	return &Diagnostic{Code: code, Phase: phase, Position: position, Message: message}
}

// NewSpan constructs a diagnostic with an exclusive end position. The rendered
// error string intentionally keeps the legacy start-only location format.
func NewSpan(code, phase string, start, end token.Position, message string) *Diagnostic {
	return &Diagnostic{
		Code: code, Phase: phase, Position: start,
		EndPosition: end, HasEndPosition: true, Message: message,
	}
}

func (d *Diagnostic) Error() string { return d.formatError(false) }

// formatError preserves the stable Error() representation unless the caller
// explicitly requests a valid source range for user-facing output.
func (d *Diagnostic) formatError(includeEnd bool) string {
	prefix := fmt.Sprintf("%s error: %s", d.Phase, d.Message)
	if includeEnd && d.hasValidSourceSpan() {
		if d.Source != "" {
			prefix = fmt.Sprintf("%s:%d:%d-%d:%d: %s", d.Source,
				d.Position.Line, d.Position.Column, d.EndPosition.Line, d.EndPosition.Column, prefix)
		} else {
			prefix = fmt.Sprintf("%d:%d-%d:%d: %s",
				d.Position.Line, d.Position.Column, d.EndPosition.Line, d.EndPosition.Column, prefix)
		}
	} else if d.Source != "" && d.Position.Line > 0 && d.Position.Column > 0 {
		prefix = fmt.Sprintf("%s:%d:%d: %s", d.Source, d.Position.Line, d.Position.Column, prefix)
	} else if d.Position.Line > 0 && d.Position.Column > 0 {
		prefix = fmt.Sprintf("%d:%d: %s", d.Position.Line, d.Position.Column, prefix)
	} else if d.Source != "" {
		prefix = fmt.Sprintf("%s: %s", d.Source, prefix)
	}
	if d.Code == "" {
		return prefix
	}
	return fmt.Sprintf("%s [%s]", prefix, d.Code)
}

func (d *Diagnostic) hasValidSourceSpan() bool {
	if d == nil || !d.HasEndPosition {
		return false
	}
	start, end := d.Position, d.EndPosition
	if start.Offset < 0 || end.Offset < start.Offset ||
		start.Line <= 0 || start.Column <= 0 || end.Line <= 0 || end.Column <= 0 {
		return false
	}
	if end.Line < start.Line || (end.Line == start.Line && end.Column < start.Column) {
		return false
	}
	return true
}

// Render formats an error for command-line display. When an error wraps a
// Diagnostic with a valid exclusive end position, Render replaces that
// diagnostic's legacy start-only text with a source-range location. Error()
// itself remains start-only for API and compatibility-sensitive callers.
func Render(err error) string {
	if err == nil {
		return ""
	}
	var d *Diagnostic
	if !errors.As(err, &d) || !d.hasValidSourceSpan() {
		return err.Error()
	}
	legacy := d.Error()
	rendered := d.formatError(true)
	text := err.Error()
	index := strings.LastIndex(text, legacy)
	if index < 0 {
		return text
	}
	return text[:index] + rendered + text[index+len(legacy):]
}

// Unwrap preserves the original error for errors.Is/errors.As when a subsystem
// adds a stable diagnostic code around an operating-system or context failure.
func (d *Diagnostic) Unwrap() error { return d.Cause }

// NewAtSource constructs a diagnostic with a source path and start position
// when an end position is unavailable.
func NewAtSource(code, phase, source string, position token.Position, message string) *Diagnostic {
	return &Diagnostic{Code: code, Phase: phase, Source: source, Position: position, Message: message}
}

// NewAtSourceSpan constructs a source-path diagnostic with an exclusive end
// position. Its Error string remains compatible with start-only diagnostics.
func NewAtSourceSpan(code, phase, source string, start, end token.Position, message string) *Diagnostic {
	return &Diagnostic{
		Code: code, Phase: phase, Source: source, Position: start,
		EndPosition: end, HasEndPosition: true, Message: message,
	}
}

// WrapBytecode assigns a stable code to a bytecode reader/verifier error.
func WrapBytecode(err error, source string) error {
	if err == nil {
		return nil
	}
	var existing *Diagnostic
	if errors.As(err, &existing) {
		if source == "" || existing.Source != "" {
			return err
		}
		copy := *existing
		copy.Source = source
		return &copy
	}
	return &Diagnostic{
		Code:    BytecodeCode(err.Error()),
		Phase:   "bytecode",
		Source:  source,
		Message: err.Error(),
		Cause:   err,
	}
}

// WrapToolchain assigns a stable code while preserving the underlying error.
func WrapToolchain(err error) error {
	if err == nil {
		return nil
	}
	var existing *Diagnostic
	if errors.As(err, &existing) {
		return err
	}
	return &Diagnostic{
		Code:    ToolchainCode(err.Error()),
		Phase:   "toolchain",
		Message: err.Error(),
		Cause:   err,
	}
}

// RuntimeCode maps runtime failure classes to stable public identifiers.
// The messages are shared by the VM and generated native runtime; changes to
// message wording must update this classifier and the generated counterpart.
func RuntimeCode(message string) string {
	switch {
	case strings.Contains(message, "assertion failed"):
		return RuntimeAssertionFailed
	case strings.Contains(message, "division by zero"):
		return RuntimeDivisionByZero
	case strings.Contains(message, "remainder by zero"):
		return RuntimeRemainderByZero
	case strings.Contains(message, "overflow"):
		return RuntimeArithmeticOverflow
	case strings.Contains(message, "non-finite floating-point result"):
		return RuntimeNonFiniteFloat
	case strings.Contains(message, "array index") && strings.Contains(message, "out of bounds"):
		return RuntimeArrayIndexBounds
	case strings.Contains(message, "slice bounds"):
		return RuntimeSliceBounds
	case strings.Contains(message, "out of range for i64 conversion"):
		return RuntimeNumericConversionRange
	case strings.Contains(message, "string length limit exceeded"):
		return RuntimeStringLimit
	case strings.Contains(message, "printed value exceeds"), strings.Contains(message, "printed array nesting"):
		return RuntimeRenderedOutputLimit
	case strings.Contains(message, "stdout byte limit exceeded"):
		return RuntimeStdoutLimit
	case strings.Contains(message, "call depth limit exceeded"):
		return RuntimeCallDepthLimit
	case strings.Contains(message, "instruction budget exceeded"):
		return RuntimeInstructionBudget
	case strings.Contains(message, "process memory limit exceeded"):
		return RuntimeProcessMemoryLimit
	case strings.Contains(message, "cumulative CPU-time budget exceeded"):
		return RuntimeCPUTimeLimit
	case strings.Contains(message, "operand stack limit exceeded"):
		return RuntimeOperandStackLimit
	case strings.Contains(message, "active local slot limit exceeded"):
		return RuntimeLocalSlotLimit
	case strings.HasPrefix(message, "write output:"):
		return RuntimeOutputWrite
	case strings.Contains(message, "invalid array element count"), strings.Contains(message, "array length limit exceeded"):
		return RuntimeArrayLimit
	default:
		return RuntimeGeneric
	}
}

// BytecodeCode classifies reader and verifier failures independently of their
// human-readable wording, which is retained in the returned diagnostic.
func BytecodeCode(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "unsupported bytecode version"):
		return BytecodeVersion
	case strings.Contains(lower, "not an aster bytecode"), strings.Contains(lower, "invalid bytecode format"):
		return BytecodeFormat
	case strings.Contains(lower, "invalid json"), strings.Contains(lower, "unexpected end of json"),
		strings.Contains(lower, "trailing json"), strings.Contains(lower, "decode"),
		strings.Contains(lower, "invalid character"), strings.Contains(lower, "unexpected eof"),
		strings.Contains(lower, "cannot unmarshal"), strings.Contains(lower, "unexpected end of input"):
		return BytecodeDecode
	case strings.Contains(lower, "safety limit"), strings.Contains(lower, "exceeds limit"),
		strings.Contains(lower, "exceeds safety"), strings.Contains(lower, "too large"):
		return BytecodeResourceLimit
	case strings.Contains(lower, "abstract"), strings.Contains(lower, "operand stack"),
		strings.Contains(lower, "type mismatch"), strings.Contains(lower, "type "),
		strings.Contains(lower, "local "), strings.Contains(lower, "return type"):
		return BytecodeTypeValidation
	case strings.Contains(lower, "function"), strings.Contains(lower, "instruction"),
		strings.Contains(lower, "entry"), strings.Contains(lower, "opcode"),
		strings.Contains(lower, "constant"), strings.Contains(lower, "jump"),
		strings.Contains(lower, "invalid"):
		return BytecodeStructure
	default:
		return BytecodeGeneric
	}
}

// ToolchainCode classifies external Go toolchain invocation and filesystem
// failures. Compiler output remains in the message for actionable debugging.
func ToolchainCode(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "canceled"), strings.Contains(lower, "cancelled"):
		return ToolchainCanceled
	case strings.Contains(lower, "resource isolation unavailable"):
		return ToolchainResourceIsolationUnavailable
	case strings.Contains(lower, "timed out"), strings.Contains(lower, "deadline exceeded"):
		return ToolchainTimeout
	case strings.Contains(lower, "not found in path"), strings.Contains(lower, "requires go 1.23"),
		strings.Contains(lower, "requires gofmt"), strings.Contains(lower, "executable \"go\""):
		return ToolchainUnavailable
	case strings.Contains(lower, "format generated code"):
		return ToolchainFormat
	case strings.Contains(lower, "native build"), strings.Contains(lower, "go build"):
		return ToolchainBuild
	case strings.Contains(lower, "native program"), strings.Contains(lower, "executable run"):
		return ToolchainRun
	case strings.Contains(lower, "directory"), strings.Contains(lower, "temporary"),
		strings.Contains(lower, "output"), strings.Contains(lower, "write "),
		strings.Contains(lower, "replace "):
		return ToolchainFilesystem
	default:
		return ToolchainGeneric
	}
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
	TypeNumericCastArity        = "TYP050"
	TypeNumericCastOperand      = "TYP051"
	TypeDuplicateStruct         = "TYP052"
	TypeDuplicateStructField    = "TYP053"
	TypeInvalidStructField      = "TYP054"
	TypeFieldOnNonStruct        = "TYP055"
	TypeUnknownStructField      = "TYP056"
	TypeStructConstructorArity  = "TYP057"
	TypeStructFieldMismatch     = "TYP058"
	TypeRecursiveStruct         = "TYP059"
	TypePrintStructUnsupported  = "TYP060"
	TypeInvalidStructName       = "TYP061"
	TypeInvalidStructFieldName  = "TYP062"

	TypeDuplicateEnum                   = "TYP063"
	TypeDuplicateEnumVariant            = "TYP064"
	TypeInvalidEnumPayload              = "TYP065"
	TypeUnknownEnumVariant              = "TYP066"
	TypeEnumConstructorArity            = "TYP067"
	TypeEnumPayloadMismatch             = "TYP068"
	TypeMatchNonEnum                    = "TYP069"
	TypeMatchVariantEnum                = "TYP070"
	TypeMatchPayloadArity               = "TYP071"
	TypeMatchDuplicateArm               = "TYP072"
	TypeMatchUnreachableArm             = "TYP073"
	TypeMatchNonExhaustive              = "TYP074"
	TypeMatchArmTypeMismatch            = "TYP075"
	TypeMatchDuplicateBinding           = "TYP076"
	TypeRecursiveEnumLayout             = "TYP077"
	TypeResourceLimit                   = "TYP078"
	TypeInvalidGenericParameter         = "TYP079"
	TypeGenericArgumentCount            = "TYP080"
	TypeGenericInference                = "TYP081"
	TypeGenericArgumentsOnNonGeneric    = "TYP082"
	TypeGenericInstantiationLimit       = "TYP083"
	TypeGenericStructArgumentCount      = "TYP084"
	TypeGenericStructInference          = "TYP085"
	TypeGenericStructInstantiationLimit = "TYP086"
	TypeGenericEnumArgumentCount        = "TYP087"
	TypeGenericEnumInference            = "TYP088"
	TypeGenericEnumInstantiationLimit   = "TYP089"
)

const (
	RuntimeGeneric                = "RUN000"
	RuntimeAssertionFailed        = "RUN001"
	RuntimeArithmeticOverflow     = "RUN002"
	RuntimeDivisionByZero         = "RUN003"
	RuntimeRemainderByZero        = "RUN004"
	RuntimeNonFiniteFloat         = "RUN005"
	RuntimeArrayIndexBounds       = "RUN006"
	RuntimeSliceBounds            = "RUN007"
	RuntimeNumericConversionRange = "RUN008"
	RuntimeStringLimit            = "RUN009"
	RuntimeRenderedOutputLimit    = "RUN010"
	RuntimeStdoutLimit            = "RUN011"
	RuntimeCallDepthLimit         = "RUN012"
	RuntimeInstructionBudget      = "RUN013"
	RuntimeOperandStackLimit      = "RUN014"
	RuntimeLocalSlotLimit         = "RUN015"
	RuntimeOutputWrite            = "RUN016"
	RuntimeArrayLimit             = "RUN017"
	RuntimeProcessMemoryLimit     = "RUN018"
	RuntimeCPUTimeLimit           = "RUN019"

	BytecodeGeneric        = "BC000"
	BytecodeFormat         = "BC001"
	BytecodeVersion        = "BC002"
	BytecodeResourceLimit  = "BC003"
	BytecodeDecode         = "BC004"
	BytecodeStructure      = "BC005"
	BytecodeTypeValidation = "BC006"

	ToolchainGeneric                      = "TOOL000"
	ToolchainCanceled                     = "TOOL001"
	ToolchainTimeout                      = "TOOL002"
	ToolchainUnavailable                  = "TOOL003"
	ToolchainFormat                       = "TOOL004"
	ToolchainBuild                        = "TOOL005"
	ToolchainRun                          = "TOOL006"
	ToolchainFilesystem                   = "TOOL007"
	ToolchainResourceIsolationUnavailable = "TOOL008"
)
