// Package parser implements a recursive-descent parser with Pratt expression parsing.
package parser

import (
	"fmt"
	"strconv"

	"github.com/aster-lang/aster/internal/ast"
	"github.com/aster-lang/aster/internal/langtypes"
	"github.com/aster-lang/aster/internal/lexer"
	"github.com/aster-lang/aster/internal/token"
)

type Parser struct {
	tokens  []token.Token
	current int
}

func Parse(source, sourceName string) (*ast.Program, error) {
	toks, err := lexer.New(source).Lex()
	if err != nil {
		return nil, err
	}
	p := &Parser{tokens: toks}
	program := &ast.Program{SourceName: sourceName}
	for !p.check(token.EOF) {
		before := p.current
		d, err := p.declaration()
		if err != nil {
			return nil, err
		}
		program.Decls = append(program.Decls, d)
		if p.current == before {
			return nil, p.errorAt(p.peek(), "parser made no progress")
		}
	}
	return program, nil
}
func (p *Parser) declaration() (ast.Decl, error) {
	if p.match(token.Fn) {
		return p.function(p.previous())
	}
	s, err := p.statement()
	if err != nil {
		return nil, err
	}
	return &ast.StmtDecl{Statement: s}, nil
}
func (p *Parser) function(start token.Token) (ast.Decl, error) {
	name, err := p.consume(token.Ident, "expected function name after 'fn'")
	if err != nil {
		return nil, err
	}
	if len(name.Lexeme) >= 2 && name.Lexeme[0] == '_' && name.Lexeme[1] == '_' {
		return nil, p.errorAt(name, "function names beginning with '__' are reserved for compiler internals")
	}
	if name.Lexeme == "main" {
		return nil, p.errorAt(name, "'main' is reserved for the top-level program")
	}
	if _, err = p.consume(token.LParen, "expected '(' after function name"); err != nil {
		return nil, err
	}
	params := []ast.Param{}
	if !p.check(token.RParen) {
		for {
			paramName, e := p.consume(token.Ident, "expected parameter name")
			if e != nil {
				return nil, e
			}
			if _, e = p.consume(token.Colon, "expected ':' after parameter name"); e != nil {
				return nil, e
			}
			typ, e := p.typeRef()
			if e != nil {
				return nil, e
			}
			params = append(params, ast.Param{At: paramName.Pos, Name: paramName.Lexeme, Type: typ})
			if !p.match(token.Comma) {
				break
			}
		}
	}
	if _, err = p.consume(token.RParen, "expected ')' after parameters"); err != nil {
		return nil, err
	}
	var ret *ast.TypeRef
	if p.match(token.Arrow) {
		r, e := p.typeRef()
		if e != nil {
			return nil, e
		}
		ret = &r
	}
	body, err := p.block()
	if err != nil {
		return nil, err
	}
	return &ast.Function{At: start.Pos, Name: name.Lexeme, Params: params, ReturnType: ret, Body: body}, nil
}
func (p *Parser) typeRef() (ast.TypeRef, error) {
	return p.typeRefDepth(0)
}

