// Package ast defines Aster's source-level abstract syntax tree.
package ast

import "github.com/aster-lang/aster/internal/token"

type Program struct {
	Decls      []Decl
	SourceName string
}
type Node interface {
	// Pos retains the existing diagnostic anchor for this node.
	Pos() token.Position
	// EndPos returns the exclusive end position of the node's source syntax.
	EndPos() token.Position
}

// SourceRangeStart returns the syntactic left edge for the user-visible
// source range. Pos remains the legacy diagnostic anchor, which can point at an
// operator or postfix delimiter. Parenthesized expressions retain the opening
// parenthesis as RangeAt while their existing diagnostic anchor remains stable.
func SourceRangeStart(node Node) token.Position {
	if node == nil {
		return token.Position{}
	}
	if expr, ok := node.(Expr); ok {
		if start := expressionRangeStart(expr); start.Line > 0 && start.Column > 0 && start.Offset >= 0 {
			return start
		}
	}
	switch n := node.(type) {
	case *BinaryExpr:
		return SourceRangeStart(n.Left)
	case *FieldExpr:
		return SourceRangeStart(n.Object)
	case *IndexExpr:
		return SourceRangeStart(n.Collection)
	case *SliceExpr:
		return SourceRangeStart(n.Collection)
	case *CallExpr:
		return SourceRangeStart(n.Callee)
	default:
		return node.Pos()
	}
}

func expressionRangeStart(expr Expr) token.Position {
	switch n := expr.(type) {
	case *IdentifierExpr:
		return n.RangeAt
	case *FieldExpr:
		return n.RangeAt
	case *LiteralExpr:
		return n.RangeAt
	case *UnaryExpr:
		return n.RangeAt
	case *BinaryExpr:
		return n.RangeAt
	case *CallExpr:
		return n.RangeAt
	case *EnumVariantExpr:
		return n.RangeAt
	case *MatchExpr:
		return n.RangeAt
	case *ArrayExpr:
		return n.RangeAt
	case *IndexExpr:
		return n.RangeAt
	case *SliceExpr:
		return n.RangeAt
	default:
		return token.Position{}
	}
}

type Decl interface {
	Node
	declNode()
}
type Stmt interface {
	Node
	stmtNode()
}
type Expr interface {
	Node
	exprNode()
}

type TypeRef struct {
	Name    string
	At      token.Position
	EndAt   token.Position
	Element *TypeRef
	Args    []TypeRef
}

func (t TypeRef) Pos() token.Position    { return t.At }
func (t TypeRef) EndPos() token.Position { return t.EndAt }

type Function struct {
	At         token.Position
	EndAt      token.Position
	Name       string
	TypeParams []string
	Params     []Param
	ReturnType *TypeRef
	Body       *BlockStmt
}

func (n *Function) Pos() token.Position    { return n.At }
func (n *Function) EndPos() token.Position { return n.EndAt }
func (*Function) declNode()                {}

// StructDecl declares a nominal record type. Generic declarations are
// monomorphized into concrete declarations before type checking.
type StructDecl struct {
	At         token.Position
	EndAt      token.Position
	Name       string
	TypeParams []string
	Fields     []StructField
}

func (n *StructDecl) Pos() token.Position    { return n.At }
func (n *StructDecl) EndPos() token.Position { return n.EndAt }
func (*StructDecl) declNode()                {}

// EnumDecl declares a closed nominal sum type. Variant payloads are positional.
type EnumDecl struct {
	At         token.Position
	EndAt      token.Position
	Name       string
	TypeParams []string
	Variants   []EnumVariant
}

func (n *EnumDecl) Pos() token.Position    { return n.At }
func (n *EnumDecl) EndPos() token.Position { return n.EndAt }
func (*EnumDecl) declNode()                {}

type EnumVariant struct {
	At      token.Position
	EndAt   token.Position
	Name    string
	Payload []TypeRef
}

func (n EnumVariant) Pos() token.Position    { return n.At }
func (n EnumVariant) EndPos() token.Position { return n.EndAt }

type StructField struct {
	At    token.Position
	EndAt token.Position
	Name  string
	Type  TypeRef
}

