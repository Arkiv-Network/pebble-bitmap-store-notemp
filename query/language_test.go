package query

import (
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func pointerOf[T any](v T) *T {
	return &v
}

func TestParse(t *testing.T) {
	t.Run("quoted string", func(t *testing.T) {
		v, err := Parse(`name = "test\"2"`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										Assign: &Equality{
											Var:   "name",
											IsNot: false,
											Value: Value{
												String: pointerOf("test\"2"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	t.Run("empty query", func(t *testing.T) {
		_, err := Parse(``)
		require.Error(t, err)
	})

	t.Run("all", func(t *testing.T) {
		v, err := Parse(`$all`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: nil,
			},
			v,
		)
	})

	t.Run("number", func(t *testing.T) {
		v, err := Parse(`name = 123`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										Assign: &Equality{
											Var:   "name",
											IsNot: false,
											Value: Value{
												Number: pointerOf(uint64(123)),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	t.Run("not parentheses", func(t *testing.T) {
		v, err := Parse(`!(name = 123 || name = 456)`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										Assign: &Equality{
											Var:   "name",
											IsNot: true,
											Value: Value{
												Number: pointerOf(uint64(123)),
											},
										},
									},
									{
										Assign: &Equality{
											Var:   "name",
											IsNot: true,
											Value: Value{
												Number: pointerOf(uint64(456)),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	t.Run("not number", func(t *testing.T) {
		v, err := Parse(`!(name = 123)`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										Assign: &Equality{
											Var:   "name",
											IsNot: true,
											Value: Value{
												Number: pointerOf(uint64(123)),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	t.Run("not equal", func(t *testing.T) {
		v, err := Parse(`name != 123`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										Assign: &Equality{
											Var:   "name",
											IsNot: true,
											Value: Value{
												Number: pointerOf(uint64(123)),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	t.Run("lessthan", func(t *testing.T) {
		v, err := Parse(`name < 123`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										LessThan: &LessThan{
											Var: "name",
											Value: Value{
												Number: pointerOf(uint64(123)),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)

		v, err = Parse(`name < "123"`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										LessThan: &LessThan{
											Var: "name",
											Value: Value{
												String: pointerOf("123"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)

		v, err = Parse(`!(name < 123)`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										GreaterOrEqualThan: &GreaterOrEqualThan{
											Var: "name",
											Value: Value{
												Number: pointerOf(uint64(123)),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	t.Run("lessthanequal", func(t *testing.T) {
		v, err := Parse(`name <= 123`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										LessOrEqualThan: &LessOrEqualThan{
											Var: "name",
											Value: Value{
												Number: pointerOf(uint64(123)),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)

		v, err = Parse(`name <= "123"`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										LessOrEqualThan: &LessOrEqualThan{
											Var: "name",
											Value: Value{
												String: pointerOf("123"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	t.Run("greaterthan", func(t *testing.T) {
		v, err := Parse(`name > 123`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										GreaterThan: &GreaterThan{
											Var: "name",
											Value: Value{
												Number: pointerOf(uint64(123)),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)

		v, err = Parse(`name > "123"`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										GreaterThan: &GreaterThan{
											Var: "name",
											Value: Value{
												String: pointerOf("123"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	t.Run("greaterthanequal", func(t *testing.T) {
		v, err := Parse(`name >= 123`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										GreaterOrEqualThan: &GreaterOrEqualThan{
											Var: "name",
											Value: Value{
												Number: pointerOf(uint64(123)),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)

		v, err = Parse(`name >= "123"`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										GreaterOrEqualThan: &GreaterOrEqualThan{
											Var: "name",
											Value: Value{
												String: pointerOf("123"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	t.Run("glob", func(t *testing.T) {
		v, err := Parse(`name ~ "foo"`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										Glob: &Glob{
											Var:   "name",
											IsNot: false,
											Value: "foo",
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	t.Run("owner", func(t *testing.T) {
		owner := common.HexToAddress("0x1").Hex()
		v, err := Parse(fmt.Sprintf(`$owner = %s`, owner))
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										Assign: &Equality{
											Var:   "$owner",
											IsNot: false,
											Value: Value{
												String: &owner,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	t.Run("owner quoted", func(t *testing.T) {
		owner := common.HexToAddress("0x1").Hex()
		v, err := Parse(fmt.Sprintf(`$owner = "%s"`, owner))
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										Assign: &Equality{
											Var:   "$owner",
											IsNot: false,
											Value: Value{
												String: &owner,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	t.Run("not owner", func(t *testing.T) {
		owner := common.HexToAddress("0x1").Hex()
		v, err := Parse(fmt.Sprintf(`$owner != %s`, owner))
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										Assign: &Equality{
											Var:   "$owner",
											IsNot: true,
											Value: Value{
												String: &owner,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	t.Run("glob", func(t *testing.T) {
		v, err := Parse(`name ~ "foo"`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										Glob: &Glob{
											Var:   "name",
											IsNot: false,
											Value: "foo",
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	t.Run("not glob", func(t *testing.T) {
		v, err := Parse(`name !~ "foo"`)
		require.NoError(t, err)

		require.Equal(
			t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										Glob: &Glob{
											Var:   "name",
											IsNot: true,
											Value: "foo",
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	t.Run("and", func(t *testing.T) {
		v, err := Parse(`(name = 123 && name2 = "abc")`)
		require.NoError(t, err)

		require.Equal(t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										Assign: &Equality{
											Var:   "name",
											IsNot: false,
											Value: Value{
												Number: pointerOf(uint64(123)),
											},
										},
									},
									{
										Assign: &Equality{
											Var:   "name2",
											IsNot: false,
											Value: Value{
												String: pointerOf("abc"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	t.Run("or", func(t *testing.T) {
		v, err := Parse(`name = 123 || name2 = "abc"`)
		require.NoError(t, err)

		require.Equal(t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										Assign: &Equality{
											Var:   "name",
											IsNot: false,
											Value: Value{
												Number: pointerOf(uint64(123)),
											},
										},
									},
								},
							},
							{
								Terms: []ASTTerm{
									{
										Assign: &Equality{
											Var:   "name2",
											IsNot: false,
											Value: Value{
												String: pointerOf("abc"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	// (n1 || (n2 && (n3 && (n5 || n4))))
	// (n1 || (n2 && (n3 && n5) || (n3 && n4))))
	// (n1 || ((n2 && n3 && n5) || (n2 && n3 && n4)))
	// n1 || (n2 && n3 && n5) || (n2 && n3 && n4)
	t.Run("nested parentheses", func(t *testing.T) {
		v, err := Parse(`(n1 = 1 || (n2 = 2 && (n3 = 3 && (n5 = 5 || n4 = 4))))`)
		require.NoError(t, err)

		require.Equal(t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										Assign: &Equality{
											Var:   "n1",
											IsNot: false,
											Value: Value{
												Number: pointerOf(uint64(1)),
											},
										},
									},
								},
							},
							{
								Terms: []ASTTerm{
									{
										Assign: &Equality{
											Var:   "n2",
											IsNot: false,
											Value: Value{
												Number: pointerOf(uint64(2)),
											},
										},
									},
									{
										Assign: &Equality{
											Var:   "n3",
											IsNot: false,
											Value: Value{
												Number: pointerOf(uint64(3)),
											},
										},
									},
									{
										Assign: &Equality{
											Var:   "n5",
											IsNot: false,
											Value: Value{
												Number: pointerOf(uint64(5)),
											},
										},
									},
								},
							},
							{
								Terms: []ASTTerm{
									{
										Assign: &Equality{
											Var:   "n2",
											IsNot: false,
											Value: Value{
												Number: pointerOf(uint64(2)),
											},
										},
									},
									{
										Assign: &Equality{
											Var:   "n3",
											IsNot: false,
											Value: Value{
												Number: pointerOf(uint64(3)),
											},
										},
									},
									{
										Assign: &Equality{
											Var:   "n4",
											IsNot: false,
											Value: Value{
												Number: pointerOf(uint64(4)),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	// (n || n2) && (n3 && n5) || n4
	// ( (n || n2) && n3 && n5 ) || n4
	// ( n && n3 && n5 ) || ( n2 && n3 && n5 ) || n4
	t.Run("parentheses", func(t *testing.T) {
		v, err := Parse(`(name = 123 || name2 = "abc") && (name3 = "def" && name5 = 5) || name4 = 456`)
		require.NoError(t, err)

		require.Equal(t,
			&AST{
				Expr: &ASTExpr{
					Or: ASTOr{
						Terms: []ASTAnd{
							{
								Terms: []ASTTerm{
									{
										Assign: &Equality{
											Var:   "name",
											IsNot: false,
											Value: Value{
												Number: pointerOf(uint64(123)),
											},
										},
									},
									{
										Assign: &Equality{
											Var:   "name3",
											IsNot: false,
											Value: Value{
												String: pointerOf("def"),
											},
										},
									},
									{
										Assign: &Equality{
											Var:   "name5",
											IsNot: false,
											Value: Value{
												Number: pointerOf(uint64(5)),
											},
										},
									},
								},
							},
							{
								Terms: []ASTTerm{
									{
										Assign: &Equality{
											Var:   "name2",
											IsNot: false,
											Value: Value{
												String: pointerOf("abc"),
											},
										},
									},
									{
										Assign: &Equality{
											Var:   "name3",
											IsNot: false,
											Value: Value{
												String: pointerOf("def"),
											},
										},
									},
									{
										Assign: &Equality{
											Var:   "name5",
											IsNot: false,
											Value: Value{
												Number: pointerOf(uint64(5)),
											},
										},
									},
								},
							},
							{
								Terms: []ASTTerm{
									{
										Assign: &Equality{
											Var:   "name4",
											IsNot: false,
											Value: Value{
												Number: pointerOf(uint64(456)),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			v,
		)
	})

	t.Run("invalid expression", func(t *testing.T) {
		_, err := Parse(`key = 8e`)
		require.Error(t, err, `1:8: unexpected token "e"`)
	})

	t.Run("invalid expression", func(t *testing.T) {
		_, err := Parse(`key = 8e`)
		require.Error(t, err, `1:8: unexpected token "e"`)
	})

}