func (p *Parser) typeRefDepth(depth int) (ast.TypeRef, error) {
	t, err := p.consume(token.Ident, "expected type name")
	if err != nil {
		return ast.TypeRef{}, err
	}
	ref := ast.TypeRef{Name: t.Lexeme, At: t.Pos}
	if p.match(token.Less) {
		if depth >= langtypes.MaxNestingDepth {
			return ast.TypeRef{}, p.errorAt(t, "type nesting exceeds limit %d", langtypes.MaxNestingDepth)
		}
		element, e := p.typeRefDepth(depth + 1)
		if e != nil {
			return ast.TypeRef{}, e
		}
		if _, e = p.consume(token.Greater, "expected '>' after generic type argument"); e != nil {
			return ast.TypeRef{}, e
		}
		ref.Element = &element
	}
	return ref, nil
}
func (p *Parser) statement() (ast.Stmt, error) {
	if p.match(token.LBrace) {
		return p.blockAfterOpen(p.previous())
	}
	if p.match(token.Let) {
		return p.letStatement(p.previous())
	}
	if p.match(token.If) {
		return p.ifStatement(p.previous())
	}
	if p.match(token.While) {
		return p.whileStatement(p.previous())
	}
	if p.match(token.Return) {
		return p.returnStatement(p.previous())
	}
	if p.indexedAssignmentAhead() {
		return nil, p.errorAt(p.peek(), "array element assignment is not supported in Aster 0.1")
	}
	if p.check(token.Ident) && p.checkNext(token.Assign) {
		id := p.advance()
		p.advance()
		value, err := p.expression(0)
		if err != nil {
			return nil, err
		}
		if _, err = p.consume(token.Semicolon, "expected ';' after assignment"); err != nil {
			return nil, err
		}
		return &ast.AssignStmt{At: id.Pos, Name: id.Lexeme, Value: value}, nil
	}
	expr, err := p.expression(0)
	if err != nil {
		return nil, err
	}
	if _, err = p.consume(token.Semicolon, "expected ';' after expression"); err != nil {
		return nil, err
	}
	return &ast.ExprStmt{At: expr.Pos(), Expr: expr}, nil
}

// indexedAssignmentAhead recognizes the unsupported target form `name[index] = value`
// to return a direct, actionable diagnostic instead of a generic missing-semicolon error.
func (p *Parser) indexedAssignmentAhead() bool {
	if !p.check(token.Ident) || !p.checkNext(token.LBracket) {
		return false
	}
	depth := 0
	for i := p.current + 1; i < len(p.tokens); i++ {
		switch p.tokens[i].Kind {
		case token.LBracket:
			depth++
		case token.RBracket:
			depth--
			if depth == 0 {
				return i+1 < len(p.tokens) && p.tokens[i+1].Kind == token.Assign
			}
		}
	}
	return false
}

func (p *Parser) letStatement(start token.Token) (ast.Stmt, error) {
	mutable := p.match(token.Mut)
	name, err := p.consume(token.Ident, "expected variable name after 'let'")
	if err != nil {
		return nil, err
	}
	var annotation *ast.TypeRef
	if p.match(token.Colon) {
		t, e := p.typeRef()
		if e != nil {
			return nil, e
		}
		annotation = &t
	}
	if _, err = p.consume(token.Assign, "expected '=' in variable declaration"); err != nil {
		return nil, err
	}
	value, err := p.expression(0)
	if err != nil {
		return nil, err
	}
	if _, err = p.consume(token.Semicolon, "expected ';' after variable declaration"); err != nil {
		return nil, err
	}
	return &ast.LetStmt{At: start.Pos, Name: name.Lexeme, Mutable: mutable, Annotation: annotation, Value: value}, nil
}
func (p *Parser) ifStatement(start token.Token) (ast.Stmt, error) {
	cond, err := p.expression(0)
	if err != nil {
		return nil, err
	}
	then, err := p.block()
	if err != nil {
		return nil, err
	}
	var otherwise ast.Stmt
	if p.match(token.Else) {
		if p.match(token.If) {
			otherwise, err = p.ifStatement(p.previous())
		} else {
			otherwise, err = p.block()
		}
		if err != nil {
			return nil, err
		}
	}
	return &ast.IfStmt{At: start.Pos, Condition: cond, Then: then, Else: otherwise}, nil
}
func (p *Parser) whileStatement(start token.Token) (ast.Stmt, error) {
	cond, err := p.expression(0)
	if err != nil {
		return nil, err
	}
	body, err := p.block()
	if err != nil {
		return nil, err
	}
	return &ast.WhileStmt{At: start.Pos, Condition: cond, Body: body}, nil
}
func (p *Parser) returnStatement(start token.Token) (ast.Stmt, error) {
	var value ast.Expr
	if !p.check(token.Semicolon) {
		e, err := p.expression(0)
		if err != nil {
			return nil, err
		}
		value = e
	}
	if _, err := p.consume(token.Semicolon, "expected ';' after return"); err != nil {
		return nil, err
	}
	return &ast.ReturnStmt{At: start.Pos, Value: value}, nil
}
func (p *Parser) block() (*ast.BlockStmt, error) {
	t, err := p.consume(token.LBrace, "expected '{'")
	if err != nil {
		return nil, err
	}
	return p.blockAfterOpen(t)
}
func (p *Parser) blockAfterOpen(start token.Token) (*ast.BlockStmt, error) {
	b := &ast.BlockStmt{At: start.Pos}
	for !p.check(token.RBrace) && !p.check(token.EOF) {
		before := p.current
		s, err := p.statement()
		if err != nil {
			return nil, err
		}
		b.Statements = append(b.Statements, s)
		if before == p.current {
			return nil, p.errorAt(p.peek(), "parser made no progress in block")
		}
	}
	if _, err := p.consume(token.RBrace, "expected '}' to close block"); err != nil {
		return nil, err
	}
	return b, nil
}

