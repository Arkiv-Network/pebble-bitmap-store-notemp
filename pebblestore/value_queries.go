package pebblestore

import (
	"path"

	"github.com/cockroachdb/pebble"
)

// scanDistinctStringValues scans all string bitmap keys for the given name
// and returns values that pass the filter.
func (s *PebbleStore) scanDistinctStringValues(reader pebble.Reader, name string, filter func(value string) bool) ([]string, error) {
	prefix := stringBitmapNamePrefix(name)
	iter, err := reader.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixUpperBound(prefix),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var results []string
	for iter.First(); iter.Valid(); iter.Next() {
		_, value := decodeStringBitmapKey(iter.Key())
		if filter(value) {
			results = append(results, value)
		}
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}
	return results, nil
}

// scanDistinctNumericValues scans all numeric bitmap keys for the given name
// and returns values that pass the filter.
func (s *PebbleStore) scanDistinctNumericValues(reader pebble.Reader, name string, filter func(value uint64) bool) ([]uint64, error) {
	prefix := numericBitmapNamePrefix(name)
	iter, err := reader.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixUpperBound(prefix),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var results []uint64
	for iter.First(); iter.Valid(); iter.Next() {
		_, value := decodeNumericBitmapKey(iter.Key())
		if filter(value) {
			results = append(results, value)
		}
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// String value queries
// ---------------------------------------------------------------------------

// GetMatchingStringValuesEqual returns all distinct string values for name
// that are equal to value.
func (s *PebbleStore) GetMatchingStringValuesEqual(reader pebble.Reader, name, value string) ([]string, error) {
	return s.scanDistinctStringValues(reader, name, func(v string) bool { return v == value })
}

// GetMatchingStringValuesNotEqual returns all distinct string values for name
// that are not equal to value.
func (s *PebbleStore) GetMatchingStringValuesNotEqual(reader pebble.Reader, name, value string) ([]string, error) {
	return s.scanDistinctStringValues(reader, name, func(v string) bool { return v != value })
}

// GetMatchingStringValuesLessThan returns all distinct string values for name
// that are lexicographically less than value.
func (s *PebbleStore) GetMatchingStringValuesLessThan(reader pebble.Reader, name, value string) ([]string, error) {
	return s.scanDistinctStringValues(reader, name, func(v string) bool { return v < value })
}

// GetMatchingStringValuesGreaterThan returns all distinct string values for name
// that are lexicographically greater than value.
func (s *PebbleStore) GetMatchingStringValuesGreaterThan(reader pebble.Reader, name, value string) ([]string, error) {
	return s.scanDistinctStringValues(reader, name, func(v string) bool { return v > value })
}

// GetMatchingStringValuesLessOrEqualThan returns all distinct string values for
// name that are lexicographically less than or equal to value.
func (s *PebbleStore) GetMatchingStringValuesLessOrEqualThan(reader pebble.Reader, name, value string) ([]string, error) {
	return s.scanDistinctStringValues(reader, name, func(v string) bool { return v <= value })
}

// GetMatchingStringValuesGreaterOrEqualThan returns all distinct string values
// for name that are lexicographically greater than or equal to value.
func (s *PebbleStore) GetMatchingStringValuesGreaterOrEqualThan(reader pebble.Reader, name, value string) ([]string, error) {
	return s.scanDistinctStringValues(reader, name, func(v string) bool { return v >= value })
}

// GetMatchingStringValuesGlob returns all distinct string values for name that
// match the given glob pattern. Uses path.Match which supports * and ?
// wildcards, similar to SQLite GLOB.
func (s *PebbleStore) GetMatchingStringValuesGlob(reader pebble.Reader, name, pattern string) ([]string, error) {
	return s.scanDistinctStringValues(reader, name, func(v string) bool {
		matched, _ := path.Match(pattern, v)
		return matched
	})
}

// GetMatchingStringValuesNotGlob returns all distinct string values for name
// that do not match the given glob pattern.
func (s *PebbleStore) GetMatchingStringValuesNotGlob(reader pebble.Reader, name, pattern string) ([]string, error) {
	return s.scanDistinctStringValues(reader, name, func(v string) bool {
		matched, _ := path.Match(pattern, v)
		return !matched
	})
}

// GetMatchingStringValuesInclusion returns all distinct string values for name
// that are contained in the provided values slice.
func (s *PebbleStore) GetMatchingStringValuesInclusion(reader pebble.Reader, name string, values []string) ([]string, error) {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	return s.scanDistinctStringValues(reader, name, func(v string) bool {
		_, ok := set[v]
		return ok
	})
}

// GetMatchingStringValuesNotInclusion returns all distinct string values for
// name that are not contained in the provided values slice.
func (s *PebbleStore) GetMatchingStringValuesNotInclusion(reader pebble.Reader, name string, values []string) ([]string, error) {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	return s.scanDistinctStringValues(reader, name, func(v string) bool {
		_, ok := set[v]
		return !ok
	})
}

// ---------------------------------------------------------------------------
// Numeric value queries
// ---------------------------------------------------------------------------

// GetMatchingNumericValuesEqual returns all distinct numeric values for name
// that are equal to value.
func (s *PebbleStore) GetMatchingNumericValuesEqual(reader pebble.Reader, name string, value uint64) ([]uint64, error) {
	return s.scanDistinctNumericValues(reader, name, func(v uint64) bool { return v == value })
}

// GetMatchingNumericValuesNotEqual returns all distinct numeric values for name
// that are not equal to value.
func (s *PebbleStore) GetMatchingNumericValuesNotEqual(reader pebble.Reader, name string, value uint64) ([]uint64, error) {
	return s.scanDistinctNumericValues(reader, name, func(v uint64) bool { return v != value })
}

// GetMatchingNumericValuesLessThan returns all distinct numeric values for name
// that are less than value.
func (s *PebbleStore) GetMatchingNumericValuesLessThan(reader pebble.Reader, name string, value uint64) ([]uint64, error) {
	return s.scanDistinctNumericValues(reader, name, func(v uint64) bool { return v < value })
}

// GetMatchingNumericValuesGreaterThan returns all distinct numeric values for
// name that are greater than value.
func (s *PebbleStore) GetMatchingNumericValuesGreaterThan(reader pebble.Reader, name string, value uint64) ([]uint64, error) {
	return s.scanDistinctNumericValues(reader, name, func(v uint64) bool { return v > value })
}

// GetMatchingNumericValuesLessOrEqualThan returns all distinct numeric values
// for name that are less than or equal to value.
func (s *PebbleStore) GetMatchingNumericValuesLessOrEqualThan(reader pebble.Reader, name string, value uint64) ([]uint64, error) {
	return s.scanDistinctNumericValues(reader, name, func(v uint64) bool { return v <= value })
}

// GetMatchingNumericValuesGreaterOrEqualThan returns all distinct numeric values
// for name that are greater than or equal to value.
func (s *PebbleStore) GetMatchingNumericValuesGreaterOrEqualThan(reader pebble.Reader, name string, value uint64) ([]uint64, error) {
	return s.scanDistinctNumericValues(reader, name, func(v uint64) bool { return v >= value })
}

// GetMatchingNumericValuesInclusion returns all distinct numeric values for name
// that are contained in the provided values slice.
func (s *PebbleStore) GetMatchingNumericValuesInclusion(reader pebble.Reader, name string, values []uint64) ([]uint64, error) {
	set := make(map[uint64]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	return s.scanDistinctNumericValues(reader, name, func(v uint64) bool {
		_, ok := set[v]
		return ok
	})
}

// GetMatchingNumericValuesNotInclusion returns all distinct numeric values for
// name that are not contained in the provided values slice.
func (s *PebbleStore) GetMatchingNumericValuesNotInclusion(reader pebble.Reader, name string, values []uint64) ([]uint64, error) {
	set := make(map[uint64]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	return s.scanDistinctNumericValues(reader, name, func(v uint64) bool {
		_, ok := set[v]
		return !ok
	})
}
