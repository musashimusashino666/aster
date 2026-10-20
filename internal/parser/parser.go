// Package parser implements a recursive-descent parser with Pratt expression parsing.
package parser

import (
	"fmt"
	"strconv"

	"github.com/aster-lang/aster/internal/ast"
	"github.com/aster-lang/aster/internal/diagnostic"
	"github.com/aster-lang/aster/internal/langtypes"
	"github.com/aster-lang/aster/internal/lexer"
	"github.com/aster-lang/aster/internal/token"
)

const maxSyntaxNestingDepth = 256

type Parser struct {
	tokens       []token.Token
	current      int
	nestingDepth int
}

func (p *Parser) enterNesting(at token.Token, context string) (func(), error) {
	if p.nestingDepth >= maxSyntaxNestingDepth {
		return nil, p.errorAtCode(at, diagnostic.ParserSyntaxNesting, fmt.Sprintf("syntax nesting exceeds limit %d while parsing %s", maxSyntaxNestingDepth, context))
	}
	p.nestingDepth++
	return func() { p.nestingDepth-- }, nil
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
			return nil, p.errorAtCode(p.peek(), diagnostic.ParserNoProgress, "parser made no progress")
		}
	}
	return program, nil
}
func (p *Parser) declaration() (ast.Decl, error) {
	if p.match(token.Fn) {
		return p.function(p.previous())
	}
	if p.match(token.Struct) {
		return p.structDeclaration(p.previous())
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
		return nil, p.errorAtCode(name, diagnostic.ParserReservedInternalName, "function names beginning with '__' are reserved for compiler internals")
	}
	if name.Lexeme == "main" {
		return nil, p.errorAtCode(name, diagnostic.ParserReservedMainName, "'main' is reserved for the top-level program")
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
func (p *Parser) structDeclaration(start token.Token) (*ast.StructDecl, error) {
	name, err := p.consume(token.Ident, "expected struct name after 'struct'")
	if err != nil {
		return nil, err
	}
	open, err := p.consume(token.LBrace, "expected '{' after struct name")
	if err != nil {
		return nil, err
	}
	leave, err := p.enterNesting(open, "struct declaration")
	if err != nil {
		return nil, err
	}
	defer leave()

	fields := []ast.StructField{}
	for !p.check(token.RBrace) && !p.check(token.EOF) {
		field, e := p.consume(token.Ident, "expected field name")
		if e != nil {
			return nil, e
		}
		if _, e = p.consume(token.Colon, "expected ':' after field name"); e != nil {
			return nil, e
		}
		fieldType, e := p.typeRef()
		if e != nil {
			return nil, e
		}
		fields = append(fields, ast.StructField{At: field.Pos, Name: field.Lexeme, Type: fieldType})
		if !p.match(token.Comma) {
			break
		}
	}
	if _, err = p.consume(token.RBrace, "expected '}' after struct fields"); err != nil {
		return nil, err
	}
	return &ast.StructDecl{At: start.Pos, Name: name.Lexeme, Fields: fields}, nil
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
			return ast.TypeRef{}, p.errorAtCode(t, diagnostic.ParserTypeNesting, fmt.Sprintf("type nesting exceeds limit %d", langtypes.MaxNestingDepth))
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
		target, err := p.expression(0)
		if err != nil {
			return nil, err
		}
		if _, err := p.consume(token.Assign, "expected '=' after array index target"); err != nil {
			return nil, err
		}
		value, err := p.expression(0)
		if err != nil {
			return nil, err
		}
		if _, err := p.consume(token.Semicolon, "expected ';' after array element assignment"); err != nil {
			return nil, err
		}
		return &ast.IndexAssignStmt{At: target.Pos(), Target: target, Value: value}, nil
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

// indexedAssignmentAhead recognizes an identifier-rooted index chain followed by assignment.
func (p *Parser) indexedAssignmentAhead() bool {
	if !p.check(token.Ident) || !p.checkNext(token.LBracket) {
		return false
	}
	i := p.current + 1
	for i < len(p.tokens) && p.tokens[i].Kind == token.LBracket {
		depth := 1
		i++
		for i < len(p.tokens) && depth > 0 {
			switch p.tokens[i].Kind {
			case token.LBracket:
				depth++
			case token.RBracket:
				depth--
			}
			i++
		}
		if depth != 0 || i >= len(p.tokens) {
			return false
		}
		if p.tokens[i].Kind == token.Assign {
			return true
		}
		if p.tokens[i].Kind != token.LBracket {
			return false
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
	leave, err := p.enterNesting(start, "if statement")
	if err != nil {
		return nil, err
	}
	defer leave()
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
	leave, err := p.enterNesting(start, "block")
	if err != nil {
		return nil, err
	}
	defer leave()
	b := &ast.BlockStmt{At: start.Pos}
	for !p.check(token.RBrace) && !p.check(token.EOF) {
		before := p.current
		s, err := p.statement()
		if err != nil {
			return nil, err
		}
		b.Statements = append(b.Statements, s)
		if before == p.current {
			return nil, p.errorAtCode(p.peek(), diagnostic.ParserNoProgress, "parser made no progress in block")
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
	expr, _, err := p.parseExpression(min)
	return expr, err
}

// parseExpression returns the expression and its AST height. Tracking AST height
// matters in addition to recursive parser depth: a long left-associative operator
// or postfix chain is built iteratively but later traversed recursively by the
// checker and backends.
func (p *Parser) parseExpression(min int) (ast.Expr, int, error) {
	leave, err := p.enterNesting(p.peek(), "expression")
	if err != nil {
		return nil, 0, err
	}
	defer leave()

	var left ast.Expr
	leftDepth := 0
	t := p.advance()
	switch t.Kind {
	case token.Int:
		n, err := strconv.ParseInt(t.Lexeme, 10, 64)
		if err != nil {
			if t.Lexeme == "9223372036854775808" {
				return nil, 0, p.errorAtCode(t, diagnostic.ParserIntegerOutOfRange, "integer literal out of range; this magnitude is valid only immediately after unary '-' (write -9223372036854775808)")
			}
			return nil, 0, p.errorAtCode(t, diagnostic.ParserIntegerOutOfRange, "integer literal out of range")
		}
		left = &ast.LiteralExpr{At: t.Pos, Kind: "i64", Text: t.Lexeme, Int: n}
		leftDepth = 1
	case token.Float:
		n, err := strconv.ParseFloat(t.Lexeme, 64)
		if err != nil {
			return nil, 0, p.errorAtCode(t, diagnostic.ParserInvalidFloat, "invalid float")
		}
		left = &ast.LiteralExpr{At: t.Pos, Kind: "f64", Text: t.Lexeme, Float: n}
		leftDepth = 1
	case token.String:
		left = &ast.LiteralExpr{At: t.Pos, Kind: "string", Text: t.Lexeme, String: t.Lexeme}
		leftDepth = 1
	case token.True:
		left = &ast.LiteralExpr{At: t.Pos, Kind: "bool", Text: "true", Bool: true}
		leftDepth = 1
	case token.False:
		left = &ast.LiteralExpr{At: t.Pos, Kind: "bool", Text: "false", Bool: false}
		leftDepth = 1
	case token.Ident:
		left = &ast.IdentifierExpr{At: t.Pos, Name: t.Lexeme}
		leftDepth = 1
	case token.LBracket:
		elements := []ast.Expr{}
		maxElementDepth := 0
		if !p.check(token.RBracket) {
			for {
				item, itemDepth, err := p.parseExpression(0)
				if err != nil {
					return nil, 0, err
				}
				elements = append(elements, item)
				if itemDepth > maxElementDepth {
					maxElementDepth = itemDepth
				}
				if !p.match(token.Comma) {
					break
				}
			}
		}
		if _, err := p.consume(token.RBracket, "expected ']' after array elements"); err != nil {
			return nil, 0, err
		}
		left = &ast.ArrayExpr{At: t.Pos, Elements: elements}
		leftDepth = maxElementDepth + 1
	case token.Minus:
		// The positive magnitude 2^63 is only valid as part of the i64 minimum
		// literal. The lexer permits this one magnitude so the unary parser can
		// recognize it without accepting it as a positive i64 value.
		if p.check(token.Int) && p.peek().Lexeme == "9223372036854775808" {
			p.advance()
			left = &ast.LiteralExpr{At: t.Pos, Kind: "i64", Text: "-9223372036854775808", Int: -1 << 63}
			leftDepth = 1
		} else {
			right, rightDepth, err := p.parseExpression(precUnary)
			if err != nil {
				return nil, 0, err
			}
			left = &ast.UnaryExpr{At: t.Pos, Op: string(t.Kind), Right: right}
			leftDepth = rightDepth + 1
		}
	case token.Bang:
		right, rightDepth, err := p.parseExpression(precUnary)
		if err != nil {
			return nil, 0, err
		}
		left = &ast.UnaryExpr{At: t.Pos, Op: string(t.Kind), Right: right}
		leftDepth = rightDepth + 1
	case token.LParen:
		e, expressionDepth, err := p.parseExpression(0)
		if err != nil {
			return nil, 0, err
		}
		if _, err = p.consume(token.RParen, "expected ')' after expression"); err != nil {
			return nil, 0, err
		}
		left = e
		leftDepth = expressionDepth
	default:
		return nil, 0, p.errorAtCode(t, diagnostic.ParserExpectedExpression, fmt.Sprintf("expected expression, got %s", t.Kind))
	}

	if leftDepth > maxSyntaxNestingDepth {
		return nil, 0, p.errorAtCode(t, diagnostic.ParserExpressionNesting, fmt.Sprintf("expression nesting exceeds limit %d", maxSyntaxNestingDepth))
	}
	for {
		if p.check(token.Dot) && precCall >= min {
			dot := p.advance()
			field, err := p.consume(token.Ident, "expected field name after '.'")
			if err != nil {
				return nil, 0, err
			}
			leftDepth++
			if leftDepth > maxSyntaxNestingDepth {
				return nil, 0, p.errorAtCode(dot, diagnostic.ParserExpressionNesting, fmt.Sprintf("expression nesting exceeds limit %d", maxSyntaxNestingDepth))
			}
			left = &ast.FieldExpr{At: dot.Pos, Object: left, Name: field.Lexeme}
			continue
		}
		if p.check(token.LBracket) && precCall >= min {
			var err error
			left, leftDepth, err = p.finishIndex(left, leftDepth)
			if err != nil {
				return nil, 0, err
			}
			continue
		}
		if p.check(token.LParen) && precCall >= min {
			var err error
			left, leftDepth, err = p.finishCall(left, leftDepth)
			if err != nil {
				return nil, 0, err
			}
			continue
		}
		pr := precedence(p.peek().Kind)
		if pr == 0 || pr < min {
			break
		}
		op := p.advance()
		right, rightDepth, err := p.parseExpression(pr + 1)
		if err != nil {
			return nil, 0, err
		}
		if rightDepth > leftDepth {
			leftDepth = rightDepth
		}
		leftDepth++
		if leftDepth > maxSyntaxNestingDepth {
			return nil, 0, p.errorAtCode(op, diagnostic.ParserExpressionNesting, fmt.Sprintf("expression nesting exceeds limit %d", maxSyntaxNestingDepth))
		}
		left = &ast.BinaryExpr{At: op.Pos, Left: left, Op: string(op.Kind), Right: right}
	}
	return left, leftDepth, nil
}

func (p *Parser) finishIndex(collection ast.Expr, collectionDepth int) (ast.Expr, int, error) {
	start := p.advance()
	lower, lowerDepth, err := p.parseExpression(0)
	if err != nil {
		return nil, 0, err
	}
	if p.match(token.Colon) {
		upper, upperDepth, err := p.parseExpression(0)
		if err != nil {
			return nil, 0, err
		}
		if _, err = p.consume(token.RBracket, "expected ']' after slice bounds"); err != nil {
			return nil, 0, err
		}
		depth := collectionDepth
		if lowerDepth > depth {
			depth = lowerDepth
		}
		if upperDepth > depth {
			depth = upperDepth
		}
		depth++
		if depth > maxSyntaxNestingDepth {
			return nil, 0, p.errorAtCode(start, diagnostic.ParserExpressionNesting, fmt.Sprintf("expression nesting exceeds limit %d", maxSyntaxNestingDepth))
		}
		return &ast.SliceExpr{At: start.Pos, Collection: collection, Start: lower, End: upper}, depth, nil
	}
	if _, err = p.consume(token.RBracket, "expected ']' after index"); err != nil {
		return nil, 0, err
	}
	depth := collectionDepth
	if lowerDepth > depth {
		depth = lowerDepth
	}
	depth++
	if depth > maxSyntaxNestingDepth {
		return nil, 0, p.errorAtCode(start, diagnostic.ParserExpressionNesting, fmt.Sprintf("expression nesting exceeds limit %d", maxSyntaxNestingDepth))
	}
	return &ast.IndexExpr{At: start.Pos, Collection: collection, Index: lower}, depth, nil
}

func (p *Parser) finishCall(callee ast.Expr, calleeDepth int) (ast.Expr, int, error) {
	start := p.advance()
	args := []ast.Expr{}
	maxChildDepth := calleeDepth
	if !p.check(token.RParen) {
		for {
			arg, argDepth, err := p.parseExpression(0)
			if err != nil {
				return nil, 0, err
			}
			args = append(args, arg)
			if argDepth > maxChildDepth {
				maxChildDepth = argDepth
			}
			if !p.match(token.Comma) {
				break
			}
		}
	}
	if _, err := p.consume(token.RParen, "expected ')' after arguments"); err != nil {
		return nil, 0, err
	}
	depth := maxChildDepth + 1
	if depth > maxSyntaxNestingDepth {
		return nil, 0, p.errorAtCode(start, diagnostic.ParserExpressionNesting, fmt.Sprintf("expression nesting exceeds limit %d", maxSyntaxNestingDepth))
	}
	return &ast.CallExpr{At: start.Pos, Callee: callee, Args: args}, depth, nil
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
	return p.errorAtCode(t, diagnostic.ParserGeneric, msg)
}

func (p *Parser) errorAtCode(t token.Token, code, msg string) error {
	return diagnostic.New(code, "parser", t.Pos, msg)
}
