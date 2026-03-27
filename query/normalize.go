package query

import (
	"slices"
	"strings"
)

type AST struct {
	Expr *ASTExpr
}
type ASTExpr struct {
	Or ASTOr
}
type ASTOr struct {
	Terms []ASTAnd
}
type ASTAnd struct {
	Terms []ASTTerm
}
type ASTTerm struct {
	Assign             *Equality
	Inclusion          *Inclusion
	LessThan           *LessThan
	LessOrEqualThan    *LessOrEqualThan
	GreaterThan        *GreaterThan
	GreaterOrEqualThan *GreaterOrEqualThan
	Glob               *Glob
}

func (t *TopLevel) Normalize() *AST {
	if t.Expression != nil {
		return &AST{
			Expr: t.Expression.Normalize(),
		}
	}
	return &AST{}
}

func (e *Expression) Normalize() *ASTExpr {
	normalised := e.Or.Normalize()
	return &ASTExpr{
		Or: *normalised,
	}
}

func (e *Expression) invert() *Expression {

	newLeft := e.Or.invert()

	if len(newLeft.Right) == 0 {
		// By construction, this will always be a Paren
		if newLeft.Left.Paren == nil {
			panic("This should never happen!")
		}
		return &newLeft.Left.Paren.Nested
	}

	return &Expression{
		Or: OrExpression{
			Left: *newLeft,
		},
	}
}

func (e *OrExpression) Normalize() *ASTOr {
	terms := e.Left.Normalize()
	for _, rhs := range e.Right {
		terms = append(terms, rhs.Normalize()...)
	}

	return &ASTOr{
		Terms: terms,
	}
}

func (e *OrExpression) invert() *AndExpression {
	newLeft := EqualExpr{
		Paren: &Paren{
			IsNot: false,
			Nested: Expression{
				Or: *e.Left.invert(),
			},
		},
	}

	var newRight []*AndRHS = nil

	if e.Right != nil {
		newRight = make([]*AndRHS, 0, len(e.Right))
		for _, rhs := range e.Right {
			newRight = append(newRight, rhs.invert())
		}
	}

	return &AndExpression{
		Left:  newLeft,
		Right: newRight,
	}
}

func (e *OrRHS) Normalize() []ASTAnd {
	return e.Expr.Normalize()
}

func (e *OrRHS) invert() *AndRHS {
	return &AndRHS{
		Expr: EqualExpr{
			Paren: &Paren{
				IsNot: false,
				Nested: Expression{
					Or: *e.Expr.invert(),
				},
			},
		},
	}
}

func (e *EqualExpr) convertToTerms() [][]ASTTerm {
	// First level is OR, second level is AND
	es := [][]ASTTerm{}

	if e.Paren != nil {
		// This is where we recursively convert to DNF and also where negations
		// get pushed down
		normalised := e.Paren.Normalize()
		for _, conjunction := range normalised.Or.Terms {
			// Add one array per OR term, containing all the AND terms
			es = append(es, conjunction.Terms)
		}
	} else {
		es = append(es, []ASTTerm{e.Normalize()})
	}

	return es
}

func (e *AndExpression) Normalize() []ASTAnd {
	// We have an AND node and all its terms are ASTs with potential nesting.
	// We can eliminate nesting by normalising the parens (which will recurse into
	// the sub-ASTs and flatten them into DNF), and then construct an array
	// with for every term a nested array representing the OR and AND nodes.
	terms := [][][]ASTTerm{e.Left.convertToTerms()}
	for _, rhs := range e.Right {
		terms = append(terms, rhs.Expr.convertToTerms())
	}

	// Calculate the cross product, this distributes the outer AND into the nested ORs
	// and gives us a new AST that has no nested disjunctions
	ast := []ASTAnd{{
		Terms: []ASTTerm{},
	}}

	for _, disjunctions := range terms {
		// The part of the AST that we construct in this step
		// This starts off empty at every step, and we fill it up based on the
		// AST that we build in the previous step, until we're done.
		tmpAst := []ASTAnd{}
		for _, conjunction := range ast {
			for _, terms := range disjunctions {
				// Copy the conjunction with the terms collected so far
				combined := slices.Clone(conjunction.Terms)
				// Add the additional terms found in this step
				combined = append(combined, terms...)
				// Add the new AND node to the AST that we're building
				tmpAst = append(tmpAst, ASTAnd{Terms: combined})
			}
		}
		ast = tmpAst
	}

	return ast
}

func (e *AndExpression) invert() *OrExpression {
	newLeft := AndExpression{
		Left: *e.Left.invert(),
	}

	var newRight []*OrRHS = nil

	if e.Right != nil {
		newRight = make([]*OrRHS, 0, len(e.Right))
		for _, rhs := range e.Right {
			newRight = append(newRight, rhs.invert())
		}
	}

	return &OrExpression{
		Left:  newLeft,
		Right: newRight,
	}
}

func (e *AndRHS) Normalize() ASTTerm {
	return e.Expr.Normalize()
}

func (e *AndRHS) invert() *OrRHS {
	return &OrRHS{
		Expr: AndExpression{
			Left: *e.Expr.invert(),
		},
	}
}

