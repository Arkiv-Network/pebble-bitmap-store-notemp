package query

import (
	"context"
	"fmt"

	"github.com/RoaringBitmap/roaring/v2/roaring64"
)

func (t *AST) Evaluate(
	ctx context.Context,
	eval Evaluator,
) (*roaring64.Bitmap, error) {
	if t.Expr == nil {
		ids, err := eval.EvaluateAllCurrent(ctx)
		if err != nil {
			return nil, err
		}
		bm := roaring64.New()
		bm.AddMany(ids)
		return bm, nil
	}
	return t.Expr.Evaluate(ctx, eval)
}

func (e *ASTExpr) Evaluate(
	ctx context.Context,
	eval Evaluator,
) (*roaring64.Bitmap, error) {
	return e.Or.Evaluate(ctx, eval)
}

func (e *ASTOr) Evaluate(
	ctx context.Context,
	eval Evaluator,
) (*roaring64.Bitmap, error) {
	var tmp *roaring64.Bitmap

	for _, term := range e.Terms {
		bm, err := term.Evaluate(ctx, eval)
		if err != nil {
			return nil, err
		}
		if tmp == nil {
			tmp = bm
		} else {
			tmp.Or(bm)
		}
	}

	return tmp, nil
}

func (e *ASTAnd) Evaluate(
	ctx context.Context,
	eval Evaluator,
) (*roaring64.Bitmap, error) {
	var tmp *roaring64.Bitmap

	for _, term := range e.Terms {
		bm, err := term.Evaluate(ctx, eval)
		if err != nil {
			return nil, err
		}
		if tmp == nil {
			tmp = bm
		} else {
			tmp.And(bm)
		}
	}

	return tmp, nil
}

func (e *ASTTerm) Evaluate(
	ctx context.Context,
	eval Evaluator,
) (*roaring64.Bitmap, error) {
	switch {
	case e.Assign != nil:
		return e.Assign.Evaluate(ctx, eval)
	case e.Inclusion != nil:
		return e.Inclusion.Evaluate(ctx, eval)
	case e.LessThan != nil:
		return e.LessThan.Evaluate(ctx, eval)
	case e.LessOrEqualThan != nil:
		return e.LessOrEqualThan.Evaluate(ctx, eval)
	case e.GreaterThan != nil:
		return e.GreaterThan.Evaluate(ctx, eval)
	case e.GreaterOrEqualThan != nil:
		return e.GreaterOrEqualThan.Evaluate(ctx, eval)
	case e.Glob != nil:
		return e.Glob.Evaluate(ctx, eval)
	default:
		return nil, fmt.Errorf("unknown equal expression: %v", e)
	}
}

func reconstructStringBitmapsOR(ctx context.Context, eval Evaluator, name string, values []string) (*roaring64.Bitmap, error) {
	bm := roaring64.New()
	for _, v := range values {
		b, err := eval.ReconstructStringBitmap(ctx, name, v)
		if err != nil {
			return nil, err
		}
		bm.Or(b.Bitmap)
	}
	return bm, nil
}

func reconstructNumericBitmapsOR(ctx context.Context, eval Evaluator, name string, values []uint64) (*roaring64.Bitmap, error) {
	bm := roaring64.New()
	for _, v := range values {
		b, err := eval.ReconstructNumericBitmap(ctx, name, v)
		if err != nil {
			return nil, err
		}
		bm.Or(b.Bitmap)
	}
	return bm, nil
}

func (e *Glob) Evaluate(
	ctx context.Context,
	eval Evaluator,
) (*roaring64.Bitmap, error) {
	var values []string
	var err error

	if e.IsNot {
		values, err = eval.GetMatchingStringValuesNotGlob(ctx, e.Var, e.Value)
	} else {
		values, err = eval.GetMatchingStringValuesGlob(ctx, e.Var, e.Value)
	}
	if err != nil {
		return nil, err
	}

	return reconstructStringBitmapsOR(ctx, eval, e.Var, values)
}

func (e *LessThan) Evaluate(
	ctx context.Context,
	eval Evaluator,
) (*roaring64.Bitmap, error) {
	if e.Value.String != nil {
		values, err := eval.GetMatchingStringValuesLessThan(ctx, e.Var, *e.Value.String)
		if err != nil {
			return nil, err
		}
		return reconstructStringBitmapsOR(ctx, eval, e.Var, values)
	}

	values, err := eval.GetMatchingNumericValuesLessThan(ctx, e.Var, *e.Value.Number)
	if err != nil {
		return nil, err
	}
	return reconstructNumericBitmapsOR(ctx, eval, e.Var, values)
}

