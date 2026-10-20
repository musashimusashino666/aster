// Package ast defines Aster's source-level abstract syntax tree.
package ast

import "github.com/aster-lang/aster/internal/token"

type Program struct {
	Decls      []Decl
	SourceName string
}
type Node interface{ Pos() token.Position }
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
	Element *TypeRef
}

func (t TypeRef) Pos() token.Position { return t.At }

type Function struct {
	At         token.Position
	Name       string
	Params     []Param
	ReturnType *TypeRef
	Body       *BlockStmt
}

func (n *Function) Pos() token.Position { return n.At }
func (*Function) declNode()             {}

// StructDecl declares a nominal, non-generic record type.
type StructDecl struct {
	At     token.Position
	Name   string
	Fields []StructField
}

func (n *StructDecl) Pos() token.Position { return n.At }
func (*StructDecl) declNode()             {}

// EnumDecl declares a closed nominal sum type. Variant payloads are positional.
type EnumDecl struct {
	At       token.Position
	Name     string
	Variants []EnumVariant
}

func (n *EnumDecl) Pos() token.Position { return n.At }
func (*EnumDecl) declNode()             {}

type EnumVariant struct {
	At      token.Position
	Name    string
	Payload []TypeRef
}


type StructField struct {
	At   token.Position
	Name string
	Type TypeRef
}

type Param struct {
	At   token.Position
	Name string
	Type TypeRef
}

type StmtDecl struct{ Statement Stmt }

func (n *StmtDecl) Pos() token.Position { return n.Statement.Pos() }
func (*StmtDecl) declNode()             {}

type BlockStmt struct {
	At         token.Position
	Statements []Stmt
}

func (n *BlockStmt) Pos() token.Position { return n.At }
func (*BlockStmt) stmtNode()             {}

type LetStmt struct {
	At         token.Position
	Name       string
	Mutable    bool
	Annotation *TypeRef
	Value      Expr
}

func (n *LetStmt) Pos() token.Position { return n.At }
func (*LetStmt) stmtNode()             {}

type AssignStmt struct {
	At    token.Position
	Name  string
	Value Expr
}

func (n *AssignStmt) Pos() token.Position { return n.At }
func (*AssignStmt) stmtNode()             {}

type IndexAssignStmt struct {
	At     token.Position
	Target Expr
	Value  Expr
}

func (n *IndexAssignStmt) Pos() token.Position { return n.At }
func (*IndexAssignStmt) stmtNode()             {}

type ExprStmt struct {
	At   token.Position
	Expr Expr
}

func (n *ExprStmt) Pos() token.Position { return n.At }
func (*ExprStmt) stmtNode()             {}

type IfStmt struct {
	At        token.Position
	Condition Expr
	Then      *BlockStmt
	Else      Stmt
}

func (n *IfStmt) Pos() token.Position { return n.At }
func (*IfStmt) stmtNode()             {}

type WhileStmt struct {
	At        token.Position
	Condition Expr
	Body      *BlockStmt
}

func (n *WhileStmt) Pos() token.Position { return n.At }
func (*WhileStmt) stmtNode()             {}

type ReturnStmt struct {
	At    token.Position
	Value Expr
}

func (n *ReturnStmt) Pos() token.Position { return n.At }
func (*ReturnStmt) stmtNode()             {}

type IdentifierExpr struct {
	At   token.Position
	Name string
}

func (n *IdentifierExpr) Pos() token.Position { return n.At }
func (*IdentifierExpr) exprNode()             {}

// FieldExpr reads a declared field from a struct value.
type FieldExpr struct {
	At     token.Position
	Object Expr
	Name   string
}

func (n *FieldExpr) Pos() token.Position { return n.At }
func (*FieldExpr) exprNode()             {}

type LiteralExpr struct {
	At     token.Position
	Kind   string
	Text   string
	Int    int64
	Float  float64
	String string
	Bool   bool
}

func (n *LiteralExpr) Pos() token.Position { return n.At }
func (*LiteralExpr) exprNode()             {}

type UnaryExpr struct {
	At    token.Position
	Op    string
	Right Expr
}

func (n *UnaryExpr) Pos() token.Position { return n.At }
func (*UnaryExpr) exprNode()             {}

type BinaryExpr struct {
	At    token.Position
	Left  Expr
	Op    string
	Right Expr
}

func (n *BinaryExpr) Pos() token.Position { return n.At }
func (*BinaryExpr) exprNode()             {}

type CallExpr struct {
	At     token.Position
	Callee Expr
	Args   []Expr
}

func (n *CallExpr) Pos() token.Position { return n.At }
func (*CallExpr) exprNode()             {}

// EnumVariantExpr constructs a concrete enum variant using a qualified name.
type EnumVariantExpr struct {
	At          token.Position
	EnumName    string
	VariantName string
	Args        []Expr
	HasPayload  bool
}

func (n *EnumVariantExpr) Pos() token.Position { return n.At }
func (*EnumVariantExpr) exprNode()             {}

// MatchPattern is either a wildcard or a flat, qualified enum variant pattern.
type MatchPattern struct {
	At          token.Position
	Wildcard    bool
	EnumName    string
	VariantName string
	Bindings    []string
	HasPayload  bool
}

type MatchArm struct {
	At      token.Position
	Pattern MatchPattern
	Expr    Expr
}

// MatchExpr evaluates its subject once and returns the selected arm expression.
type MatchExpr struct {
	At      token.Position
	Subject Expr
	Arms    []MatchArm
}

func (n *MatchExpr) Pos() token.Position { return n.At }
func (*MatchExpr) exprNode()             {}

// ArrayExpr creates a homogeneous, dynamically sized array literal.
type ArrayExpr struct {
	At       token.Position
	Elements []Expr
}

func (n *ArrayExpr) Pos() token.Position { return n.At }
func (*ArrayExpr) exprNode()             {}

// IndexExpr reads one element from an array. Bounds are checked at runtime.
type IndexExpr struct {
	At         token.Position
	Collection Expr
	Index      Expr
}

func (n *IndexExpr) Pos() token.Position { return n.At }
func (*IndexExpr) exprNode()             {}

// SliceExpr creates an independent array containing collection[start:end].
// The start is inclusive and the end is exclusive.
type SliceExpr struct {
	At         token.Position
	Collection Expr
	Start      Expr
	End        Expr
}

func (n *SliceExpr) Pos() token.Position { return n.At }
func (*SliceExpr) exprNode()             {}
