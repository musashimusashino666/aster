// Package token defines Aster's lexical tokens and source locations.
package token

import "fmt"

type Kind string

const (
	Illegal Kind = "ILLEGAL"
	EOF     Kind = "EOF"
	Ident   Kind = "IDENT"
	Int     Kind = "INT"
	Float   Kind = "FLOAT"
	String  Kind = "STRING"

	Fn     Kind = "fn"
	Struct Kind = "struct"
	Enum   Kind = "enum"
	Match  Kind = "match"
	Let    Kind = "let"
	Mut    Kind = "mut"
	If     Kind = "if"
	Else   Kind = "else"
	While  Kind = "while"
	Return Kind = "return"
	True   Kind = "true"
	False  Kind = "false"

	LParen    Kind = "("
	RParen    Kind = ")"
	LBracket  Kind = "["
	RBracket  Kind = "]"
	LBrace    Kind = "{"
	RBrace    Kind = "}"
	Comma     Kind = ","
	Colon     Kind = ":"
	DoubleColon Kind = "::"
	FatArrow Kind = "=>"
	Semicolon Kind = ";"
	Dot       Kind = "."
	Arrow     Kind = "->"

	Assign       Kind = "="
	Plus         Kind = "+"
	Minus        Kind = "-"
	Star         Kind = "*"
	Slash        Kind = "/"
	Percent      Kind = "%"
	Bang         Kind = "!"
	Equal        Kind = "=="
	NotEqual     Kind = "!="
	Less         Kind = "<"
	LessEqual    Kind = "<="
	Greater      Kind = ">"
	GreaterEqual Kind = ">="
	AndAnd       Kind = "&&"
	OrOr         Kind = "||"
)

type Position struct {
	Offset int `json:"offset"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

type Token struct {
	Kind          Kind     `json:"kind"`
	Lexeme        string   `json:"lexeme"`
	Pos           Position `json:"pos"`
	Raw           string   `json:"-"`
	LeadingTrivia string   `json:"-"`
}

func (t Token) String() string {
	return fmt.Sprintf("%s(%q) at %d:%d", t.Kind, t.Lexeme, t.Pos.Line, t.Pos.Column)
}