func (n StructField) Pos() token.Position    { return n.At }
func (n StructField) EndPos() token.Position { return n.EndAt }

type Param struct {
	At    token.Position
	EndAt token.Position
	Name  string
	Type  TypeRef
}

func (n Param) Pos() token.Position    { return n.At }
func (n Param) EndPos() token.Position { return n.EndAt }

type StmtDecl struct{ Statement Stmt }

func (n *StmtDecl) Pos() token.Position    { return n.Statement.Pos() }
func (n *StmtDecl) EndPos() token.Position { return n.Statement.EndPos() }
func (*StmtDecl) declNode()                {}

type BlockStmt struct {
	At         token.Position
	EndAt      token.Position
	Statements []Stmt
}

func (n *BlockStmt) Pos() token.Position    { return n.At }
func (n *BlockStmt) EndPos() token.Position { return n.EndAt }
func (*BlockStmt) stmtNode()                {}

type LetStmt struct {
	At         token.Position
	EndAt      token.Position
	Name       string
	Mutable    bool
	Annotation *TypeRef
	Value      Expr
}

func (n *LetStmt) Pos() token.Position    { return n.At }
func (n *LetStmt) EndPos() token.Position { return n.EndAt }
func (*LetStmt) stmtNode()                {}

type AssignStmt struct {
	At    token.Position
	EndAt token.Position
	Name  string
	Value Expr
}

func (n *AssignStmt) Pos() token.Position    { return n.At }
func (n *AssignStmt) EndPos() token.Position { return n.EndAt }
func (*AssignStmt) stmtNode()                {}

type IndexAssignStmt struct {
	At     token.Position
	EndAt  token.Position
	Target Expr
	Value  Expr
}

func (n *IndexAssignStmt) Pos() token.Position    { return n.At }
func (n *IndexAssignStmt) EndPos() token.Position { return n.EndAt }
func (*IndexAssignStmt) stmtNode()                {}

type ExprStmt struct {
	At    token.Position
	EndAt token.Position
	Expr  Expr
}

func (n *ExprStmt) Pos() token.Position    { return n.At }
func (n *ExprStmt) EndPos() token.Position { return n.EndAt }
func (*ExprStmt) stmtNode()                {}

type IfStmt struct {
	At        token.Position
	EndAt     token.Position
	Condition Expr
	Then      *BlockStmt
	Else      Stmt
}

func (n *IfStmt) Pos() token.Position    { return n.At }
func (n *IfStmt) EndPos() token.Position { return n.EndAt }
func (*IfStmt) stmtNode()                {}

type WhileStmt struct {
	At        token.Position
	EndAt     token.Position
	Condition Expr
	Body      *BlockStmt
}

func (n *WhileStmt) Pos() token.Position    { return n.At }
func (n *WhileStmt) EndPos() token.Position { return n.EndAt }
func (*WhileStmt) stmtNode()                {}

type ReturnStmt struct {
	At    token.Position
	EndAt token.Position
	Value Expr
}

func (n *ReturnStmt) Pos() token.Position    { return n.At }
func (n *ReturnStmt) EndPos() token.Position { return n.EndAt }
func (*ReturnStmt) stmtNode()                {}

type IdentifierExpr struct {
	At      token.Position
	RangeAt token.Position
	EndAt   token.Position
	Name    string
}

func (n *IdentifierExpr) Pos() token.Position    { return n.At }
func (n *IdentifierExpr) EndPos() token.Position { return n.EndAt }
func (*IdentifierExpr) exprNode()                {}

// FieldExpr reads a declared field from a struct value.
type FieldExpr struct {
	At      token.Position
	RangeAt token.Position
	EndAt   token.Position
	Object  Expr
	Name    string
}

func (n *FieldExpr) Pos() token.Position    { return n.At }
func (n *FieldExpr) EndPos() token.Position { return n.EndAt }
func (*FieldExpr) exprNode()                {}

type LiteralExpr struct {
	At      token.Position
	RangeAt token.Position
	EndAt   token.Position
	Kind    string
	Text    string
	Int     int64
	Float   float64
	String  string
	Bool    bool
}

