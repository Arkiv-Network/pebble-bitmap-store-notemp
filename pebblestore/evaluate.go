package pebblestore

import (
	"context"
	"fmt"

	"github.com/RoaringBitmap/roaring/v2/roaring64"
	"github.com/cockroachdb/pebble"

	"github.com/Arkiv-Network/sqlite-bitmap-store/query"
)

func (s *PebbleStore) evaluateAST(ctx context.Context, reader pebble.Reader, ast *query.AST) (*roaring64.Bitmap, error) {
	if ast.Expr == nil {
		ids, err := s.evaluateAllCurrent(reader)
		if err != nil {
			return nil, err
		}
		bm := roaring64.New()
		bm.AddMany(ids)
		return bm, nil
	}
	return s.evaluateExpr(ctx, reader, &ast.Expr.Or)
}

func (s *PebbleStore) evaluateExpr(ctx context.Context, reader pebble.Reader, or *query.ASTOr) (*roaring64.Bitmap, error) {
	var tmp *roaring64.Bitmap

	for i := range or.Terms {
		bm, err := s.evaluateAnd(ctx, reader, &or.Terms[i])
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

func (s *PebbleStore) evaluateAnd(ctx context.Context, reader pebble.Reader, and *query.ASTAnd) (*roaring64.Bitmap, error) {
	var tmp *roaring64.Bitmap

	for i := range and.Terms {
		bm, err := s.evaluateTerm(ctx, reader, &and.Terms[i])
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

func (s *PebbleStore) evaluateTerm(ctx context.Context, reader pebble.Reader, term *query.ASTTerm) (*roaring64.Bitmap, error) {
	switch {
	case term.Assign != nil:
		return s.evaluateEquality(ctx, reader, term.Assign)
	case term.Inclusion != nil:
		return s.evaluateInclusion(ctx, reader, term.Inclusion)
	case term.LessThan != nil:
		return s.evaluateLessThan(ctx, reader, term.LessThan)
	case term.LessOrEqualThan != nil:
		return s.evaluateLessOrEqualThan(ctx, reader, term.LessOrEqualThan)
	case term.GreaterThan != nil:
		return s.evaluateGreaterThan(ctx, reader, term.GreaterThan)
	case term.GreaterOrEqualThan != nil:
		return s.evaluateGreaterOrEqualThan(ctx, reader, term.GreaterOrEqualThan)
	case term.Glob != nil:
		return s.evaluateGlob(ctx, reader, term.Glob)
	default:
		return nil, fmt.Errorf("unknown term: %v", term)
	}
}

func (s *PebbleStore) reconstructStringBitmapsOR(reader pebble.Reader, name string, values []string) (*roaring64.Bitmap, error) {
	bm := roaring64.New()
	for _, v := range values {
		b, err := s.GetStringBitmap(reader, name, v)
		if err != nil {
			return nil, err
		}
		bm.Or(b.Bitmap)
	}
	return bm, nil
}

func (s *PebbleStore) reconstructNumericBitmapsOR(reader pebble.Reader, name string, values []uint64) (*roaring64.Bitmap, error) {
	bm := roaring64.New()
	for _, v := range values {
		b, err := s.GetNumericBitmap(reader, name, v)
		if err != nil {
			return nil, err
		}
		bm.Or(b.Bitmap)
	}
	return bm, nil
}

func (s *PebbleStore) evaluateGlob(_ context.Context, reader pebble.Reader, e *query.Glob) (*roaring64.Bitmap, error) {
	var values []string
	var err error

	if e.IsNot {
		values, err = s.GetMatchingStringValuesNotGlob(reader, e.Var, e.Value)
	} else {
		values, err = s.GetMatchingStringValuesGlob(reader, e.Var, e.Value)
	}
	if err != nil {
		return nil, err
	}

	return s.reconstructStringBitmapsOR(reader, e.Var, values)
}

func (s *PebbleStore) evaluateLessThan(_ context.Context, reader pebble.Reader, e *query.LessThan) (*roaring64.Bitmap, error) {
	if e.Value.String != nil {
		values, err := s.GetMatchingStringValuesLessThan(reader, e.Var, *e.Value.String)
		if err != nil {
			return nil, err
		}
		return s.reconstructStringBitmapsOR(reader, e.Var, values)
	}

	values, err := s.GetMatchingNumericValuesLessThan(reader, e.Var, *e.Value.Number)
	if err != nil {
		return nil, err
	}
	return s.reconstructNumericBitmapsOR(reader, e.Var, values)
}

func (s *PebbleStore) evaluateLessOrEqualThan(_ context.Context, reader pebble.Reader, e *query.LessOrEqualThan) (*roaring64.Bitmap, error) {
	if e.Value.String != nil {
		values, err := s.GetMatchingStringValuesLessOrEqualThan(reader, e.Var, *e.Value.String)
		if err != nil {
			return nil, err
		}
		return s.reconstructStringBitmapsOR(reader, e.Var, values)
	}

	values, err := s.GetMatchingNumericValuesLessOrEqualThan(reader, e.Var, *e.Value.Number)
	if err != nil {
		return nil, err
	}
	return s.reconstructNumericBitmapsOR(reader, e.Var, values)
}

func (s *PebbleStore) evaluateGreaterThan(_ context.Context, reader pebble.Reader, e *query.GreaterThan) (*roaring64.Bitmap, error) {
	if e.Value.String != nil {
		values, err := s.GetMatchingStringValuesGreaterThan(reader, e.Var, *e.Value.String)
		if err != nil {
			return nil, err
		}
		return s.reconstructStringBitmapsOR(reader, e.Var, values)
	}

	values, err := s.GetMatchingNumericValuesGreaterThan(reader, e.Var, *e.Value.Number)
	if err != nil {
		return nil, err
	}
	return s.reconstructNumericBitmapsOR(reader, e.Var, values)
}

func (s *PebbleStore) evaluateGreaterOrEqualThan(_ context.Context, reader pebble.Reader, e *query.GreaterOrEqualThan) (*roaring64.Bitmap, error) {
	if e.Value.String != nil {
		values, err := s.GetMatchingStringValuesGreaterOrEqualThan(reader, e.Var, *e.Value.String)
		if err != nil {
			return nil, err
		}
		return s.reconstructStringBitmapsOR(reader, e.Var, values)
	}

	values, err := s.GetMatchingNumericValuesGreaterOrEqualThan(reader, e.Var, *e.Value.Number)
	if err != nil {
		return nil, err
	}
	return s.reconstructNumericBitmapsOR(reader, e.Var, values)
}

func (s *PebbleStore) evaluateEquality(_ context.Context, reader pebble.Reader, e *query.Equality) (*roaring64.Bitmap, error) {
	if e.Value.String != nil {
		if e.IsNot {
			values, err := s.GetMatchingStringValuesNotEqual(reader, e.Var, *e.Value.String)
			if err != nil {
				return nil, err
			}
			return s.reconstructStringBitmapsOR(reader, e.Var, values)
		}

		values, err := s.GetMatchingStringValuesEqual(reader, e.Var, *e.Value.String)
		if err != nil {
			return nil, err
		}
		return s.reconstructStringBitmapsOR(reader, e.Var, values)
	}

	if e.IsNot {
		values, err := s.GetMatchingNumericValuesNotEqual(reader, e.Var, *e.Value.Number)
		if err != nil {
			return nil, err
		}
		return s.reconstructNumericBitmapsOR(reader, e.Var, values)
	}

	values, err := s.GetMatchingNumericValuesEqual(reader, e.Var, *e.Value.Number)
	if err != nil {
		return nil, err
	}
	return s.reconstructNumericBitmapsOR(reader, e.Var, values)
}

func (s *PebbleStore) evaluateInclusion(_ context.Context, reader pebble.Reader, e *query.Inclusion) (*roaring64.Bitmap, error) {
	if len(e.Values.Strings) != 0 {
		var values []string
		var err error

		if e.IsNot {
			values, err = s.GetMatchingStringValuesNotInclusion(reader, e.Var, e.Values.Strings)
		} else {
			values, err = s.GetMatchingStringValuesInclusion(reader, e.Var, e.Values.Strings)
		}
		if err != nil {
			return nil, err
		}
		return s.reconstructStringBitmapsOR(reader, e.Var, values)
	}

	var values []uint64
	var err error

	if e.IsNot {
		values, err = s.GetMatchingNumericValuesNotInclusion(reader, e.Var, e.Values.Numbers)
	} else {
		values, err = s.GetMatchingNumericValuesInclusion(reader, e.Var, e.Values.Numbers)
	}
	if err != nil {
		return nil, err
	}
	return s.reconstructNumericBitmapsOR(reader, e.Var, values)
}
