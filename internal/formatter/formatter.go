// Package formatter implements a source-preserving formatter for the current Aster syntax.
package formatter

import (
	"strings"

	"github.com/aster-lang/aster/internal/lexer"
	"github.com/aster-lang/aster/internal/parser"
	"github.com/aster-lang/aster/internal/token"
)

// Format validates source syntax and returns a canonical whitespace layout.
// Raw token spellings and comments are preserved, including string escapes and
// nested block comments. Four spaces are used for indentation and output ends
// with one newline unless the input is empty.
func Format(source string) (string, error) {
	if _, err := parser.Parse(source, "<format>"); err != nil {
		return "", err
	}
	tokens, err := lexer.New(source).Lex()
	if err != nil {
		return "", err
	}

	f := state{atLineStart: true}
	for i, current := range tokens {
		f.emitTrivia(current.LeadingTrivia)
		if current.Kind == token.EOF {
			if f.pendingNewline {
				f.newline()
				f.pendingNewline = false
			}
			break
		}

		inlineElse := f.hasPrev && f.prev.Kind == token.RBrace &&
			current.Kind == token.Else && !strings.Contains(current.LeadingTrivia, "\n")
		if inlineElse {
			f.pendingNewline = false
		} else if f.pendingNewline {
			f.newline()
			f.pendingNewline = false
		}

		if current.Kind == token.RBrace {
			if !f.atLineStart {
				f.newline()
			}
			if f.indent > 0 {
				f.indent--
			}
			f.writeToken(current.Raw)
			f.pendingNewline = true
			f.setPrev(current, false)
			continue
		}

		typeArgumentContext := i >= 2 && f.hasPrev && f.prev.Kind == token.Ident && (tokens[i-2].Kind == token.Colon ||
			tokens[i-2].Kind == token.Arrow ||
			tokens[i-2].Raw == "fn" ||
			tokens[i-2].Raw == "struct" ||
			tokens[i-2].Raw == "enum")
		genericOpen := current.Kind == token.Less &&
			(f.genericDepth > 0 || (f.hasPrev && f.prev.Raw == "::") || typeArgumentContext)
		genericClose := current.Kind == token.Greater && f.genericDepth > 0

		if current.Kind == token.LBrace {
			if f.needsSpaceBefore(current, genericOpen, genericClose) {
				f.space()
			}
			f.writeToken(current.Raw)
			f.indent++
			f.newline()
			f.setPrev(current, false)
			continue
		}
		if current.Kind == token.Semicolon {
			if f.needsSpaceBefore(current, genericOpen, genericClose) {
				f.space()
			}
			f.writeToken(current.Raw)
			f.pendingNewline = true
			f.setPrev(current, false)
			continue
		}

		if f.needsSpaceBefore(current, genericOpen, genericClose) {
			f.space()
		}
		f.writeToken(current.Raw)
		if genericOpen {
			f.genericDepth++
		}
		if genericClose {
			f.genericDepth--
		}
		f.setPrev(current, isPrefixOperator(tokens, i))
		f.prevWasGenericClose = genericClose
	}

	if f.out.Len() == 0 {
		return "", nil
	}
	if f.lastByte != '\n' {
		f.newline()
	}
	return f.out.String(), nil
}

type state struct {
	out                 strings.Builder
	indent              int
	atLineStart         bool
	hasPrev             bool
	prev                token.Token
	prevWasPrefix       bool
	prevWasGenericClose bool
	pendingNewline      bool
	genericDepth        int
	lastByte            byte
}

func (f *state) setPrev(current token.Token, prefix bool) {
	f.prev = current
	f.hasPrev = true
	f.prevWasPrefix = prefix
	f.prevWasGenericClose = false
}

func (f *state) needsSpaceBefore(current token.Token, genericOpen, genericClose bool) bool {
	if !f.hasPrev {
		return false
	}
	if f.prevWasPrefix {
		return false
	}
	if f.prev.Kind == token.LParen || f.prev.Kind == token.LBracket {
		return false
	}
	// Inside a type argument list, the less-than token is a delimiter rather
	// than a comparison operator; do not insert a space after the opener.
	if f.prev.Kind == token.Less && f.genericDepth > 0 {
		return false
	}
	switch current.Kind {
	case token.RParen, token.RBracket, token.Comma, token.Semicolon, token.Colon, token.DoubleColon, token.RBrace:
		return false
	case token.LParen:
		return !f.prevWasGenericClose && f.prev.Kind != token.Ident && f.prev.Kind != token.RParen && f.prev.Kind != token.RBracket
	case token.LBracket:
		return f.prev.Kind != token.Ident && f.prev.Kind != token.RParen && f.prev.Kind != token.RBracket
	case token.LBrace:
		return true
	case token.Less:
		return !genericOpen
	case token.Greater:
		return !genericClose
	}
	if isOperator(current.Kind) || isOperator(f.prev.Kind) {
		return true
	}
	if f.prev.Kind == token.Comma || f.prev.Kind == token.Colon || f.prev.Kind == token.Arrow {
		return true
	}
	if f.prev.Kind == token.RBrace && current.Kind == token.Else {
		return true
	}
	return isWord(f.prev.Kind) && isWord(current.Kind)
}