func (n *LiteralExpr) Pos() token.Position    { return n.At }
func (n *LiteralExpr) EndPos() token.Position { return n.EndAt }
func (*LiteralExpr) exprNode()                {}

type UnaryExpr struct {
	At      token.Position
	RangeAt token.Position
	EndAt   token.Position
	Op      string
	Right   Expr
}

func (n *UnaryExpr) Pos() token.Position    { return n.At }
func (n *UnaryExpr) EndPos() token.Position { return n.EndAt }
func (*UnaryExpr) exprNode()                {}

type BinaryExpr struct {
	At      token.Position
	RangeAt token.Position
	EndAt   token.Position
	Left    Expr
	Op      string
	Right   Expr
}

func (n *BinaryExpr) Pos() token.Position    { return n.At }
func (n *BinaryExpr) EndPos() token.Position { return n.EndAt }
func (*BinaryExpr) exprNode()                {}

type CallExpr struct {
	At       token.Position
	RangeAt  token.Position
	EndAt    token.Position
	Callee   Expr
	TypeArgs []TypeRef
	Args     []Expr
}

func (n *CallExpr) Pos() token.Position    { return n.At }
func (n *CallExpr) EndPos() token.Position { return n.EndAt }
func (*CallExpr) exprNode()                {}

// EnumVariantExpr constructs a concrete enum variant using a qualified name.
type EnumVariantExpr struct {
	At          token.Position
	RangeAt     token.Position
	EndAt       token.Position
	EnumName    string
	VariantName string
	TypeArgs    []TypeRef
	Args        []Expr
	HasPayload  bool
}

func (n *EnumVariantExpr) Pos() token.Position    { return n.At }
func (n *EnumVariantExpr) EndPos() token.Position { return n.EndAt }
func (*EnumVariantExpr) exprNode()                {}

// MatchPattern is either a wildcard or a flat, qualified enum variant pattern.
type MatchPattern struct {
	At          token.Position
	EndAt       token.Position
	Wildcard    bool
	EnumName    string
	VariantName string
	Bindings    []string
	HasPayload  bool
}

func (n MatchPattern) Pos() token.Position    { return n.At }
func (n MatchPattern) EndPos() token.Position { return n.EndAt }

type MatchArm struct {
	At      token.Position
	EndAt   token.Position
	Pattern MatchPattern
	Expr    Expr
}

func (n MatchArm) Pos() token.Position    { return n.At }
func (n MatchArm) EndPos() token.Position { return n.EndAt }

// MatchExpr evaluates its subject once and returns the selected arm expression.
type MatchExpr struct {
	At      token.Position
	RangeAt token.Position
	EndAt   token.Position
	Subject Expr
	Arms    []MatchArm
}

func (n *MatchExpr) Pos() token.Position    { return n.At }
func (n *MatchExpr) EndPos() token.Position { return n.EndAt }
func (*MatchExpr) exprNode()                {}

// ArrayExpr creates a homogeneous, dynamically sized array literal.
type ArrayExpr struct {
	At       token.Position
	RangeAt  token.Position
	EndAt    token.Position
	Elements []Expr
}

func (n *ArrayExpr) Pos() token.Position    { return n.At }
func (n *ArrayExpr) EndPos() token.Position { return n.EndAt }
func (*ArrayExpr) exprNode()                {}

// IndexExpr reads one element from an array. Bounds are checked at runtime.
type IndexExpr struct {
	At         token.Position
	RangeAt    token.Position
	EndAt      token.Position
	Collection Expr
	Index      Expr
}

func (n *IndexExpr) Pos() token.Position    { return n.At }
func (n *IndexExpr) EndPos() token.Position { return n.EndAt }
func (*IndexExpr) exprNode()                {}

// SliceExpr creates an independent array containing collection[start:end].
// The start is inclusive and the end is exclusive.
type SliceExpr struct {
	At         token.Position
	RangeAt    token.Position
	EndAt      token.Position
	Collection Expr
	Start      Expr
	End        Expr
}

func (n *SliceExpr) Pos() token.Position    { return n.At }
func (n *SliceExpr) EndPos() token.Position { return n.EndAt }
func (*SliceExpr) exprNode()                {}
