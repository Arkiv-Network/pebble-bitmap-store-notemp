package query

import (
	"context"

	"github.com/Arkiv-Network/sqlite-bitmap-store/store"
)

type Evaluator interface {
	EvaluateAllCurrent(ctx context.Context) ([]uint64, error)

	// String attribute queries
	GetMatchingStringValuesEqual(ctx context.Context, name, value string) ([]string, error)
	GetMatchingStringValuesNotEqual(ctx context.Context, name, value string) ([]string, error)
	GetMatchingStringValuesLessThan(ctx context.Context, name, value string) ([]string, error)
	GetMatchingStringValuesGreaterThan(ctx context.Context, name, value string) ([]string, error)
	GetMatchingStringValuesLessOrEqualThan(ctx context.Context, name, value string) ([]string, error)
	GetMatchingStringValuesGreaterOrEqualThan(ctx context.Context, name, value string) ([]string, error)
	GetMatchingStringValuesGlob(ctx context.Context, name, pattern string) ([]string, error)
	GetMatchingStringValuesNotGlob(ctx context.Context, name, pattern string) ([]string, error)
	GetMatchingStringValuesInclusion(ctx context.Context, name string, values []string) ([]string, error)
	GetMatchingStringValuesNotInclusion(ctx context.Context, name string, values []string) ([]string, error)

	// Numeric attribute queries
	GetMatchingNumericValuesEqual(ctx context.Context, name string, value uint64) ([]uint64, error)
	GetMatchingNumericValuesNotEqual(ctx context.Context, name string, value uint64) ([]uint64, error)
	GetMatchingNumericValuesLessThan(ctx context.Context, name string, value uint64) ([]uint64, error)
	GetMatchingNumericValuesGreaterThan(ctx context.Context, name string, value uint64) ([]uint64, error)
	GetMatchingNumericValuesLessOrEqualThan(ctx context.Context, name string, value uint64) ([]uint64, error)
	GetMatchingNumericValuesGreaterOrEqualThan(ctx context.Context, name string, value uint64) ([]uint64, error)
	GetMatchingNumericValuesInclusion(ctx context.Context, name string, values []uint64) ([]uint64, error)
	GetMatchingNumericValuesNotInclusion(ctx context.Context, name string, values []uint64) ([]uint64, error)

	// Bitmap reconstruction (current state only)
	ReconstructStringBitmap(ctx context.Context, name, value string) (*store.Bitmap, error)
	ReconstructNumericBitmap(ctx context.Context, name string, value uint64) (*store.Bitmap, error)
}