func (e *LessOrEqualThan) Evaluate(
	ctx context.Context,
	eval Evaluator,
) (*roaring64.Bitmap, error) {
	if e.Value.String != nil {
		values, err := eval.GetMatchingStringValuesLessOrEqualThan(ctx, e.Var, *e.Value.String)
		if err != nil {
			return nil, err
		}
		return reconstructStringBitmapsOR(ctx, eval, e.Var, values)
	}

	values, err := eval.GetMatchingNumericValuesLessOrEqualThan(ctx, e.Var, *e.Value.Number)
	if err != nil {
		return nil, err
	}
	return reconstructNumericBitmapsOR(ctx, eval, e.Var, values)
}

func (e *GreaterThan) Evaluate(
	ctx context.Context,
	eval Evaluator,
) (*roaring64.Bitmap, error) {
	if e.Value.String != nil {
		values, err := eval.GetMatchingStringValuesGreaterThan(ctx, e.Var, *e.Value.String)
		if err != nil {
			return nil, err
		}
		return reconstructStringBitmapsOR(ctx, eval, e.Var, values)
	}

	values, err := eval.GetMatchingNumericValuesGreaterThan(ctx, e.Var, *e.Value.Number)
	if err != nil {
		return nil, err
	}
	return reconstructNumericBitmapsOR(ctx, eval, e.Var, values)
}

func (e *GreaterOrEqualThan) Evaluate(
	ctx context.Context,
	eval Evaluator,
) (*roaring64.Bitmap, error) {
	if e.Value.String != nil {
		values, err := eval.GetMatchingStringValuesGreaterOrEqualThan(ctx, e.Var, *e.Value.String)
		if err != nil {
			return nil, err
		}
		return reconstructStringBitmapsOR(ctx, eval, e.Var, values)
	}

	values, err := eval.GetMatchingNumericValuesGreaterOrEqualThan(ctx, e.Var, *e.Value.Number)
	if err != nil {
		return nil, err
	}
	return reconstructNumericBitmapsOR(ctx, eval, e.Var, values)
}

func (e *Equality) Evaluate(
	ctx context.Context,
	eval Evaluator,
) (*roaring64.Bitmap, error) {
	if e.Value.String != nil {
		if e.IsNot {
			values, err := eval.GetMatchingStringValuesNotEqual(ctx, e.Var, *e.Value.String)
			if err != nil {
				return nil, err
			}
			return reconstructStringBitmapsOR(ctx, eval, e.Var, values)
		}

		values, err := eval.GetMatchingStringValuesEqual(ctx, e.Var, *e.Value.String)
		if err != nil {
			return nil, err
		}
		return reconstructStringBitmapsOR(ctx, eval, e.Var, values)
	}

	if e.IsNot {
		values, err := eval.GetMatchingNumericValuesNotEqual(ctx, e.Var, *e.Value.Number)
		if err != nil {
			return nil, err
		}
		return reconstructNumericBitmapsOR(ctx, eval, e.Var, values)
	}

	values, err := eval.GetMatchingNumericValuesEqual(ctx, e.Var, *e.Value.Number)
	if err != nil {
		return nil, err
	}
	return reconstructNumericBitmapsOR(ctx, eval, e.Var, values)
}

func (e *Inclusion) Evaluate(
	ctx context.Context,
	eval Evaluator,
) (*roaring64.Bitmap, error) {
	if len(e.Values.Strings) != 0 {
		var values []string
		var err error

		if e.IsNot {
			values, err = eval.GetMatchingStringValuesNotInclusion(ctx, e.Var, e.Values.Strings)
		} else {
			values, err = eval.GetMatchingStringValuesInclusion(ctx, e.Var, e.Values.Strings)
		}
		if err != nil {
			return nil, err
		}
		return reconstructStringBitmapsOR(ctx, eval, e.Var, values)
	}

	var values []uint64
	var err error

	if e.IsNot {
		values, err = eval.GetMatchingNumericValuesNotInclusion(ctx, e.Var, e.Values.Numbers)
	} else {
		values, err = eval.GetMatchingNumericValuesInclusion(ctx, e.Var, e.Values.Numbers)
	}
	if err != nil {
		return nil, err
	}
	return reconstructNumericBitmapsOR(ctx, eval, e.Var, values)
}