func isOperator(kind token.Kind) bool {
	switch kind {
	case token.Assign, token.Plus, token.Minus, token.Star, token.Slash, token.Percent,
		token.Bang, token.Equal, token.NotEqual, token.Less, token.LessEqual,
		token.Greater, token.GreaterEqual, token.AndAnd, token.OrOr, token.Arrow:
		return true
	default:
		return false
	}
}

func isWord(kind token.Kind) bool {
	switch kind {
	case token.Ident, token.Int, token.Float, token.String, token.Fn, token.Struct, token.Enum,
		token.Match, token.Let, token.Mut, token.If, token.Else, token.While, token.Return,
		token.True, token.False:
		return true
	default:
		return false
	}
}

func isPrefixOperator(tokens []token.Token, index int) bool {
	current := tokens[index]
	if current.Kind == token.Bang {
		return true
	}
	if current.Kind != token.Minus {
		return false
	}
	if index == 0 {
		return true
	}
	switch tokens[index-1].Kind {
	case token.LParen, token.LBracket, token.Comma, token.Semicolon, token.Colon,
		token.Assign, token.Plus, token.Minus, token.Star, token.Slash, token.Percent,
		token.Equal, token.NotEqual, token.Less, token.LessEqual, token.Greater,
		token.GreaterEqual, token.AndAnd, token.OrOr, token.Arrow, token.Return,
		token.If, token.While:
		return true
	default:
		return false
	}
}

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

// emitTrivia discards ordinary whitespace but writes comments in their original
// order. Line comments stay with the preceding line when the source placed them
// there; block comments retain their complete raw text.
func (f *state) emitTrivia(trivia string) {
	if !strings.Contains(trivia, "/") {
		return
	}
	i := 0
	sawNewline := false
	emittedComment := false
	for i < len(trivia) {
		if isWhitespace(trivia[i]) {
			if trivia[i] == '\n' {
				sawNewline = true
			}
			i++
			continue
		}
		if strings.HasPrefix(trivia[i:], "//") {
			if sawNewline {
				f.newline()
			} else if !f.atLineStart {
				f.space()
			}
			end := i
			for end < len(trivia) && trivia[end] != '\n' {
				end++
			}
			f.writeComment(trivia[i:end])
			emittedComment = true
			f.newline()
			i = end
			if i < len(trivia) && trivia[i] == '\n' {
				i++
			}
			sawNewline = false
			continue
		}
		if strings.HasPrefix(trivia[i:], "/*") {
			if sawNewline {
				f.newline()
			} else if !f.atLineStart {
				f.space()
			}
			end := blockCommentEnd(trivia, i)
			f.writeComment(trivia[i:end])
			emittedComment = true
			i = end
			sawNewline = false
			continue
		}
		i++
	}
	if sawNewline {
		f.newline()
	} else if emittedComment && !f.atLineStart {
		f.space()
	}
}

func blockCommentEnd(source string, start int) int {
	depth := 0
	for i := start; i+1 < len(source); i++ {
		switch {
		case source[i] == '/' && source[i+1] == '*':
			depth++
			i++
		case source[i] == '*' && source[i+1] == '/':
			depth--
			i++
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(source)
}

func (f *state) writeComment(text string) {
	f.writeRaw(text)
}

func (f *state) writeToken(text string) {
	f.ensureIndent()
	f.writeRaw(text)
}

func (f *state) writeRaw(text string) {
	if text == "" {
		return
	}
	f.ensureIndent()
	f.out.WriteString(text)
	f.lastByte = text[len(text)-1]
	f.atLineStart = f.lastByte == '\n'
}

func (f *state) ensureIndent() {
	if !f.atLineStart {
		return
	}
	if f.indent > 0 {
		pad := strings.Repeat("    ", f.indent)
		f.out.WriteString(pad)
		f.lastByte = pad[len(pad)-1]
	}
	f.atLineStart = false
}

func (f *state) space() {
	if f.atLineStart || f.out.Len() == 0 {
		return
	}
	switch f.lastByte {
	case ' ', '\t', '\r', '\n':
		return
	default:
		f.out.WriteByte(' ')
		f.lastByte = ' '
	}
}

func (f *state) newline() {
	if f.out.Len() == 0 {
		f.atLineStart = true
		return
	}
	if f.lastByte == ' ' || f.lastByte == '\t' || f.lastByte == '\r' {
		trimmed := strings.TrimRight(f.out.String(), " \t\r")
		f.out.Reset()
		f.out.WriteString(trimmed)
		if len(trimmed) == 0 {
			f.lastByte = 0
		} else {
			f.lastByte = trimmed[len(trimmed)-1]
		}
	}
	if f.lastByte != '\n' {
		f.out.WriteByte('\n')
		f.lastByte = '\n'
	}
	f.atLineStart = true
}