const (
	precOr       = 1
	precAnd      = 2
	precEquality = 3
	precCompare  = 4
	precTerm     = 5
	precFactor   = 6
	precUnary    = 7
	precCall     = 8
)

func precedence(k token.Kind) int {
	switch k {
	case token.OrOr:
		return precOr
	case token.AndAnd:
		return precAnd
	case token.Equal, token.NotEqual:
		return precEquality
	case token.Less, token.LessEqual, token.Greater, token.GreaterEqual:
		return precCompare
	case token.Plus, token.Minus:
		return precTerm
	case token.Star, token.Slash, token.Percent:
		return precFactor
	default:
		return 0
	}
}
func (p *Parser) expression(min int) (ast.Expr, error) {
	var left ast.Expr
	t := p.advance()
	switch t.Kind {
	case token.Int:
		n, err := strconv.ParseInt(t.Lexeme, 10, 64)
		if err != nil {
			if t.Lexeme == "9223372036854775808" {
				return nil, p.errorAt(t, "integer literal out of range; this magnitude is valid only immediately after unary '-' (write -9223372036854775808)")
			}
			return nil, p.errorAt(t, "integer literal out of range")
		}
		left = &ast.LiteralExpr{At: t.Pos, Kind: "i64", Text: t.Lexeme, Int: n}
	case token.Float:
		n, err := strconv.ParseFloat(t.Lexeme, 64)
		if err != nil {
			return nil, p.errorAt(t, "invalid float")
		}
		left = &ast.LiteralExpr{At: t.Pos, Kind: "f64", Text: t.Lexeme, Float: n}
	case token.String:
		left = &ast.LiteralExpr{At: t.Pos, Kind: "string", Text: t.Lexeme, String: t.Lexeme}
	case token.True:
		left = &ast.LiteralExpr{At: t.Pos, Kind: "bool", Text: "true", Bool: true}
	case token.False:
		left = &ast.LiteralExpr{At: t.Pos, Kind: "bool", Text: "false", Bool: false}
	case token.Ident:
		left = &ast.IdentifierExpr{At: t.Pos, Name: t.Lexeme}
	case token.LBracket:
		elements := []ast.Expr{}
		if !p.check(token.RBracket) {
			for {
				item, err := p.expression(0)
				if err != nil {
					return nil, err
				}
				elements = append(elements, item)
				if !p.match(token.Comma) {
					break
				}
			}
		}
		if _, err := p.consume(token.RBracket, "expected ']' after array elements"); err != nil {
			return nil, err
		}
		left = &ast.ArrayExpr{At: t.Pos, Elements: elements}
	case token.Minus:
		// The positive magnitude 2^63 is only valid as part of the i64 minimum
		// literal. The lexer permits this one magnitude so the unary parser can
		// recognize it without accepting it as a positive i64 value.
		if p.check(token.Int) && p.peek().Lexeme == "9223372036854775808" {
			p.advance()
			left = &ast.LiteralExpr{At: t.Pos, Kind: "i64", Text: "-9223372036854775808", Int: -1 << 63}
		} else {
			right, err := p.expression(precUnary)
			if err != nil {
				return nil, err
			}
			left = &ast.UnaryExpr{At: t.Pos, Op: string(t.Kind), Right: right}
		}
	case token.Bang:
		right, err := p.expression(precUnary)
		if err != nil {
			return nil, err
		}
		left = &ast.UnaryExpr{At: t.Pos, Op: string(t.Kind), Right: right}
	case token.LParen:
		e, err := p.expression(0)
		if err != nil {
			return nil, err
		}
		if _, err = p.consume(token.RParen, "expected ')' after expression"); err != nil {
			return nil, err
		}
		left = e
	default:
		return nil, p.errorAt(t, fmt.Sprintf("expected expression, got %s", t.Kind))
	}
	for {
		if p.check(token.LBracket) && precCall >= min {
			var err error
			left, err = p.finishIndex(left)
			if err != nil {
				return nil, err
			}
			continue
		}
		if p.check(token.LParen) && precCall >= min {
			var err error
			left, err = p.finishCall(left)
			if err != nil {
				return nil, err
			}
			continue
		}
		pr := precedence(p.peek().Kind)
		if pr == 0 || pr < min {
			break
		}
		op := p.advance()
		right, err := p.expression(pr + 1)
		if err != nil {
			return nil, err
		}
		left = &ast.BinaryExpr{At: op.Pos, Left: left, Op: string(op.Kind), Right: right}
	}
	return left, nil
}
func (p *Parser) finishIndex(collection ast.Expr) (ast.Expr, error) {
	start := p.advance()
	index, err := p.expression(0)
	if err != nil {
		return nil, err
	}
	if _, err = p.consume(token.RBracket, "expected ']' after index"); err != nil {
		return nil, err
	}
	return &ast.IndexExpr{At: start.Pos, Collection: collection, Index: index}, nil
}