func (e *EqualExpr) Normalize() ASTTerm {

	if e.Paren != nil {
		panic("Called EqualExpr::Normalize on a paren, this is a bug!")
	}

	if e.LessThan != nil {
		return ASTTerm{LessThan: e.LessThan.Normalize()}
	}

	if e.LessOrEqualThan != nil {
		return ASTTerm{LessOrEqualThan: e.LessOrEqualThan.Normalize()}
	}

	if e.GreaterThan != nil {
		return ASTTerm{GreaterThan: e.GreaterThan.Normalize()}
	}

	if e.GreaterOrEqualThan != nil {
		return ASTTerm{GreaterOrEqualThan: e.GreaterOrEqualThan.Normalize()}
	}

	if e.Glob != nil {
		return ASTTerm{Glob: e.Glob.Normalize()}
	}

	if e.Assign != nil {
		return ASTTerm{Assign: e.Assign.Normalize()}
	}

	if e.Inclusion != nil {
		return ASTTerm{Inclusion: e.Inclusion.Normalize()}
	}

	panic("This should not happen!")
}

func (e *EqualExpr) invert() *EqualExpr {
	if e.Paren != nil {
		return &EqualExpr{Paren: e.Paren.invert()}
	}

	if e.LessThan != nil {
		return &EqualExpr{GreaterOrEqualThan: e.LessThan.invert()}
	}

	if e.LessOrEqualThan != nil {
		return &EqualExpr{GreaterThan: e.LessOrEqualThan.invert()}
	}

	if e.GreaterThan != nil {
		return &EqualExpr{LessOrEqualThan: e.GreaterThan.invert()}
	}

	if e.GreaterOrEqualThan != nil {
		return &EqualExpr{LessThan: e.GreaterOrEqualThan.invert()}
	}

	if e.Glob != nil {
		return &EqualExpr{Glob: e.Glob.invert()}
	}

	if e.Assign != nil {
		return &EqualExpr{Assign: e.Assign.invert()}
	}

	if e.Inclusion != nil {
		return &EqualExpr{Inclusion: e.Inclusion.invert()}
	}

	panic("This should not happen!")
}

func (e *Paren) Normalize() *ASTExpr {
	nested := e.Nested

	if e.IsNot {
		nested = *nested.invert()
	}

	return nested.Normalize()
}

func (e *Paren) invert() *Paren {
	return &Paren{
		IsNot:  !e.IsNot,
		Nested: e.Nested,
	}
}

func (e *Glob) Normalize() *Glob {
	// TODO do we need to change casing here too?
	return e
}

func (e *Glob) invert() *Glob {
	return &Glob{
		Var:   e.Var,
		IsNot: !e.IsNot,
		Value: e.Value,
	}
}

func (e *LessThan) Normalize() *LessThan {
	switch e.Var {
	case KeyAttributeKey, OwnerAttributeKey, CreatorAttributeKey:
		val := strings.ToLower(*e.Value.String)
		return &LessThan{
			Var: e.Var,
			Value: Value{
				String: &val,
			},
		}
	default:
		return e
	}
}

func (e *LessThan) invert() *GreaterOrEqualThan {
	return &GreaterOrEqualThan{
		Var:   e.Var,
		Value: e.Value,
	}
}

func (e *LessOrEqualThan) Normalize() *LessOrEqualThan {
	switch e.Var {
	case KeyAttributeKey, OwnerAttributeKey, CreatorAttributeKey:
		val := strings.ToLower(*e.Value.String)
		return &LessOrEqualThan{
			Var: e.Var,
			Value: Value{
				String: &val,
			},
		}
	default:
		return e
	}
}

func (e *LessOrEqualThan) invert() *GreaterThan {
	return &GreaterThan{
		Var:   e.Var,
		Value: e.Value,
	}
}

func (e *GreaterThan) Normalize() *GreaterThan {
	switch e.Var {
	case KeyAttributeKey, OwnerAttributeKey, CreatorAttributeKey:
		val := strings.ToLower(*e.Value.String)
		return &GreaterThan{
			Var: e.Var,
			Value: Value{
				String: &val,
			},
		}
	default:
		return e
	}
}

func (e *GreaterThan) invert() *LessOrEqualThan {
	return &LessOrEqualThan{
		Var:   e.Var,
		Value: e.Value,
	}
}

func (e *GreaterOrEqualThan) Normalize() *GreaterOrEqualThan {
	switch e.Var {
	case KeyAttributeKey, OwnerAttributeKey, CreatorAttributeKey:
		val := strings.ToLower(*e.Value.String)
		return &GreaterOrEqualThan{
			Var: e.Var,
			Value: Value{
				String: &val,
			},
		}
	default:
		return e
	}
}

func (e *GreaterOrEqualThan) invert() *LessThan {
	return &LessThan{
		Var:   e.Var,
		Value: e.Value,
	}
}

func (e *Equality) Normalize() *Equality {
	switch e.Var {
	case KeyAttributeKey, OwnerAttributeKey, CreatorAttributeKey:
		val := strings.ToLower(*e.Value.String)
		return &Equality{
			Var:   e.Var,
			IsNot: e.IsNot,
			Value: Value{
				String: &val,
			},
		}
	default:
		return e
	}
}

func (e *Equality) invert() *Equality {
	return &Equality{
		Var:   e.Var,
		IsNot: !e.IsNot,
		Value: e.Value,
	}
}

func (e *Inclusion) Normalize() *Inclusion {
	switch e.Var {
	case KeyAttributeKey, OwnerAttributeKey, CreatorAttributeKey:
		vals := make([]string, 0, len(e.Values.Strings))
		for _, val := range e.Values.Strings {
			vals = append(vals, strings.ToLower(val))
		}
		return &Inclusion{
			Var: e.Var,
			Values: Values{
				Strings: vals,
			},
		}
	default:
		return e
	}
}

func (e *Inclusion) invert() *Inclusion {
	return &Inclusion{
		Var:    e.Var,
		IsNot:  !e.IsNot,
		Values: e.Values,
	}
}
