// Package lexer converts Aster source into tokens. It intentionally has no
// parser dependencies so tokenization can be tested independently.
package lexer

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/aster-lang/aster/internal/diagnostic"
	"github.com/aster-lang/aster/internal/token"
)

type Lexer struct {
	source string
	offset int
	line   int
	column int
}

func New(source string) *Lexer {
	l := &Lexer{source: source, line: 1, column: 1}
	// A UTF-8 BOM is metadata at the start of a file, not source syntax.
	// Keep the source byte offsets intact while beginning lexing after it.
	if strings.HasPrefix(source, "\uFEFF") {
		l.offset = len("\uFEFF")
	}
	return l
}

func (l *Lexer) Lex() ([]token.Token, error) {
	if !utf8.ValidString(l.source) {
		return nil, diagnostic.New(diagnostic.LexerInvalidUTF8, "lexer", token.Position{Line: 1, Column: 1}, "source is not valid UTF-8")
	}
	var out []token.Token
	for {
		triviaStart := l.offset
		if err := l.skipTrivia(); err != nil {
			return nil, err
		}
		leadingTrivia := l.source[triviaStart:l.offset]
		pos := l.position()
		if l.atEnd() {
			out = append(out, token.Token{Kind: token.EOF, Pos: pos, End: pos, LeadingTrivia: leadingTrivia})
			return out, nil
		}
		start := l.offset
		ch := l.advance()
		if isIdentStart(ch) {
			for isIdentContinue(l.peek()) {
				l.advance()
			}
			text := l.source[start:l.offset]
			out = append(out, token.Token{Kind: keywordKind(text), Lexeme: text, Pos: pos, End: l.position(), Raw: text, LeadingTrivia: leadingTrivia})
			continue
		}
		if ch >= '0' && ch <= '9' {
			kind := token.Int
			for isDigit(l.peek()) {
				l.advance()
			}
			if l.peek() == '.' && isDigit(l.peekNext()) {
				kind = token.Float
				l.advance()
				for isDigit(l.peek()) {
					l.advance()
				}
			}
			text := l.source[start:l.offset]
			if kind == token.Int {
				if _, err := strconv.ParseInt(text, 10, 64); err != nil && text != "9223372036854775808" {
					return nil, l.errorAt(pos, diagnostic.LexerIntegerOutOfRange, "integer literal is outside the i64 range")
				}
			}
			if kind == token.Float {
				if _, err := strconv.ParseFloat(text, 64); err != nil {
					return nil, l.errorAt(pos, diagnostic.LexerInvalidFloat, "invalid floating-point literal")
				}
			}
			out = append(out, token.Token{Kind: kind, Lexeme: text, Pos: pos, End: l.position(), Raw: text, LeadingTrivia: leadingTrivia})
			continue
		}
		if ch == '"' {
			var b strings.Builder
			closed := false
			for !l.atEnd() {
				c := l.advance()
				if c == '"' {
					closed = true
					break
				}
				if c == '\n' {
					return nil, l.errorAt(pos, diagnostic.LexerNewlineInString, "newline in string literal")
				}
				if c == '\\' {
					if l.atEnd() {
						break
					}
					esc := l.advance()
					switch esc {
					case 'n':
						b.WriteByte('\n')
					case 'r':
						b.WriteByte('\r')
					case 't':
						b.WriteByte('\t')
					case '0':
						b.WriteByte('\x00')
					case '\\':
						b.WriteByte('\\')
					case '"':
						b.WriteByte('"')
					default:
						return nil, l.errorAt(pos, diagnostic.LexerUnsupportedEscape, fmt.Sprintf("unsupported escape sequence \\%c", esc))
					}
				} else {
					b.WriteByte(c)
				}
			}
			if !closed {
				return nil, l.errorAt(pos, diagnostic.LexerUnterminatedString, "unterminated string literal")
			}
			out = append(out, token.Token{Kind: token.String, Lexeme: b.String(), Pos: pos, End: l.position(), Raw: l.source[start:l.offset], LeadingTrivia: leadingTrivia})
			continue
		}
		kind := token.Illegal
		switch ch {
		case '(':
			kind = token.LParen
		case ')':
			kind = token.RParen
		case '[':
			kind = token.LBracket
		case ']':
			kind = token.RBracket
		case '{':
			kind = token.LBrace
		case '}':
			kind = token.RBrace
		case ',':
			kind = token.Comma
		case ':':
			if l.match(':') {
				kind = token.DoubleColon
			} else {
				kind = token.Colon
			}
		case ';':
			kind = token.Semicolon
		case '.':
			kind = token.Dot
		case '+':
			kind = token.Plus
		case '-':
			if l.match('>') {
				kind = token.Arrow
			} else {
				kind = token.Minus
			}
		case '*':
			kind = token.Star
		case '/':
			kind = token.Slash
		case '%':
			kind = token.Percent
		case '=':
			if l.match('=') {
				kind = token.Equal
			} else if l.match('>') {
				kind = token.FatArrow
			} else {
				kind = token.Assign
			}
		case '!':
			if l.match('=') {
				kind = token.NotEqual
			} else {
				kind = token.Bang
			}
		case '<':
			if l.match('=') {
				kind = token.LessEqual
			} else {
				kind = token.Less
			}
		case '>':
			if l.match('=') {
				kind = token.GreaterEqual
			} else {
				kind = token.Greater
			}
		case '&':
			if l.match('&') {
				kind = token.AndAnd
			}
		case '|':
			if l.match('|') {
				kind = token.OrOr
			}
		}
		if kind == token.Illegal {
			message := fmt.Sprintf("unexpected character %q", ch)
			if ch >= utf8.RuneSelf {
				r, _ := utf8.DecodeRuneInString(l.source[start:])
				message = fmt.Sprintf("unexpected character U+%04X %q", r, r)
			}
			return nil, l.errorAt(pos, diagnostic.LexerUnexpectedCharacter, message)
		}
		out = append(out, token.Token{Kind: kind, Lexeme: l.source[start:l.offset], Pos: pos, End: l.position(), Raw: l.source[start:l.offset], LeadingTrivia: leadingTrivia})
	}
}