func (p *Parser) finishCall(callee ast.Expr) (ast.Expr, error) {
	start := p.advance()
	args := []ast.Expr{}
	if !p.check(token.RParen) {
		for {
			arg, err := p.expression(0)
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			if !p.match(token.Comma) {
				break
			}
		}
	}
	if _, err := p.consume(token.RParen, "expected ')' after arguments"); err != nil {
		return nil, err
	}
	return &ast.CallExpr{At: start.Pos, Callee: callee, Args: args}, nil
}
func (p *Parser) check(k token.Kind) bool { return p.peek().Kind == k }
func (p *Parser) checkNext(k token.Kind) bool {
	return p.current+1 < len(p.tokens) && p.tokens[p.current+1].Kind == k
}
func (p *Parser) match(k token.Kind) bool {
	if p.check(k) {
		p.advance()
		return true
	}
	return false
}
func (p *Parser) consume(k token.Kind, msg string) (token.Token, error) {
	if p.check(k) {
		return p.advance(), nil
	}
	return token.Token{}, p.errorAt(p.peek(), msg)
}
func (p *Parser) advance() token.Token {
	current := p.tokens[p.current]
	if p.current < len(p.tokens)-1 {
		p.current++
	}
	return current
}
func (p *Parser) peek() token.Token     { return p.tokens[p.current] }
func (p *Parser) previous() token.Token { return p.tokens[p.current-1] }
func (p *Parser) errorAt(t token.Token, msg string) error {
	return fmt.Errorf("%d:%d: parser error: %s", t.Pos.Line, t.Pos.Column, msg)
}