func keywordKind(s string) token.Kind {
	switch s {
	case "fn":
		return token.Fn
	case "struct":
		return token.Struct
	case "enum":
		return token.Enum
	case "match":
		return token.Match
	case "let":
		return token.Let
	case "mut":
		return token.Mut
	case "if":
		return token.If
	case "else":
		return token.Else
	case "while":
		return token.While
	case "return":
		return token.Return
	case "true":
		return token.True
	case "false":
		return token.False
	default:
		return token.Ident
	}
}
func isIdentStart(r byte) bool    { return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }
func isIdentContinue(r byte) bool { return isIdentStart(r) || isDigit(r) }
func isDigit(r byte) bool         { return r >= '0' && r <= '9' }
func (l *Lexer) atEnd() bool      { return l.offset >= len(l.source) }
func (l *Lexer) peek() byte {
	if l.atEnd() {
		return 0
	}
	return l.source[l.offset]
}
func (l *Lexer) peekNext() byte {
	if l.offset+1 >= len(l.source) {
		return 0
	}
	return l.source[l.offset+1]
}
func (l *Lexer) advance() byte {
	c := l.source[l.offset]
	l.offset++
	if c == '\n' {
		l.line++
		l.column = 1
	} else {
		l.column++
	}
	return c
}
func (l *Lexer) match(expected byte) bool {
	if l.atEnd() || l.source[l.offset] != expected {
		return false
	}
	l.advance()
	return true
}
func (l *Lexer) position() token.Position {
	return token.Position{Offset: l.offset, Line: l.line, Column: l.column}
}
func (l *Lexer) errorAt(p token.Position, code, message string) error {
	return diagnostic.New(code, "lexer", p, message)
}
func (l *Lexer) skipTrivia() error {
	for !l.atEnd() {
		r, size := utf8.DecodeRuneInString(l.source[l.offset:])
		if unicode.IsSpace(r) {
			// Consume the full UTF-8 encoding while preserving normal
			// line accounting for ASCII newline characters.
			for i := 0; i < size; i++ {
				l.advance()
			}
			continue
		}
		c := l.peek()
		if c == '/' && l.peekNext() == '/' {
			for !l.atEnd() && l.peek() != '\n' {
				l.advance()
			}
			continue
		}
		if c == '/' && l.peekNext() == '*' {
			pos := l.position()
			l.advance()
			l.advance()
			depth := 1
			for !l.atEnd() && depth > 0 {
				if l.peek() == '/' && l.peekNext() == '*' {
					l.advance()
					l.advance()
					depth++
					continue
				}
				if l.peek() == '*' && l.peekNext() == '/' {
					l.advance()
					l.advance()
					depth--
					continue
				}
				l.advance()
			}
			if depth != 0 {
				return l.errorAt(pos, diagnostic.LexerUnterminatedBlockComment, "unterminated block comment")
			}
			continue
		}
		return nil
	}
	return nil
}
